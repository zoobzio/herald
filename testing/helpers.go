// Package testing provides test utilities and helpers for herald users.
// These utilities help users test their own herald-based applications.
package testing

import (
	"context"
	"sync"
	"time"

	"github.com/zoobzio/herald"
)

// MockProvider is a test provider for verifying publish/subscribe behavior.
// Thread-safe for concurrent use in tests.
type MockProvider struct {
	mu        sync.Mutex
	published []PublishedMessage
	subCh     chan herald.Result[herald.Message]
	closed    bool
	onPublish func(data []byte, metadata herald.Metadata)
}

// PublishedMessage represents a message that was published.
type PublishedMessage struct {
	Data     []byte
	Metadata herald.Metadata
}

// NewMockProvider creates a new MockProvider for testing.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		published: make([]PublishedMessage, 0),
	}
}

// WithSubscribeChannel sets a channel for delivering messages during Subscribe.
func (m *MockProvider) WithSubscribeChannel(ch chan herald.Result[herald.Message]) *MockProvider {
	m.subCh = ch
	return m
}

// WithPublishCallback sets a callback to be invoked on each Publish call.
func (m *MockProvider) WithPublishCallback(fn func(data []byte, metadata herald.Metadata)) *MockProvider {
	m.onPublish = fn
	return m
}

// Publish records the published message.
func (m *MockProvider) Publish(_ context.Context, data []byte, metadata herald.Metadata) error {
	m.mu.Lock()
	m.published = append(m.published, PublishedMessage{
		Data:     data,
		Metadata: metadata,
	})
	onPublish := m.onPublish
	m.mu.Unlock()

	if onPublish != nil {
		onPublish(data, metadata)
	}
	return nil
}

// Subscribe returns the configured channel or a closed channel if none set.
func (m *MockProvider) Subscribe(_ context.Context) <-chan herald.Result[herald.Message] {
	if m.subCh != nil {
		return m.subCh
	}
	ch := make(chan herald.Result[herald.Message])
	close(ch)
	return ch
}

// Ping verifies mock provider connectivity.
func (m *MockProvider) Ping(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return herald.ErrNoWriter
	}
	return nil
}

// Close marks the provider as closed.
func (m *MockProvider) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// Published returns a copy of all published messages.
func (m *MockProvider) Published() []PublishedMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]PublishedMessage, len(m.published))
	copy(result, m.published)
	return result
}

// PublishCount returns the number of published messages.
func (m *MockProvider) PublishCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.published)
}

// IsClosed returns whether Close has been called.
func (m *MockProvider) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// Reset clears all published messages.
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = m.published[:0]
}

// NewTestMessage creates a Message with configurable ack/nack for testing.
func NewTestMessage(data []byte, metadata herald.Metadata) herald.Message {
	return herald.Message{
		Data:     data,
		Metadata: metadata,
		Ack:      func() error { return nil },
		Nack:     func() error { return nil },
	}
}

// NewTestMessageWithAck creates a Message with tracking ack/nack functions.
func NewTestMessageWithAck(data []byte, metadata herald.Metadata, onAck, onNack func()) herald.Message {
	return herald.Message{
		Data:     data,
		Metadata: metadata,
		Ack: func() error {
			if onAck != nil {
				onAck()
			}
			return nil
		},
		Nack: func() error {
			if onNack != nil {
				onNack()
			}
			return nil
		},
	}
}

// MessageCapture captures messages for testing and verification.
// Thread-safe for concurrent capture.
type MessageCapture struct {
	messages []herald.Message
	mu       sync.Mutex
}

// NewMessageCapture creates a new MessageCapture instance.
func NewMessageCapture() *MessageCapture {
	return &MessageCapture{
		messages: make([]herald.Message, 0),
	}
}

// Capture adds a message to the capture.
func (mc *MessageCapture) Capture(msg herald.Message) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.messages = append(mc.messages, msg)
}

// Messages returns a copy of all captured messages.
func (mc *MessageCapture) Messages() []herald.Message {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	result := make([]herald.Message, len(mc.messages))
	copy(result, mc.messages)
	return result
}

// Count returns the number of captured messages.
func (mc *MessageCapture) Count() int {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return len(mc.messages)
}

// Reset clears all captured messages.
func (mc *MessageCapture) Reset() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.messages = mc.messages[:0]
}

// WaitForCount blocks until the capture has at least n messages or timeout occurs.
// Returns true if count reached, false if timeout.
func (mc *MessageCapture) WaitForCount(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if mc.Count() >= n {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

// ErrorCapture captures herald errors for testing.
type ErrorCapture struct {
	errors []herald.Error
	mu     sync.Mutex
}

// NewErrorCapture creates a new ErrorCapture instance.
func NewErrorCapture() *ErrorCapture {
	return &ErrorCapture{
		errors: make([]herald.Error, 0),
	}
}

// Capture adds an error to the capture.
func (ec *ErrorCapture) Capture(err herald.Error) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.errors = append(ec.errors, err)
}

// Errors returns a copy of all captured errors.
func (ec *ErrorCapture) Errors() []herald.Error {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	result := make([]herald.Error, len(ec.errors))
	copy(result, ec.errors)
	return result
}

// Count returns the number of captured errors.
func (ec *ErrorCapture) Count() int {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return len(ec.errors)
}

// Reset clears all captured errors.
func (ec *ErrorCapture) Reset() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.errors = ec.errors[:0]
}

// WaitForCount blocks until the capture has at least n errors or timeout occurs.
func (ec *ErrorCapture) WaitForCount(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ec.Count() >= n {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}
