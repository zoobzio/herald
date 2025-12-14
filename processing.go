package herald

import (
	"context"
	"time"

	"github.com/zoobzio/pipz"
)

// -----------------------------------------------------------------------------
// Pipeline Options (With*)
// -----------------------------------------------------------------------------
// These wrap the entire pipeline with additional behavior.

// WithFilter wraps the pipeline with a condition.
// If the condition returns false, the pipeline is skipped.
func WithFilter[T any](name string, condition func(context.Context, *Envelope[T]) bool) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		return pipz.NewFilter(pipz.Name(name), condition, pipeline)
	}
}

// WithMiddleware wraps the pipeline with a sequence of processors.
// Processors execute in order, with the wrapped pipeline last.
//
// Example:
//
//	herald.NewPublisher[Event](
//	    provider, signal, key,
//	    []herald.Option[Event]{
//	        herald.WithMiddleware(
//	            herald.UseEffect[Event]("log", logFn),
//	            herald.UseApply[Event]("validate", validateFn),
//	        ),
//	    },
//	)
func WithMiddleware[T any](processors ...pipz.Chainable[*Envelope[T]]) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		processors = append(processors, pipeline)
		return pipz.NewSequence("middleware", processors...)
	}
}

// -----------------------------------------------------------------------------
// Middleware Processors (Use*)
// -----------------------------------------------------------------------------
// These create processors for use inside WithMiddleware.
// They can be nested and combined to build complex processing pipelines.
// All processors operate on *Envelope[T], providing access to both Value and Metadata.

// UseTransform creates a processor that transforms the envelope.
// Cannot fail. Use for pure transformations that always succeed.
func UseTransform[T any](name string, fn func(context.Context, *Envelope[T]) *Envelope[T]) pipz.Chainable[*Envelope[T]] {
	return pipz.Transform(pipz.Name(name), fn)
}

// UseApply creates a processor that can transform the envelope and fail.
// Use for operations like enrichment, validation, or transformation
// that may produce errors.
func UseApply[T any](name string, fn func(context.Context, *Envelope[T]) (*Envelope[T], error)) pipz.Chainable[*Envelope[T]] {
	return pipz.Apply(pipz.Name(name), fn)
}

// UseEffect creates a processor that performs a side effect.
// The envelope passes through unchanged. Use for logging, metrics,
// or notifications that should not affect the value.
func UseEffect[T any](name string, fn func(context.Context, *Envelope[T]) error) pipz.Chainable[*Envelope[T]] {
	return pipz.Effect(pipz.Name(name), fn)
}

// UseMutate creates a processor that conditionally transforms the envelope.
// The transformer is only applied if the condition returns true.
func UseMutate[T any](name string, transformer func(context.Context, *Envelope[T]) *Envelope[T], condition func(context.Context, *Envelope[T]) bool) pipz.Chainable[*Envelope[T]] {
	return pipz.Mutate(pipz.Name(name), transformer, condition)
}

// UseEnrich creates a processor that attempts optional enhancement.
// If the enrichment fails, processing continues with the original envelope.
func UseEnrich[T any](name string, fn func(context.Context, *Envelope[T]) (*Envelope[T], error)) pipz.Chainable[*Envelope[T]] {
	return pipz.Enrich(pipz.Name(name), fn)
}

// UseRetry wraps a processor with retry logic.
// Failed operations are retried immediately up to maxAttempts times.
func UseRetry[T any](maxAttempts int, processor pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	return pipz.NewRetry("retry", processor, maxAttempts)
}

// UseBackoff wraps a processor with exponential backoff retry logic.
// Failed operations are retried with increasing delays.
func UseBackoff[T any](maxAttempts int, baseDelay time.Duration, processor pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	return pipz.NewBackoff("backoff", processor, maxAttempts, baseDelay)
}

// UseTimeout wraps a processor with a deadline.
// If processing takes longer than the specified duration, the operation fails.
func UseTimeout[T any](d time.Duration, processor pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	return pipz.NewTimeout("timeout", processor, d)
}

// UseFallback wraps a processor with fallback alternatives.
// If the primary fails, each fallback is tried in order.
func UseFallback[T any](primary pipz.Chainable[*Envelope[T]], fallbacks ...pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	all := append([]pipz.Chainable[*Envelope[T]]{primary}, fallbacks...)
	return pipz.NewFallback("fallback", all...)
}

// UseFilter wraps a processor with a condition.
// If the condition returns false, the envelope passes through unchanged.
func UseFilter[T any](name string, condition func(context.Context, *Envelope[T]) bool, processor pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	return pipz.NewFilter(pipz.Name(name), condition, processor)
}

// UseRateLimit creates a rate limiting processor.
// Uses a token bucket algorithm with the specified rate (tokens per second)
// and burst size.
func UseRateLimit[T any](rate float64, burst int) pipz.Chainable[*Envelope[T]] {
	return pipz.NewRateLimiter[*Envelope[T]]("rate-limiter", rate, burst)
}
