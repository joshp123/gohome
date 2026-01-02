# RFC: Core Rate Limits (Plugin‑Declared, Core‑Enforced)

- Date: 2026-01-02
- Status: Draft
- Audience: End users, operators, plugin authors

## 1) Narrative: what we are building and why

Plugins currently re‑implement rate‑limit handling (Daikin does caching + 429 cooldown; others do nothing).
This is brittle and inconsistent. We need **batteries‑included defaults** so that plugins declare their provider
limits once and the core handles pacing, cooldown, metrics, and cache TTLs.

This aligns with GoHome’s North Star: deterministic, reproducible, operator‑friendly behavior with minimal
runtime surprises.

## 1.1) Non‑negotiables

- No runtime config editing.
- Deterministic behavior only (no heuristics).
- Plugins **declare** provider‑specific details; core **enforces**.
- Single‑instance only (multi‑instance unsupported).

## 2) Goals / Non‑goals

Goals:
- Centralize rate‑limit logic and metrics.
- Reduce plugin boilerplate.
- Provide safe defaults when provider headers are missing.

Non‑goals:
- Multi‑instance coordination or distributed locks.
- Automatic provider discovery.
- UI for tuning limits.

## 3) System overview

Core provides a **rate‑limit guard** with built‑in, readable defaults.
Plugins optionally implement a `RateLimited` interface to declare **plain‑English limits**.
If no policy is declared, core does nothing. If a policy is declared **without explicit limits**, the plugin is blocked until limits are provided.

## 4) Components and responsibilities

- **Core**: policy enforcement, cooldown gating, shared metrics, and a shared HTTP wrapper helper (`rate.WrapHTTP`).
- **Plugins**: declare limits (data only) and wrap their HTTP client once with the helper. No guard calls. Core does not inject a global client.

## 5) Inputs / workflow profiles

Minimum input for rate‑limited plugins:
- A `rate.Declaration` in plugin code (plain‑English limits).
- (Optional) cache TTLs and header names for provider‑reported limits.

Validation rules:
- Policies are deterministic.
- Explicit limits are required (no implicit defaults).
- Unknown policy types must use the “custom” escape hatch explicitly.

## 6) Artifacts / outputs

- Standardized metrics (core emits):
  - `gohome_rate_limit_remaining{provider,window}`
  - `gohome_rate_limit_retry_after_seconds{provider}`
  - `gohome_rate_limit_last_status_code{provider}`

## 6.1) Glossary (plain English)

- `rate.Minute` / `rate.Day`: provider rate‑limit buckets, **not** wall‑clock schedules.
- “Budget”: remaining requests in a bucket.
- “Cooldown”: block calls until `Retry-After` expires.

## 7) State machine (if applicable)

Policy state is local to the process:
- `idle` → `cooldown` when `Retry-After` received
- `cooldown` → `idle` after expiry
- `idle` → `blocked` if budget floor reached (optional)

## 8) API surface (protobuf)

None. This is internal Go behavior.

## 9) Interaction model

Plugins **do not** call guard APIs.
Plugins wrap their HTTP client once via `rate.WrapHTTP`, and the wrapper enforces limits automatically.
Rate limits apply to **all HTTP methods** (GET/POST/PATCH/etc).

## 10) System interaction diagram

```
Plugin -> rate.WrapHTTP(...) -> HTTP client
  Wrapper checks policy
    if allowed:
       Wrapper -> Provider API
       Wrapper updates limits from response headers
    else:
       Wrapper returns cached data or a rate-limit error
```

## 11) API call flow (detailed)

1) Plugin uses the wrapped HTTP client.
2) Wrapper checks cooldown and budget floor.
3) Wrapper performs request if allowed.
4) Wrapper updates remaining, retry‑after, and metrics.

## 12) Determinism and validation

- All decisions are deterministic (header‑driven or fixed intervals).
- No heuristics or dynamic tuning.
- Compile‑time enforcement for plugins that opt in (see §12.1).

### 12.1) Compile‑time enforcement

Plugins that want core rate‑limit handling should **declare** it via interface and enforce at compile time:

```go
// rate/contract.go
type RateLimited interface {
  RateLimits() rate.Declaration
}

// plugin code: compile-time enforcement
var _ rate.RateLimited = (*Plugin)(nil)
```

If a plugin claims to be rate‑limited but does not implement the interface, it fails to compile.

## 13) Outputs and materialization

The guard’s only output is:
- shared metrics
- deterministic gating of API calls

### 13.1) Rate-limit error contract

When blocked, the wrapper returns a typed error:

```go
type RateLimitError struct {
  Provider string
  Reason   string // cooldown | budget | disabled
  RetryAt  time.Time
}
```

Plugins may surface this as a scrape error or fall back to cached data.

### 13.2) Cache behavior

Cache is **optional** per provider:
- If `CacheFor(...)` is set, the wrapper will serve stale data when blocked.
- If no cache is configured, blocked calls return `RateLimitError`.

### 13.3) Standard headers

