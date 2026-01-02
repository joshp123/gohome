package rate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// RateLimitError is returned when calls are blocked.
type RateLimitError struct {
	Provider string
	Reason   string
	RetryAt  time.Time
}

func (e RateLimitError) Error() string {
	if e.RetryAt.IsZero() {
		return fmt.Sprintf("%s rate limited: %s", e.Provider, e.Reason)
	}
	return fmt.Sprintf("%s rate limited: %s (retry at %s)", e.Provider, e.Reason, e.RetryAt.UTC().Format(time.RFC3339))
}

type Decision struct {
	Allowed bool
	Reason  string
	RetryAt time.Time
}

type bucket struct {
	capacity int
	tokens   float64
	last     time.Time
}

type cacheEntry struct {
	status   int
	header   http.Header
	body     []byte
	expires  time.Time
	provider string
}

// State tracks observed limits.
type State struct {
	remaining   map[Window]int
	limits      map[Window]int
	budgetFloor map[Window]int
	buckets     map[Window]*bucket
	hasHeaders  map[Window]bool
	cooldown    time.Time
	lastStatus  int
}

// Guard enforces rate limits for a provider.
type Guard struct {
	decl Declaration
	mu   sync.Mutex
	// state is mutated under mu
	state State
	cache map[string]cacheEntry
}

// WrapHTTP wraps an http.Client with rate-limit enforcement.
func WrapHTTP(decl Declaration, base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	client := *base
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	guard := newGuard(decl)
	client.Transport = &roundTripper{
		base:  transport,
		guard: guard,
	}
	return &client
}

func newGuard(decl Declaration) *Guard {
	state := State{
		remaining:   make(map[Window]int),
		limits:      make(map[Window]int),
		budgetFloor: make(map[Window]int),
		buckets:     make(map[Window]*bucket),
		hasHeaders:  make(map[Window]bool),
	}
	for window, limit := range decl.Limits() {
		state.limits[window] = limit
		state.remaining[window] = limit
		state.buckets[window] = &bucket{
			capacity: limit,
			tokens:   float64(limit),
			last:     time.Now(),
		}
	}
	for window, floor := range decl.BudgetFloors() {
		state.budgetFloor[window] = floor
	}

	return &Guard{
		decl:  decl,
		state: state,
		cache: make(map[string]cacheEntry),
	}
}

