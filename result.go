package herald

// Result represents either a successful value or an error.
// Used for stream-based message consumption where errors and values
// flow through the same channel.
type Result[T any] struct {
	value T
	err   error
}

// NewSuccess creates a successful Result containing the given value.
func NewSuccess[T any](value T) Result[T] {
	return Result[T]{value: value}
}

// NewError creates a failed Result containing the given error.
func NewError[T any](err error) Result[T] {
	return Result[T]{err: err}
}

// IsError returns true if this Result contains an error.
func (r Result[T]) IsError() bool {
	return r.err != nil
}

// IsSuccess returns true if this Result contains a successful value.
func (r Result[T]) IsSuccess() bool {
	return r.err == nil
}

// Value returns the successful value.
// Returns the zero value if this is an error Result.
func (r Result[T]) Value() T {
	return r.value
}

// Error returns the error, or nil if this is a successful Result.
func (r Result[T]) Error() error {
	return r.err
}
