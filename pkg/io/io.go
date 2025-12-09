// Package io provides a Herald provider for io.Reader/io.Writer.
// Useful for testing, CLI piping, and file-based messaging.
package io

import (
	"bufio"
	"context"
	"io"
	"sync"

	"github.com/zoobzio/herald"
)

// Provider implements herald.Provider for io.Reader/io.Writer.
type Provider struct {
	reader    io.Reader
	writer    io.Writer
	delimiter byte
	mu        sync.Mutex
}

// Option configures a Provider.
type Option func(*Provider)

// WithReader sets the io.Reader for subscribing.
func WithReader(r io.Reader) Option {
	return func(p *Provider) {
		p.reader = r
	}
}

// WithWriter sets the io.Writer for publishing.
func WithWriter(w io.Writer) Option {
	return func(p *Provider) {
		p.writer = w
	}
}

// WithDelimiter sets the message delimiter (default: newline).
func WithDelimiter(d byte) Option {
	return func(p *Provider) {
		p.delimiter = d
	}
}

// New creates an io provider.
func New(opts ...Option) *Provider {
	p := &Provider{
		delimiter: '\n',
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish writes data to the writer followed by the delimiter.
// Note: io streams do not support metadata natively; metadata is ignored.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.writer == nil {
		return herald.ErrNoWriter
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Write data + delimiter
	if _, err := p.writer.Write(data); err != nil {
		return err
	}
	if _, err := p.writer.Write([]byte{p.delimiter}); err != nil {
		return err
	}
	return nil
}

// Subscribe reads from the reader, splitting on the delimiter.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])

	if p.reader == nil {
		go func() {
			out <- herald.NewError[herald.Message](herald.ErrNoReader)
			close(out)
		}()
		return out
	}

	go func() {
		defer close(out)

		scanner := bufio.NewScanner(p.reader)
		if p.delimiter != '\n' {
			scanner.Split(splitFunc(p.delimiter))
		}

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			data := make([]byte, len(scanner.Bytes()))
			copy(data, scanner.Bytes())

			msg := herald.Message{
				Data: data,
				Ack: func() error {
					// No-op for io - data is consumed
					return nil
				},
				Nack: func() error {
					// No-op for io - can't rewind
					return nil
				},
			}

			select {
			case out <- herald.NewSuccess(msg):
			case <-ctx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case out <- herald.NewError[herald.Message](err):
			case <-ctx.Done():
			}
		}
	}()

	return out
}

// Close is a no-op for io provider.
// The caller is responsible for closing the underlying reader/writer.
func (p *Provider) Close() error {
	return nil
}

// splitFunc returns a bufio.SplitFunc for the given delimiter.
func splitFunc(delim byte) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		for i, b := range data {
			if b == delim {
				return i + 1, data[:i], nil
			}
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
}
