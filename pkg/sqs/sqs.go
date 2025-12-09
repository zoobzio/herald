// Package sqs provides a Herald provider for AWS SQS.
package sqs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/zoobzio/herald"
)

// Provider implements herald.Provider for SQS.
type Provider struct {
	client   *sqs.Client
	queueURL string
}

// Option configures a Provider.
type Option func(*Provider)

// WithClient sets the SQS client.
func WithClient(c *sqs.Client) Option {
	return func(p *Provider) {
		p.client = c
	}
}

// New creates an SQS provider for the given queue URL.
func New(queueURL string, opts ...Option) *Provider {
	p := &Provider{
		queueURL: queueURL,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish sends raw bytes with metadata to SQS.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.client == nil {
		return herald.ErrNoWriter
	}

	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(p.queueURL),
		MessageBody: aws.String(string(data)),
	}

	// Convert herald.Metadata to SQS message attributes
	if len(metadata) > 0 {
		input.MessageAttributes = make(map[string]types.MessageAttributeValue, len(metadata))
		for k, v := range metadata {
			input.MessageAttributes[k] = types.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(v),
			}
		}
	}

	_, err := p.client.SendMessage(ctx, input)
	return err
}

// Subscribe returns a stream of messages from SQS.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])

	if p.client == nil {
		go func() {
			out <- herald.NewError[herald.Message](herald.ErrNoReader)
			close(out)
		}()
		return out
	}

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			result, err := p.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
				QueueUrl:              aws.String(p.queueURL),
				MaxNumberOfMessages:   10,
				WaitTimeSeconds:       20, // Long polling
				MessageAttributeNames: []string{"All"},
			})
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				select {
				case out <- herald.NewError[herald.Message](err):
				case <-ctx.Done():
					return
				}
				continue
			}

			for _, msg := range result.Messages {
				// Capture for closure
				receiptHandle := msg.ReceiptHandle
				queueURL := p.queueURL
				client := p.client

				// Convert SQS attributes to metadata
				var metadata herald.Metadata
				if len(msg.MessageAttributes) > 0 {
					metadata = make(herald.Metadata, len(msg.MessageAttributes))
					for k, v := range msg.MessageAttributes {
						if v.StringValue != nil {
							metadata[k] = *v.StringValue
						}
					}
				}

				heraldMsg := herald.Message{
					Data:     []byte(aws.ToString(msg.Body)),
					Metadata: metadata,
					Ack: func() error {
						_, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
							QueueUrl:      aws.String(queueURL),
							ReceiptHandle: receiptHandle,
						})
						return err
					},
					Nack: func() error {
						// SQS: don't delete = message returns to queue after visibility timeout
						return nil
					},
				}

				select {
				case out <- herald.NewSuccess(heraldMsg):
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

// Close releases SQS resources.
func (p *Provider) Close() error {
	return nil
}
