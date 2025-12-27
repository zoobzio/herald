package herald

import (
	"context"
	"time"

	"github.com/zoobzio/pipz"
)

// Internal identities for middleware processors.
var (
	middlewareID          = pipz.NewIdentity("herald:middleware", "Middleware sequence")
	middlewareRetryID     = pipz.NewIdentity("herald:middleware:retry", "Middleware retry")
	middlewareBackoffID   = pipz.NewIdentity("herald:middleware:backoff", "Middleware backoff")
	middlewareTimeoutID   = pipz.NewIdentity("herald:middleware:timeout", "Middleware timeout")
	middlewareFallbackID  = pipz.NewIdentity("herald:middleware:fallback", "Middleware fallback")
	middlewareRateLimitID = pipz.NewIdentity("herald:middleware:rate-limit", "Middleware rate limiting")
)

// -----------------------------------------------------------------------------
// Pipeline Options (With*)
// -----------------------------------------------------------------------------
// These wrap the entire pipeline with additional behavior.

// WithFilter wraps the pipeline with a condition.
// If the condition returns false, the pipeline is skipped.
func WithFilter[T any](identity pipz.Identity, condition func(context.Context, *Envelope[T]) bool) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		return pipz.NewFilter(identity, condition, pipeline)
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
//	            herald.UseEffect[Event](logID, logFn),
//	            herald.UseApply[Event](validateID, validateFn),
//	        ),
//	    },
//	)
func WithMiddleware[T any](processors ...pipz.Chainable[*Envelope[T]]) Option[T] {
	return func(pipeline pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
		processors = append(processors, pipeline)
		return pipz.NewSequence(middlewareID, processors...)
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
func UseTransform[T any](identity pipz.Identity, fn func(context.Context, *Envelope[T]) *Envelope[T]) pipz.Chainable[*Envelope[T]] {
	return pipz.Transform(identity, fn)
}

// UseApply creates a processor that can transform the envelope and fail.
// Use for operations like enrichment, validation, or transformation
// that may produce errors.
func UseApply[T any](identity pipz.Identity, fn func(context.Context, *Envelope[T]) (*Envelope[T], error)) pipz.Chainable[*Envelope[T]] {
	return pipz.Apply(identity, fn)
}

// UseEffect creates a processor that performs a side effect.
// The envelope passes through unchanged. Use for logging, metrics,
// or notifications that should not affect the value.
func UseEffect[T any](identity pipz.Identity, fn func(context.Context, *Envelope[T]) error) pipz.Chainable[*Envelope[T]] {
	return pipz.Effect(identity, fn)
}

// UseMutate creates a processor that conditionally transforms the envelope.
// The transformer is only applied if the condition returns true.
func UseMutate[T any](identity pipz.Identity, transformer func(context.Context, *Envelope[T]) *Envelope[T], condition func(context.Context, *Envelope[T]) bool) pipz.Chainable[*Envelope[T]] {
	return pipz.Mutate(identity, transformer, condition)
}

// UseEnrich creates a processor that attempts optional enhancement.
// If the enrichment fails, processing continues with the original envelope.
func UseEnrich[T any](identity pipz.Identity, fn func(context.Context, *Envelope[T]) (*Envelope[T], error)) pipz.Chainable[*Envelope[T]] {
	return pipz.Enrich(identity, fn)
}

// UseRetry wraps a processor with retry logic.
// Failed operations are retried immediately up to maxAttempts times.
func UseRetry[T any](maxAttempts int, processor pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	return pipz.NewRetry(middlewareRetryID, processor, maxAttempts)
}

// UseBackoff wraps a processor with exponential backoff retry logic.
// Failed operations are retried with increasing delays.
func UseBackoff[T any](maxAttempts int, baseDelay time.Duration, processor pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	return pipz.NewBackoff(middlewareBackoffID, processor, maxAttempts, baseDelay)
}

// UseTimeout wraps a processor with a deadline.
// If processing takes longer than the specified duration, the operation fails.
func UseTimeout[T any](d time.Duration, processor pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	return pipz.NewTimeout(middlewareTimeoutID, processor, d)
}

// UseFallback wraps a processor with fallback alternatives.
// If the primary fails, each fallback is tried in order.
func UseFallback[T any](primary pipz.Chainable[*Envelope[T]], fallbacks ...pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	all := append([]pipz.Chainable[*Envelope[T]]{primary}, fallbacks...)
	return pipz.NewFallback(middlewareFallbackID, all...)
}

// UseFilter wraps a processor with a condition.
// If the condition returns false, the envelope passes through unchanged.
func UseFilter[T any](identity pipz.Identity, condition func(context.Context, *Envelope[T]) bool, processor pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	return pipz.NewFilter(identity, condition, processor)
}

// UseRateLimit wraps a processor with rate limiting.
// Uses a token bucket algorithm with the specified rate (tokens per second)
// and burst size.
func UseRateLimit[T any](rate float64, burst int, processor pipz.Chainable[*Envelope[T]]) pipz.Chainable[*Envelope[T]] {
	return pipz.NewRateLimiter(middlewareRateLimitID, rate, burst, processor)
}
