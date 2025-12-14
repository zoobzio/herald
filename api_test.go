package herald

import (
	"context"
	"testing"
)

func TestResult_IsSuccess(t *testing.T) {
	success := NewSuccess("value")
	if !success.IsSuccess() {
		t.Error("expected IsSuccess() to return true for success result")
	}
	if success.IsError() {
		t.Error("expected IsError() to return false for success result")
	}

	err := NewError[string](context.DeadlineExceeded)
	if err.IsSuccess() {
		t.Error("expected IsSuccess() to return false for error result")
	}
	if !err.IsError() {
		t.Error("expected IsError() to return true for error result")
	}
}