`rate.StandardHeaders()` is defined as:
- `LimitMinute`: `X-RateLimit-Limit-minute`
- `RemainingMinute`: `X-RateLimit-Remaining-minute`
- `LimitDay`: `X-RateLimit-Limit-day`
- `RemainingDay`: `X-RateLimit-Remaining-day`
- `RetryAfter`: `Retry-After`
- `ResetAfter`: `ratelimit-reset`

## 14) Testing philosophy and trust

- Unit tests for policy correctness.
- Plugin tests cover policy integration via `guard.ShouldCall` + `RecordResponse`.

## 15) Incremental delivery plan

1) Add core rate‑limit package with policies and metrics.
2) Migrate Daikin to use core guard (first real provider).
3) Add optional policy declaration to other plugins.

## 16) Implementation order

1) `internal/rate` package (policy + guard + metrics).
2) `RateLimited` interface in core.
3) Daikin plugin migration.

## 16.1) Dependencies / sequencing

- Depends on: none (internal only).
- Blocks: none.
- Next: migrate additional plugins.

## 17) Examples (normal, weird, Daikin, full custom)

### 17.1) Normal provider with standard headers

```go
func (p Plugin) RateLimits() rate.Declaration {
  return rate.Provider("generic").
    MaxRequestsPer(rate.Minute, 60).
    MaxRequestsPer(rate.Day, 1000).
    CacheFor(5 * time.Minute).
    ReadHeaders(rate.StandardHeaders())
}
```

### 17.2) “Weird” provider (custom header names + budget floor)

```go
func (p Plugin) RateLimits() rate.Declaration {
  return rate.Provider("weirdco").
    MaxRequestsPer(rate.Minute, 10).
    BudgetFloor(rate.Minute, 3).
    CacheFor(2 * time.Minute).
    ReadHeaders(rate.Headers{
      LimitMinute:     "X-Rate-Min",
      RemainingMinute: "X-Rate-Remain-Min",
      ResetMinute:     "X-Rate-Reset-Min",
      RetryAfter:      "Retry-After",
    })
}
```

### 17.3) Daikin (current behavior, normalized)

```go
func (p Plugin) RateLimits() rate.Declaration {
  return rate.Provider("daikin").
    MaxRequestsPer(rate.Minute, 20).
    MaxRequestsPer(rate.Day, 200).
    CacheFor(10 * time.Minute).
    ReadHeaders(rate.Headers{
      LimitMinute:     "X-RateLimit-Limit-minute",
      RemainingMinute: "X-RateLimit-Remaining-minute",
      LimitDay:        "X-RateLimit-Limit-day",
      RemainingDay:    "X-RateLimit-Remaining-day",
      RetryAfter:      "Retry-After",
      ResetAfter:      "ratelimit-reset",
    })
}
```

### 17.4) Full custom (plugin‑owned logic)

```go
func (p Plugin) RateLimits() rate.Declaration {
  return rate.Custom("legacycorp", func(state *rate.State, now time.Time) rate.Decision {
    // custom deterministic rules
    if state.Remaining("day") < 2 {
      return rate.Blocked("daily budget depleted")
    }
    if state.CooldownActive(now) {
      return rate.Blocked("cooldown active")
    }
    return rate.Allowed()
  })
}
```

### 17.5) Full pseudocode plugin declaration

```go
package provider

type Plugin struct {
  client *Client
}

// Compile-time enforcement (opt-in)
var _ rate.RateLimited = (*Plugin)(nil)

func (p Plugin) RateLimits() rate.Declaration {
  return rate.Provider("provider").
    MaxRequestsPer(rate.Minute, 20).
    MaxRequestsPer(rate.Day, 200).
    CacheFor(5 * time.Minute).
    ReadHeaders(rate.StandardHeaders())
}

func (p Plugin) NewClient(cfg Config) (*Client, error) {
  httpClient := rate.WrapHTTP(p.RateLimits(), &http.Client{Timeout: 10 * time.Second})
  return &Client{http: httpClient}, nil
}
```

## 18) Brutal self‑review (required)

**Junior engineer:**  
- Was confused by “minute/day windows” and guard internals. Fixed by using `rate.Minute`/`rate.Day` and a wrapper helper (`rate.WrapHTTP`).  
- Needs concrete examples: four variants included (normal, weird, Daikin, full custom).

**Mid‑level engineer:**  
- Wanted header defaults and readable declarations. Added `rate.StandardHeaders()` and a plain‑English chain API.  
- Noted inconsistency between “core handles” vs “plugin wraps client.” Clarified that plugin wraps once; no guard calls.

**Senior/principal engineer:**  
- Wanted deterministic, non‑heuristic behavior. Policy is header‑driven or fixed interval only.  
- Compile‑time enforcement is explicit via `RateLimited` interface + `var _ rate.RateLimited`.

**PM:**  
- Scope is tight and does not introduce operator knobs or UI.  
- Clear incremental plan: build core guard, migrate Daikin, expand later.

**EM:**  
- Risk limited to internal package + one plugin migration.  
- No runtime config changes needed.

**External stakeholder:**  
- Value is clear: fewer 429s, consistent behavior, simpler plugins.

**End user:**  
- Not visible directly, but reduces outages and “mystery” rate‑limit failures.

**Remaining gaps to fix before acceptance:**  
**Closed** (glossary + error contract + cache behavior added).
