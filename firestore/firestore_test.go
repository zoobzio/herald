package firestore

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/gcloud"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func setupFirestore(t *testing.T) *firestore.Client {
	t.Helper()

	ctx := context.Background()

	container, err := gcloud.RunFirestore(ctx, "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators")
	if err != nil {
		t.Fatalf("failed to start firestore emulator: %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	projectID := container.Settings.ProjectID

	conn, err := grpc.NewClient(container.URI, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create grpc connection: %v", err)
	}

	client, err := firestore.NewClient(ctx, projectID, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("failed to create firestore client: %v", err)
	}

	t.Cleanup(func() {
		client.Close()
	})

	return client
}

func TestProvider_Publish(t *testing.T) {
	client := setupFirestore(t)
	collection := "herald-test-publish"
	provider := New(collection, WithClient(client))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify document was created
	ctx := context.Background()
	docs, err := client.Collection(collection).Documents(ctx).GetAll()
	if err != nil {
		t.Fatalf("failed to get documents: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
}

func TestProvider_PublishWithMetadata(t *testing.T) {
	client := setupFirestore(t)
	collection := "herald-test-metadata"
	provider := New(collection, WithClient(client))

	metadata := map[string]string{"trace-id": "abc123"}
	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify document and metadata
	ctx := context.Background()
	docs, err := client.Collection(collection).Documents(ctx).GetAll()
	if err != nil {
		t.Fatalf("failed to get documents: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	var doc document
	if err := docs[0].DataTo(&doc); err != nil {
		t.Fatalf("failed to parse document: %v", err)
	}
	if doc.Metadata["trace-id"] != "abc123" {
		t.Errorf("expected trace-id abc123, got %v", doc.Metadata)
	}
}

func TestProvider_PublishNoClient(t *testing.T) {
	provider := New("test-collection")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil client")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	client := setupFirestore(t)
	collection := "herald-test-subscribe"

	provider := New(collection, WithClient(client))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := provider.Subscribe(ctx)

	// Add documents after subscription starts (to trigger DocumentAdded events)
	go func() {
		time.Sleep(500 * time.Millisecond)
		client.Collection(collection).Add(context.Background(), document{
			Data: []byte(`{"order":"1"}`),
		})
		client.Collection(collection).Add(context.Background(), document{
			Data: []byte(`{"order":"2"}`),
		})
	}()

	var received [][]byte
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch:
			if result.IsError() {
				t.Fatalf("unexpected error: %v", result.Error())
			}
			if err := result.Value().Ack(); err != nil {
				t.Errorf("ack failed: %v", err)
			}
			received = append(received, result.Value().Data)
		case <-ctx.Done():
			t.Fatalf("timeout: received %d messages", len(received))
		}
	}

	cancel()

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}
}

func TestProvider_SubscribeNoClient(t *testing.T) {
	provider := New("test-collection")

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
	provider := New("test-collection")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