type roundTripper struct {
	base  http.RoundTripper
	guard *Guard
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	bodyBytes, err := drainBody(req)
	if err != nil {
		return nil, err
	}

	decision := rt.guard.ShouldCall(time.Now())
	if !decision.Allowed {
		if cached := rt.guard.cachedResponse(req, bodyBytes); cached != nil {
			return cached, nil
		}
		return nil, RateLimitError{
			Provider: rt.guard.decl.ProviderName(),
			Reason:   decision.Reason,
			RetryAt:  decision.RetryAt,
		}
	}

	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	rt.guard.RecordResponse(resp.StatusCode, resp.Header)
	resp, err = rt.guard.maybeCacheResponse(req, bodyBytes, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (g *Guard) ShouldCall(now time.Time) Decision {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.decl.CustomPolicy() != nil {
		return g.decl.CustomPolicy()(&g.state, now)
	}

	if !g.decl.HasLimits() {
		return Decision{Allowed: false, Reason: "disabled"}
	}

	if !g.state.cooldown.IsZero() && now.Before(g.state.cooldown) {
		return Decision{Allowed: false, Reason: "cooldown", RetryAt: g.state.cooldown}
	}

	for window, limit := range g.state.limits {
		floor := g.state.budgetFloor[window]
		if g.state.hasHeaders[window] {
			if g.state.remaining[window] <= floor {
				return Decision{Allowed: false, Reason: "budget", RetryAt: g.state.cooldown}
			}
			g.state.remaining[window]--
			continue
		}
		if limit <= 0 {
			return Decision{Allowed: false, Reason: "disabled"}
		}
		if !consumeToken(g.state.buckets[window], windowDuration(window)) {
			retryAt := g.state.buckets[window].last.Add(windowDuration(window) / time.Duration(limit))
			return Decision{Allowed: false, Reason: "budget", RetryAt: retryAt}
		}
	}

	return Decision{Allowed: true}
}

func (g *Guard) RecordResponse(status int, headers http.Header) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.state.lastStatus = status
	lastStatusGauge.WithLabelValues(g.decl.ProviderName()).Set(float64(status))

	parsed := parseHeaders(headers, g.decl.Headers())
	now := time.Now()

	if parsed.retryAfter > 0 {
		g.state.cooldown = now.Add(time.Duration(parsed.retryAfter) * time.Second)
		retryAfterGauge.WithLabelValues(g.decl.ProviderName()).Set(float64(parsed.retryAfter))
	}
	if parsed.resetAfter > 0 && g.state.cooldown.IsZero() {
		g.state.cooldown = now.Add(time.Duration(parsed.resetAfter) * time.Second)
		retryAfterGauge.WithLabelValues(g.decl.ProviderName()).Set(float64(parsed.resetAfter))
	}

	updateWindow := func(window Window, remaining int, limit int) {
		if remaining < 0 {
			return
		}
		g.state.remaining[window] = remaining
		g.state.limits[window] = limit
		g.state.hasHeaders[window] = true
		remainingGauge.WithLabelValues(g.decl.ProviderName(), window.String()).Set(float64(remaining))
	}

	updateWindow(Minute, parsed.remainingMinute, parsed.limitMinute)
	updateWindow(Day, parsed.remainingDay, parsed.limitDay)
}

func (g *Guard) cachedResponse(req *http.Request, body []byte) *http.Response {
	if g.decl.CacheTTL() <= 0 {
		return nil
	}
	key := cacheKey(req, body)
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, ok := g.cache[key]
	if !ok || time.Now().After(entry.expires) {
		return nil
	}
	return cloneResponse(req, entry.status, entry.header, entry.body)
}

func (g *Guard) maybeCacheResponse(req *http.Request, body []byte, resp *http.Response) (*http.Response, error) {
	if g.decl.CacheTTL() <= 0 {
		return resp, nil
	}
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	clone := cloneResponse(req, resp.StatusCode, resp.Header, buf)
	key := cacheKey(req, body)

	g.mu.Lock()
	g.cache[key] = cacheEntry{
		status:  resp.StatusCode,
		header:  clone.Header.Clone(),
		body:    buf,
		expires: time.Now().Add(g.decl.CacheTTL()),
	}
	g.mu.Unlock()

	return clone, nil
}

type parsedHeaders struct {
	limitMinute     int
	remainingMinute int
	limitDay        int
	remainingDay    int
	retryAfter      int
	resetAfter      int
}

func parseHeaders(h http.Header, cfg Headers) parsedHeaders {
	return parsedHeaders{
		limitMinute:     headerInt(h, cfg.LimitMinute),
		remainingMinute: headerInt(h, cfg.RemainingMinute),
		limitDay:        headerInt(h, cfg.LimitDay),
		remainingDay:    headerInt(h, cfg.RemainingDay),
		retryAfter:      headerInt(h, cfg.RetryAfter),
		resetAfter:      headerInt(h, cfg.ResetAfter),
	}
}

func headerInt(h http.Header, key string) int {
	if key == "" {
		return -1
	}
	val := h.Get(key)
	if val == "" {
		return -1
	}
	var out int
	if _, err := fmt.Sscanf(val, "%d", &out); err != nil {
		return -1
	}
	return out
}

func windowDuration(window Window) time.Duration {
	switch window {
	case Minute:
		return time.Minute
	case Day:
		return 24 * time.Hour
	default:
		return time.Minute
	}
}

func consumeToken(b *bucket, window time.Duration) bool {
	now := time.Now()
	if b.last.IsZero() {
		b.last = now
	}
	elapsed := now.Sub(b.last).Seconds()
	refillRate := float64(b.capacity) / window.Seconds()
	b.tokens = minFloat(float64(b.capacity), b.tokens+elapsed*refillRate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func drainBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func cacheKey(req *http.Request, body []byte) string {
	hash := sha256.Sum256(body)
	return req.Method + " " + req.URL.String() + " " + hex.EncodeToString(hash[:])
}

func cloneResponse(req *http.Request, status int, header http.Header, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
}
