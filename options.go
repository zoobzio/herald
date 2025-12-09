package herald

import (
	"time"

	"github.com/zoobzio/pipz"
)

// Option modifies a pipeline for reliability features.
// Options wrap the terminal operation with additional behavior.
type Option[T any] func(pipz.Chainable[T]) pipz.Chainable[T]

// WithRetry adds retry logic to the pipeline.
// Failed operations are retried up to maxAttempts times immediately.
func WithRetry[T any](maxAttempts int) Option[T] {
	return func(pipeline pipz.Chainable[T]) pipz.Chainable[T] {
		return pipz.NewRetry("retry", pipeline, maxAttempts)
	}
}

// WithBackoff adds retry logic with exponential backoff to the pipeline.
// Failed operations are retried with increasing delays between attempts.
// The delay starts at baseDelay and doubles after each failure.
func WithBackoff[T any](maxAttempts int, baseDelay time.Duration) Option[T] {
	return func(pipeline pipz.Chainable[T]) pipz.Chainable[T] {
		return pipz.NewBackoff("backoff", pipeline, maxAttempts, baseDelay)
	}
}

// WithTimeout adds timeout protection to the pipeline.
// Operations exceeding this duration will be canceled.
func WithTimeout[T any](duration time.Duration) Option[T] {
	return func(pipeline pipz.Chainable[T]) pipz.Chainable[T] {
		return pipz.NewTimeout("timeout", pipeline, duration)
	}
}

// WithCircuitBreaker adds circuit breaker protection to the pipeline.
// After 'failures' consecutive failures, the circuit opens for 'recovery' duration.
func WithCircuitBreaker[T any](failures int, recovery time.Duration) Option[T] {
	return func(pipeline pipz.Chainable[T]) pipz.Chainable[T] {
		return pipz.NewCircuitBreaker("circuit-breaker", pipeline, failures, recovery)
	}
}

// WithRateLimit adds rate limiting to the pipeline.
// rate = operations per second, burst = burst capacity.
func WithRateLimit[T any](rate float64, burst int) Option[T] {
	return func(pipeline pipz.Chainable[T]) pipz.Chainable[T] {
		rateLimiter := pipz.NewRateLimiter[T]("rate-limit", rate, burst)
		return pipz.NewSequence("rate-limited", rateLimiter, pipeline)
	}
}

// WithErrorHandler adds error handling to the pipeline.
// The error handler receives error context and can process/log/alert as needed.
func WithErrorHandler[T any](handler pipz.Chainable[*pipz.Error[T]]) Option[T] {
	return func(pipeline pipz.Chainable[T]) pipz.Chainable[T] {
		return pipz.NewHandle("error-handler", pipeline, handler)
	}
}

// WithPipeline allows full control over the processing pipeline.
// Use this for advanced composition beyond the provided options.
// The provided pipeline replaces any default processing.
func WithPipeline[T any](custom pipz.Chainable[T]) Option[T] {
	return func(_ pipz.Chainable[T]) pipz.Chainable[T] {
		return custom
	}
}
