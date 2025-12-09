package sns

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
)

func setupLocalStack(t *testing.T) (*sns.Client, string) {
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

	client := sns.NewFromConfig(cfg, func(o *sns.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	// Create test topic
	topicName := "herald-test-" + t.Name()
	result, err := client.CreateTopic(ctx, &sns.CreateTopicInput{
		Name: aws.String(topicName),
	})
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}

	topicARN := *result.TopicArn

	t.Cleanup(func() {
		client.DeleteTopic(context.Background(), &sns.DeleteTopicInput{
			TopicArn: aws.String(topicARN),
		})
	})

	return client, topicARN
}

func TestProvider_Publish(t *testing.T) {
	client, topicARN := setupLocalStack(t)
	provider := New(topicARN, WithClient(client))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// SNS is fire-and-forget; we can only verify no error occurred
}

func TestProvider_PublishWithMetadata(t *testing.T) {
	client, topicARN := setupLocalStack(t)
	provider := New(topicARN, WithClient(client))

	metadata := map[string]string{"trace-id": "abc123"}
	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_PublishNoClient(t *testing.T) {
	provider := New("arn:aws:sns:us-east-1:000000000000:test")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil client")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	// SNS does not support direct subscription
	provider := New("arn:aws:sns:us-east-1:000000000000:test")

	ctx := context.Background()
	ch := provider.Subscribe(ctx)

	// Channel should be immediately closed
	_, ok := <-ch
	if ok {
		t.Error("expected closed channel for SNS subscribe")
	}
}

func TestProvider_Close(t *testing.T) {
	provider := New("arn:aws:sns:us-east-1:000000000000:test")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
