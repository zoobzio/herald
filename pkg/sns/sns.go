// Package sns provides a Herald provider for AWS SNS.
// SNS is publish-only; use the sqs package for subscribing.
package sns

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/zoobzio/herald"
)

// Provider implements herald.Provider for AWS SNS.
// Note: SNS is publish-only. Subscribe returns a closed channel.
type Provider struct {
	client   *sns.Client
	topicARN string
}

// Option configures a Provider.
type Option func(*Provider)

// WithClient sets the SNS client.
func WithClient(c *sns.Client) Option {
	return func(p *Provider) {
		p.client = c
	}
}

// New creates an SNS provider for the given topic ARN.
func New(topicARN string, opts ...Option) *Provider {
	p := &Provider{
		topicARN: topicARN,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish sends raw bytes with metadata to SNS.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.client == nil {
		return herald.ErrNoWriter
	}

	input := &sns.PublishInput{
		TopicArn: aws.String(p.topicARN),
		Message:  aws.String(string(data)),
	}

	// Convert herald.Metadata to SNS message attributes
	if len(metadata) > 0 {
		input.MessageAttributes = make(map[string]types.MessageAttributeValue, len(metadata))
		for k, v := range metadata {
			input.MessageAttributes[k] = types.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(v),
			}
		}
	}

	_, err := p.client.Publish(ctx, input)
	return err
}

// Subscribe returns a closed channel. SNS does not support direct subscription.
// Use SQS with an SNS subscription for consuming messages.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])
	close(out)
	return out
}

// Ping verifies SNS connectivity by getting topic attributes.
func (p *Provider) Ping(ctx context.Context) error {
	if p.client == nil {
		return herald.ErrNoWriter
	}
	_, err := p.client.GetTopicAttributes(ctx, &sns.GetTopicAttributesInput{
		TopicArn: aws.String(p.topicARN),
	})
	return err
}

// Close releases SNS resources.
func (p *Provider) Close() error {
	return nil
}
