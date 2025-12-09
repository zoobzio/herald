package pubsub

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/gcloud"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func setupPubSub(t *testing.T) (*pubsub.Client, *pubsub.Topic, *pubsub.Subscription) {
	t.Helper()

	ctx := context.Background()

	container, err := gcloud.RunPubsub(ctx, "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators")
	if err != nil {
		t.Fatalf("failed to start pubsub emulator: %v", err)
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

	client, err := pubsub.NewClient(ctx, projectID, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("failed to create pubsub client: %v", err)
	}

	// Create topic and subscription
	topicID := "herald-test-topic"
	topic, err := client.CreateTopic(ctx, topicID)
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}

	subID := "herald-test-sub"
	sub, err := client.CreateSubscription(ctx, subID, pubsub.SubscriptionConfig{
		Topic:       topic,
		AckDeadline: 10 * time.Second,
	})
	if err != nil {
		topic.Delete(ctx)
		t.Fatalf("failed to create subscription: %v", err)
	}

	t.Cleanup(func() {
		sub.Delete(context.Background())
		topic.Delete(context.Background())
		client.Close()
	})

	return client, topic, sub
}

func TestProvider_Publish(t *testing.T) {
	_, topic, _ := setupPubSub(t)
	provider := New(WithTopic(topic))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_PublishWithMetadata(t *testing.T) {
	_, topic, _ := setupPubSub(t)
	provider := New(WithTopic(topic))

	metadata := map[string]string{"trace-id": "abc123"}
	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_PublishNoTopic(t *testing.T) {
	provider := New()

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil topic")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	_, topic, sub := setupPubSub(t)

	// Publish messages first
	ctx := context.Background()
	topic.Publish(ctx, &pubsub.Message{Data: []byte(`{"order":"1"}`)}).Get(ctx)
	topic.Publish(ctx, &pubsub.Message{Data: []byte(`{"order":"2"}`)}).Get(ctx)

	provider := New(WithSubscription(sub))

	subCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ch := provider.Subscribe(subCtx)

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
		case <-subCtx.Done():
			t.Fatal("timeout")
		}
	}

	cancel()

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}
}

func TestProvider_SubscribeNoSubscription(t *testing.T) {
	provider := New()

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
	provider := New()

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
