package io

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestProvider_Publish(t *testing.T) {
	var buf bytes.Buffer
	provider := New(WithWriter(&buf))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "{\"test\":\"data\"}\n"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestProvider_PublishNoWriter(t *testing.T) {
	provider := New()

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil writer")
	}
}

func TestProvider_PublishCustomDelimiter(t *testing.T) {
	var buf bytes.Buffer
	provider := New(WithWriter(&buf), WithDelimiter('\x00'))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "{\"test\":\"data\"}\x00"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestProvider_Subscribe(t *testing.T) {
	input := "{\"order\":\"1\"}\n{\"order\":\"2\"}\n"
	reader := strings.NewReader(input)
	provider := New(WithReader(reader))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := provider.Subscribe(ctx)

	var received [][]byte
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch:
			if result.IsError() {
				t.Fatalf("unexpected error: %v", result.Error())
			}
			received = append(received, result.Value().Data)
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}

	if string(received[0]) != `{"order":"1"}` {
		t.Errorf("unexpected first message: %s", received[0])
	}

	if string(received[1]) != `{"order":"2"}` {
		t.Errorf("unexpected second message: %s", received[1])
	}
}

func TestProvider_SubscribeNoReader(t *testing.T) {
	provider := New()

	ctx := context.Background()
	ch := provider.Subscribe(ctx)

	// Should receive an error then close
	result, ok := <-ch
	if !ok {
		t.Fatal("expected error result before close")
	}
	if !result.IsError() {
		t.Error("expected error result")
	}

	// Channel should now be closed
	_, ok = <-ch
	if ok {
		t.Error("expected closed channel after error")
	}
}

func TestProvider_SubscribeCustomDelimiter(t *testing.T) {
	input := "{\"order\":\"1\"}\x00{\"order\":\"2\"}\x00"
	reader := strings.NewReader(input)
	provider := New(WithReader(reader), WithDelimiter('\x00'))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := provider.Subscribe(ctx)

	var received [][]byte
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch:
			if result.IsError() {
				t.Fatalf("unexpected error: %v", result.Error())
			}
			received = append(received, result.Value().Data)
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}

	if string(received[0]) != `{"order":"1"}` {
		t.Errorf("unexpected first message: %s", received[0])
	}
}

func TestProvider_SubscribeEOF(t *testing.T) {
	input := "{\"order\":\"1\"}\n"
	reader := strings.NewReader(input)
	provider := New(WithReader(reader))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := provider.Subscribe(ctx)

	// First message
	select {
	case result := <-ch:
		if result.IsError() {
			t.Fatalf("unexpected error: %v", result.Error())
		}
		if string(result.Value().Data) != `{"order":"1"}` {
			t.Errorf("unexpected message: %s", result.Value().Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	// Channel should close after EOF
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to close after EOF")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

func TestProvider_Close(t *testing.T) {
	provider := New()

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_AckNack(t *testing.T) {
	input := "{\"test\":\"data\"}\n"
	reader := strings.NewReader(input)
	provider := New(WithReader(reader))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := provider.Subscribe(ctx)

	select {
	case result := <-ch:
		if result.IsError() {
			t.Fatalf("unexpected error: %v", result.Error())
		}
		msg := result.Value()
		// Ack and Nack should be no-ops for io
		if err := msg.Ack(); err != nil {
			t.Errorf("unexpected ack error: %v", err)
		}
		if err := msg.Nack(); err != nil {
			t.Errorf("unexpected nack error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
