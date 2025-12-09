package bolt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.etcd.io/bbolt"
)

func createTestDB(t *testing.T) *bbolt.DB {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(path)
	})

	return db
}

func TestProvider_Publish(t *testing.T) {
	db := createTestDB(t)
	provider := New("test-bucket", WithDB(db))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify by reading
	var data []byte
	db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("test-bucket"))
		if b == nil {
			t.Fatal("bucket not created")
		}
		c := b.Cursor()
		_, data = c.First()
		return nil
	})

	if string(data) != `{"test":"data"}` {
		t.Errorf("unexpected data: %s", data)
	}
}

func TestProvider_PublishNoDB(t *testing.T) {
	provider := New("test-bucket")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil db")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	db := createTestDB(t)

	// Pre-populate messages
	db.Update(func(tx *bbolt.Tx) error {
		b, _ := tx.CreateBucketIfNotExists([]byte("test-bucket"))
		b.Put([]byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte(`{"order":"1"}`))
		b.Put([]byte{0, 0, 0, 0, 0, 0, 0, 2}, []byte(`{"order":"2"}`))
		return nil
	})

	provider := New("test-bucket", WithDB(db), WithPollInterval(10*time.Millisecond))

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
			// Ack to remove from bucket
			result.Value().Ack()
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}

	cancel()

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}

	if string(received[0]) != `{"order":"1"}` {
		t.Errorf("unexpected first message: %s", received[0])
	}
}

func TestProvider_SubscribeNoDB(t *testing.T) {
	provider := New("test-bucket")

	ctx := context.Background()
	ch := provider.Subscribe(ctx)

	result, ok := <-ch
	if !ok {
		t.Fatal("expected error result before close")
	}
	if !result.IsError() {
		t.Error("expected error result")
	}

	_, ok = <-ch
	if ok {
		t.Error("expected closed channel after error")
	}
}

func TestProvider_Close(t *testing.T) {
	db := createTestDB(t)
	// Create a separate db just for this test since we're testing Close
	dir := t.TempDir()
	path := filepath.Join(dir, "close-test.db")
	closeDB, _ := bbolt.Open(path, 0600, nil)

	provider := New("test-bucket", WithDB(closeDB))

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify closed by trying to use it
	err = closeDB.View(func(tx *bbolt.Tx) error { return nil })
	if err != bbolt.ErrDatabaseNotOpen {
		t.Errorf("expected database not open error, got: %v", err)
	}

	// Use the main db to satisfy the linter
	_ = db
}

func TestProvider_CloseNil(t *testing.T) {
	provider := New("test-bucket")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error with nil db: %v", err)
	}
}
