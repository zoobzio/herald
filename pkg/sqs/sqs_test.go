package sqs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
)

func setupLocalStack(t *testing.T) (*sqs.Client, string) {
	t.Helper()

	ctx := context.Background()

	container, err := localstack.Run(ctx, "localstack/localstack:latest")
	if err != nil {
		t.Fatalf("failed to start localstack: %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}

	port, err := container.MappedPort(ctx, "4566/tcp")
	if err != nil {
		t.Fatalf("failed to get port: %v", err)
	}

	endpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	client := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	// Create test queue
	queueName := "herald-test-" + t.Name()
	result, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	queueURL := *result.QueueUrl

	t.Cleanup(func() {
		client.DeleteQueue(context.Background(), &sqs.DeleteQueueInput{
			QueueUrl: aws.String(queueURL),
		})
	})

	return client, queueURL
}

func TestProvider_Publish(t *testing.T) {
	client, queueURL := setupLocalStack(t)
	provider := New(queueURL, WithClient(client))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify by receiving
	result, err := client.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     5,
	})
	if err != nil {
		t.Fatalf("failed to receive: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}

	if *result.Messages[0].Body != `{"test":"data"}` {
		t.Errorf("unexpected body: %s", *result.Messages[0].Body)
	}
}

func TestProvider_PublishWithMetadata(t *testing.T) {
	client, queueURL := setupLocalStack(t)
	provider := New(queueURL, WithClient(client))

	metadata := map[string]string{"trace-id": "abc123"}
	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := client.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(queueURL),
		MaxNumberOfMessages:   1,
		WaitTimeSeconds:       5,
		MessageAttributeNames: []string{"All"},
	})
	if err != nil {
		t.Fatalf("failed to receive: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatal("expected 1 message")
	}

	attr, ok := result.Messages[0].MessageAttributes["trace-id"]
	if !ok || *attr.StringValue != "abc123" {
		t.Errorf("unexpected attribute: %v", result.Messages[0].MessageAttributes)
	}
}

func TestProvider_PublishNoClient(t *testing.T) {
	provider := New("http://localhost/queue")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil client")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	client, queueURL := setupLocalStack(t)

	// Pre-populate messages
	client.SendMessage(context.Background(), &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(`{"order":"1"}`),
	})
	client.SendMessage(context.Background(), &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(`{"order":"2"}`),
	})

	provider := New(queueURL, WithClient(client))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := provider.Subscribe(ctx)

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
			t.Fatal("timeout")
		}
	}

	cancel()

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}
}

func TestProvider_SubscribeNoClient(t *testing.T) {
	provider := New("http://localhost/queue")

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
	provider := New("http://localhost/queue")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
