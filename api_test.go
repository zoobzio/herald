package herald

import (
	"context"
	"testing"
)

func TestContextWithMetadata(t *testing.T) {
	ctx := context.Background()
	m := Metadata{"key": "value", "trace-id": "abc123"}

	ctx = ContextWithMetadata(ctx, m)

	got := MetadataFromContext(ctx)
	if got == nil {
		t.Fatal("expected metadata, got nil")
	}
	if got["key"] != "value" {
		t.Errorf("expected key='value', got %q", got["key"])
	}
	if got["trace-id"] != "abc123" {
		t.Errorf("expected trace-id='abc123', got %q", got["trace-id"])
	}
}

func TestMetadataFromContext_Empty(t *testing.T) {
	ctx := context.Background()

	got := MetadataFromContext(ctx)
	if got != nil {
		t.Errorf("expected nil metadata from empty context, got %v", got)
	}
}

func TestContextWithMetadata_Nil(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithMetadata(ctx, nil)

	got := MetadataFromContext(ctx)
	if got != nil {
		t.Errorf("expected nil metadata when set to nil, got %v", got)
	}
}

func TestContextWithMetadata_Overwrites(t *testing.T) {
	ctx := context.Background()

	ctx = ContextWithMetadata(ctx, Metadata{"first": "1"})
	ctx = ContextWithMetadata(ctx, Metadata{"second": "2"})

	got := MetadataFromContext(ctx)
	if got == nil {
		t.Fatal("expected metadata, got nil")
	}
	if got["first"] != "" {
		t.Errorf("expected first to be overwritten, got %q", got["first"])
	}
	if got["second"] != "2" {
		t.Errorf("expected second='2', got %q", got["second"])
	}
}
