package herald

import (
	"time"

	"github.com/zoobzio/pipz"
)

// Internal identities for reliability options.
var (
	retryID          = pipz.NewIdentity("herald:retry", "Retries failed operations")
	backoffID        = pipz.NewIdentity("herald:backoff", "Retries with exponential backoff")
	timeoutID        = pipz.NewIdentity("herald:timeout", "Enforces operation timeout")
	circuitBreakerID = pipz.NewIdentity("herald:circuit-breaker", "Circuit breaker protection")
	rateLimitID      = pipz.NewIdentity("herald:rate-limit", "Rate limiting")
	errorHandlerID   = pipz.NewIdentity("herald:error-handler", "Error handling")
	fallbackID       = pipz.NewIdentity("herald:fallback", "Fallback alternatives")
)

// Option modifies a pipeline for reliability features.
// Options wrap the terminal operation with additional behavior.
type Option[T any] func(pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]]

// WithRetry adds retry logic to the pipeline.
// Failed operations are retried up to maxAttempts times immediately.
func WithRetry[T any](maxAttempts int) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		return pipz.NewRetry(retryID, pipeline, maxAttempts)
	}
}

// WithBackoff adds retry logic with exponential backoff to the pipeline.
// Failed operations are retried with increasing delays between attempts.
// The delay starts at baseDelay and doubles after each failure.
func WithBackoff[T any](maxAttempts int, baseDelay time.Duration) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		return pipz.NewBackoff(backoffID, pipeline, maxAttempts, baseDelay)
	}
}

// WithTimeout adds timeout protection to the pipeline.
// Operations exceeding this duration will be canceled.
func WithTimeout[T any](duration time.Duration) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		return pipz.NewTimeout(timeoutID, pipeline, duration)
	}
}

// WithCircuitBreaker adds circuit breaker protection to the pipeline.
// After 'failures' consecutive failures, the circuit opens for 'recovery' duration.
func WithCircuitBreaker[T any](failures int, recovery time.Duration) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		return pipz.NewCircuitBreaker(circuitBreakerID, pipeline, failures, recovery)
	}
}

// WithRateLimit adds rate limiting to the pipeline.
// rate = operations per second, burst = burst capacity.
func WithRateLimit[T any](rate float64, burst int) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		return pipz.NewRateLimiter(rateLimitID, rate, burst, pipeline)
	}
}

// WithErrorHandler adds error handling to the pipeline.
// The error handler receives error context and can process/log/alert as needed.
func WithErrorHandler[T any](handler pipz.Chainable[*pipz.Error[*Envelope[T]]]) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		return pipz.NewHandle(errorHandlerID, pipeline, handler)
	}
}

// WithFallback wraps the pipeline with fallback alternatives.
// If the primary pipeline fails, each fallback is tried in order.
// Useful for broker failover scenarios.
func WithFallback[T any](fallbacks ...pipz.Chainable[*Envelope[T]]) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		all := append([]pipz.Chainable[*Envelope[T]]{pipeline}, fallbacks...)
		return pipz.NewFallback(fallbackID, all...)
	}
}

// WithPipeline allows full control over the processing pipeline.
// Use this for advanced composition beyond the provided options.
// The provided pipeline replaces any default processing.
func WithPipeline[T any](custom pipz.Chainable[*Envelope[T]]) Option[T] {
	return func(_ pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		return custom
	}
}
