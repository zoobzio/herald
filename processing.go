package herald

import (
	"context"

	"github.com/zoobzio/pipz"
)

// WithApply wraps a function that transforms data and may fail.
// Use for validation, enrichment, or any operation that can error.
// Return an error to abort processing.
func WithApply[T any](name string, fn func(context.Context, T) (T, error)) Option[T] {
	return func(pipeline pipz.Chainable[T]) pipz.Chainable[T] {
		return pipz.NewSequence(pipz.Name(name), pipz.Apply(pipz.Name(name), fn), pipeline)
	}
}

// WithEffect wraps a function that performs side effects without modifying data.
// Use for logging, metrics, or notifications.
// Return an error to abort processing.
func WithEffect[T any](name string, fn func(context.Context, T) error) Option[T] {
	return func(pipeline pipz.Chainable[T]) pipz.Chainable[T] {
		return pipz.NewSequence(pipz.Name(name), pipz.Effect(pipz.Name(name), fn), pipeline)
	}
}

// WithTransform wraps a pure transformation function that cannot fail.
// Use for data formatting, field mapping, or computed fields.
func WithTransform[T any](name string, fn func(context.Context, T) T) Option[T] {
	return func(pipeline pipz.Chainable[T]) pipz.Chainable[T] {
		return pipz.NewSequence(pipz.Name(name), pipz.Transform(pipz.Name(name), fn), pipeline)
	}
}
