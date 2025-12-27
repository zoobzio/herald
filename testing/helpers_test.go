package testing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/herald"
)

func TestMockProvider_Publish(t *testing.T) {
	provider := NewMockProvider()

	err := provider.Publish(context.Background(), []byte("test data"), herald.Metadata{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.PublishCount() != 1 {
		t.Errorf("expected 1 published message, got %d", provider.PublishCount())
	}

	published := provider.Published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}

	if string(published[0].Data) != "test data" {
		t.Errorf("unexpected data: %s", published[0].Data)
	}
	if published[0].Metadata["key"] != "value" {
		t.Errorf("unexpected metadata: %v", published[0].Metadata)
	}
}

func TestMockProvider_PublishCallback(t *testing.T) {
	var callbackData []byte
	var callbackMeta herald.Metadata

	provider := NewMockProvider().WithPublishCallback(func(data []byte, metadata herald.Metadata) {
		callbackData = data
		callbackMeta = metadata
	})

	err := provider.Publish(context.Background(), []byte("callback test"), herald.Metadata{"cb": "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(callbackData) != "callback test" {
		t.Errorf("callback data mismatch: %s", callbackData)
	}
	if callbackMeta["cb"] != "true" {
		t.Errorf("callback metadata mismatch: %v", callbackMeta)
	}
}

func TestMockProvider_Subscribe(t *testing.T) {
	ch := make(chan herald.Result[herald.Message], 1)
	provider := NewMockProvider().WithSubscribeChannel(ch)

	subCh := provider.Subscribe(context.Background())
	if subCh != ch {
		t.Error("expected same channel")
	}
}

func TestMockProvider_SubscribeDefault(t *testing.T) {
	provider := NewMockProvider()

	ch := provider.Subscribe(context.Background())

	// Should be closed immediately
	_, ok := <-ch
	if ok {
		t.Error("expected closed channel")
	}
}

func TestMockProvider_Close(t *testing.T) {
	provider := NewMockProvider()

	if provider.IsClosed() {
		t.Error("expected not closed initially")
	}

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !provider.IsClosed() {
		t.Error("expected closed after Close()")
	}
}

func TestMockProvider_Ping(t *testing.T) {
	provider := NewMockProvider()

	// Should succeed when open
	err := provider.Ping(context.Background())
	if err != nil {
		t.Errorf("expected no error on open provider, got %v", err)
	}

	// Close and try again
	_ = provider.Close()

	err = provider.Ping(context.Background())
	if !errors.Is(err, herald.ErrNoWriter) {
		t.Errorf("expected ErrNoWriter on closed provider, got %v", err)
	}
}

func TestMockProvider_Reset(t *testing.T) {
	provider := NewMockProvider()

	_ = provider.Publish(context.Background(), []byte("data"), nil)
	_ = provider.Publish(context.Background(), []byte("data"), nil)

	if provider.PublishCount() != 2 {
		t.Errorf("expected 2 messages, got %d", provider.PublishCount())
	}

	provider.Reset()

	if provider.PublishCount() != 0 {
		t.Errorf("expected 0 messages after reset, got %d", provider.PublishCount())
	}
}

func TestMockProvider_Concurrent(t *testing.T) {
	provider := NewMockProvider()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = provider.Publish(context.Background(), []byte("concurrent"), nil)
		}()
	}
	wg.Wait()

	if provider.PublishCount() != 100 {
		t.Errorf("expected 100 messages, got %d", provider.PublishCount())
	}
}

func TestNewTestMessage(t *testing.T) {
	msg := NewTestMessage([]byte("test"), herald.Metadata{"key": "value"})

	if string(msg.Data) != "test" {
		t.Errorf("unexpected data: %s", msg.Data)
	}
	if msg.Metadata["key"] != "value" {
		t.Errorf("unexpected metadata: %v", msg.Metadata)
	}

	// Ack/Nack should not panic
	if err := msg.Ack(); err != nil {
		t.Errorf("unexpected ack error: %v", err)
	}
	if err := msg.Nack(); err != nil {
		t.Errorf("unexpected nack error: %v", err)
	}
}

func TestNewTestMessageWithAck(t *testing.T) {
	var acked, nacked bool

	msg := NewTestMessageWithAck(
		[]byte("test"),
		nil,
		func() { acked = true },
		func() { nacked = true },
	)

	_ = msg.Ack()
	if !acked {
		t.Error("expected ack callback to be called")
	}

	_ = msg.Nack()
	if !nacked {
		t.Error("expected nack callback to be called")
	}
}

func TestMessageCapture(t *testing.T) {
	capture := NewMessageCapture()

	if capture.Count() != 0 {
		t.Errorf("expected 0 messages initially, got %d", capture.Count())
	}

	capture.Capture(NewTestMessage([]byte("msg1"), nil))
	capture.Capture(NewTestMessage([]byte("msg2"), nil))

	if capture.Count() != 2 {
		t.Errorf("expected 2 messages, got %d", capture.Count())
	}

	messages := capture.Messages()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	if string(messages[0].Data) != "msg1" {
		t.Errorf("unexpected first message: %s", messages[0].Data)
	}

	capture.Reset()
	if capture.Count() != 0 {
		t.Errorf("expected 0 messages after reset, got %d", capture.Count())
	}
}

func TestMessageCapture_WaitForCount(t *testing.T) {
	capture := NewMessageCapture()

	// Should timeout
	if capture.WaitForCount(1, 10*time.Millisecond) {
		t.Error("expected timeout")
	}

	// Add message in goroutine
	go func() {
		time.Sleep(5 * time.Millisecond)
		capture.Capture(NewTestMessage([]byte("delayed"), nil))
	}()

	// Should succeed
	if !capture.WaitForCount(1, 100*time.Millisecond) {
		t.Error("expected success")
	}
}

func TestErrorCapture(t *testing.T) {
	capture := NewErrorCapture()

	if capture.Count() != 0 {
		t.Errorf("expected 0 errors initially, got %d", capture.Count())
	}

	capture.Capture(herald.Error{Operation: "publish", Signal: "test", Err: "error1"})
	capture.Capture(herald.Error{Operation: "subscribe", Signal: "test", Err: "error2"})

	if capture.Count() != 2 {
		t.Errorf("expected 2 errors, got %d", capture.Count())
	}

	errs := capture.Errors()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}

	if errs[0].Operation != "publish" {
		t.Errorf("unexpected first error: %s", errs[0].Operation)
	}

	capture.Reset()
	if capture.Count() != 0 {
		t.Errorf("expected 0 errors after reset, got %d", capture.Count())
	}
}

func TestErrorCapture_WaitForCount(t *testing.T) {
	capture := NewErrorCapture()

	// Should timeout
	if capture.WaitForCount(1, 10*time.Millisecond) {
		t.Error("expected timeout")
	}

	// Add error in goroutine
	go func() {
		time.Sleep(5 * time.Millisecond)
		capture.Capture(herald.Error{Operation: "test", Err: "delayed"})
	}()

	// Should succeed
	if !capture.WaitForCount(1, 100*time.Millisecond) {
		t.Error("expected success")
	}
}
