package sql

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type mockDB struct {
	mu       sync.Mutex
	messages []Message
	inserted []struct {
		topic string
		data  []byte
	}
	deleted   []string
	released  []string
	fetchErr  error
	insertErr error
	pingErr   error
	closed    bool
	nextID    int
}

func (m *mockDB) Insert(ctx context.Context, topic string, data []byte, metadata map[string]string) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	m.inserted = append(m.inserted, struct {
		topic string
		data  []byte
	}{topic, data})
	m.messages = append(m.messages, Message{ID: string(rune(m.nextID)), Data: data})
	return nil
}

func (m *mockDB) Fetch(_ context.Context, _ string, limit int, _ time.Duration) ([]Message, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		return nil, nil
	}
	n := limit
	if n > len(m.messages) {
		n = len(m.messages)
	}
	result := m.messages[:n]
	return result, nil
}

func (m *mockDB) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, id)
	// Remove from messages
	for i, msg := range m.messages {
		if msg.ID == id {
			m.messages = append(m.messages[:i], m.messages[i+1:]...)
			break
		}
	}
	return nil
}

func (m *mockDB) Release(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.released = append(m.released, id)
	return nil
}

func (m *mockDB) Ping(_ context.Context) error {
	return m.pingErr
}

func (m *mockDB) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockDB) Inserted() []struct {
	topic string
	data  []byte
} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inserted
}

func (m *mockDB) Deleted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleted
}

func (m *mockDB) Released() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.released
}

func TestProvider_Publish(t *testing.T) {
	db := &mockDB{}
	provider := New("test-topic", WithDB(db))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inserted := db.Inserted()
	if len(inserted) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(inserted))
	}

	if inserted[0].topic != "test-topic" {
		t.Errorf("unexpected topic: %s", inserted[0].topic)
	}

	if string(inserted[0].data) != `{"test":"data"}` {
		t.Errorf("unexpected data: %s", inserted[0].data)
	}
}

func TestProvider_PublishNoDB(t *testing.T) {
	provider := New("test-topic")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil db")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	db := &mockDB{
		messages: []Message{
			{ID: "1", Data: []byte(`{"order":"1"}`)},
			{ID: "2", Data: []byte(`{"order":"2"}`)},
		},
	}
	provider := New("test-topic", WithDB(db), WithPollInterval(10*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	ch := provider.Subscribe(ctx)

	var received [][]byte
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch:
			if result.IsError() {
				t.Fatalf("unexpected error: %v", result.Error())
			}
			received = append(received, result.Value().Data)
			// Ack to remove from queue
			result.Value().Ack()
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}

	cancel()
	provider.Close()

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}

	if string(received[0]) != `{"order":"1"}` {
		t.Errorf("unexpected first message: %s", received[0])
	}
}

func TestProvider_SubscribeNoDB(t *testing.T) {
	provider := New("test-topic")

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

func TestProvider_SubscribeFetchError(t *testing.T) {
	testErr := errors.New("fetch error")
	db := &mockDB{fetchErr: testErr}
	provider := New("test-topic", WithDB(db), WithPollInterval(10*time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch := provider.Subscribe(ctx)

	select {
	case result := <-ch:
		if !result.IsError() {
			t.Fatal("expected error result")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	provider.Close()
}

func TestProvider_Close(t *testing.T) {
	db := &mockDB{}
	provider := New("test-topic", WithDB(db))

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !db.closed {
		t.Error("db not closed")
	}
}

func TestProvider_CloseNil(t *testing.T) {
	provider := New("test-topic")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error with nil db: %v", err)
	}
}

func TestProvider_Ping(t *testing.T) {
	db := &mockDB{}
	provider := New("test-topic", WithDB(db))

	err := provider.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_PingError(t *testing.T) {
	testErr := errors.New("connection lost")
	db := &mockDB{pingErr: testErr}
	provider := New("test-topic", WithDB(db))

	err := provider.Ping(context.Background())
	if err != testErr {
		t.Fatalf("expected %v, got %v", testErr, err)
	}
}

func TestProvider_PingNoDB(t *testing.T) {
	provider := New("test-topic")

	err := provider.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error with nil db")
	}
}

func TestProvider_NackReleasesMessage(t *testing.T) {
	db := &mockDB{
		messages: []Message{
			{ID: "msg-1", Data: []byte(`{"order":"1"}`)},
		},
	}
	provider := New("test-topic", WithDB(db), WithPollInterval(10*time.Millisecond), WithVisibilityTimeout(30*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	ch := provider.Subscribe(ctx)

	select {
	case result := <-ch:
		if result.IsError() {
			t.Fatalf("unexpected error: %v", result.Error())
		}
		// Nack should release the message
		if err := result.Value().Nack(); err != nil {
			t.Fatalf("nack failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	cancel()
	provider.Close()

	released := db.Released()
	if len(released) != 1 {
		t.Fatalf("expected 1 released message, got %d", len(released))
	}
	if released[0] != "msg-1" {
		t.Errorf("expected released message 'msg-1', got %q", released[0])
	}
}

func TestProvider_NackNoOpWithoutVisibilityTimeout(t *testing.T) {
	db := &mockDB{
		messages: []Message{
			{ID: "msg-1", Data: []byte(`{"order":"1"}`)},
		},
	}
	// No visibility timeout set
	provider := New("test-topic", WithDB(db), WithPollInterval(10*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	ch := provider.Subscribe(ctx)

	select {
	case result := <-ch:
		if result.IsError() {
			t.Fatalf("unexpected error: %v", result.Error())
		}
		// Nack should be a no-op
		if err := result.Value().Nack(); err != nil {
			t.Fatalf("nack failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	cancel()
	provider.Close()

	released := db.Released()
	if len(released) != 0 {
		t.Fatalf("expected 0 released messages (no visibility timeout), got %d", len(released))
	}
}

func TestProvider_VisibilityTimeoutOption(t *testing.T) {
	db := &mockDB{}
	provider := New("test-topic", WithDB(db), WithVisibilityTimeout(5*time.Minute))

	// Check that the option was applied by verifying behavior
	// (We can't directly access private fields, so we test through behavior)
	if provider.visibilityTimeout != 5*time.Minute {
		t.Errorf("expected visibility timeout 5m, got %v", provider.visibilityTimeout)
	}
}
