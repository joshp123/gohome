package rate

import "time"

// Window represents a provider rate-limit bucket.
type Window int

const (
	Minute Window = iota
	Day
)

func (w Window) String() string {
	switch w {
	case Minute:
		return "minute"
	case Day:
		return "day"
	default:
		return "unknown"
	}
}

// Headers describes provider-specific rate limit headers.
type Headers struct {
	LimitMinute     string
	RemainingMinute string
	LimitDay        string
	RemainingDay    string
	RetryAfter      string
	ResetAfter      string
}

// StandardHeaders returns the default header mapping used by most providers.
func StandardHeaders() Headers {
	return Headers{
		LimitMinute:     "X-RateLimit-Limit-minute",
		RemainingMinute: "X-RateLimit-Remaining-minute",
		LimitDay:        "X-RateLimit-Limit-day",
		RemainingDay:    "X-RateLimit-Remaining-day",
		RetryAfter:      "Retry-After",
		ResetAfter:      "ratelimit-reset",
	}
}

// Declaration defines a provider's rate limits and header mapping.
type Declaration struct {
	provider    string
	limits      map[Window]int
	budgetFloor map[Window]int
	cacheTTL    time.Duration
	headers     Headers
	custom      CustomPolicy
}

// Provider creates a new declaration for a provider.
func Provider(name string) Declaration {
	return Declaration{provider: name}
}

func (d Declaration) ProviderName() string {
	return d.provider
}

func (d Declaration) MaxRequestsPer(window Window, limit int) Declaration {
	if d.limits == nil {
		d.limits = make(map[Window]int)
	}
	d.limits[window] = limit
	return d
}

func (d Declaration) BudgetFloor(window Window, floor int) Declaration {
	if d.budgetFloor == nil {
		d.budgetFloor = make(map[Window]int)
	}
	d.budgetFloor[window] = floor
	return d
}

func (d Declaration) CacheFor(ttl time.Duration) Declaration {
	d.cacheTTL = ttl
	return d
}

func (d Declaration) ReadHeaders(headers Headers) Declaration {
	d.headers = headers
	return d
}

func (d Declaration) Custom(policy CustomPolicy) Declaration {
	d.custom = policy
	return d
}

func (d Declaration) Limits() map[Window]int {
	return d.limits
}

func (d Declaration) BudgetFloors() map[Window]int {
	return d.budgetFloor
}

func (d Declaration) CacheTTL() time.Duration {
	return d.cacheTTL
}

func (d Declaration) Headers() Headers {
	return d.headers
}

func (d Declaration) CustomPolicy() CustomPolicy {
	return d.custom
}

func (d Declaration) HasLimits() bool {
	return len(d.limits) > 0
}

// RateLimited is the compile-time contract for plugins that declare limits.
type RateLimited interface {
	RateLimits() Declaration
}

// CustomPolicy allows plugins to override decision logic.
type CustomPolicy func(state *State, now time.Time) Decision
