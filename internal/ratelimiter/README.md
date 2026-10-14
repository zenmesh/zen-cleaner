# Rate Limiter Package

Shared rate limiting primitives for cleanup operations, extracted from multiple controllers.

## Usage

```go
import "github.com/zenmesh/zen-cleaner/internal/pkg/gc/ratelimiter"

// Create rate limiter (10 operations per second)
rl := ratelimiter.NewRateLimiter(10)

// Wait for next operation (blocks until allowed)
if err := rl.Wait(ctx); err != nil {
    return err
}

// Or check without waiting
if !rl.Allow() {
    return errors.New("rate limit exceeded")
}

// Update rate dynamically
rl.SetRate(20) // 20 ops/sec
```

## Implementation

Uses `golang.org/x/time/rate` (token bucket algorithm) for efficient rate limiting.

## Migration

**From zen-cleaner**:
```go
// Before
import "github.com/zenmesh/zen-cleaner/pkg/controller"
rl := controller.NewRateLimiter(maxPerSecond)

// After
import "github.com/zenmesh/zen-cleaner/internal/pkg/gc/ratelimiter"
rl := ratelimiter.NewRateLimiter(maxPerSecond)
```

**From a token-bucket implementation**:
```go
// Before
rl := server.NewRateLimiter(maxTokens, refillInterval)

// After
import "github.com/zenmesh/zen-cleaner/internal/pkg/gc/ratelimiter"
rl := ratelimiter.NewRateLimiter(maxPerSecond)
```

