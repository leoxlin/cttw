package acp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Transport sends and receives line-delimited JSON-RPC messages.
type Transport interface {
	Send(ctx context.Context, data []byte) error
	Recv(ctx context.Context) ([]byte, error)
	Close() error
}

// StdioTransport uses stdin for writing and stdout for reading.
type StdioTransport struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	mu     sync.Mutex
	scan   *bufio.Scanner
	lines  chan []byte
	errCh  chan error
	closed bool
	close  chan struct{}
}

func NewStdioTransport(stdin io.WriteCloser, stdout io.ReadCloser) *StdioTransport {
	return &StdioTransport{
		stdin:  stdin,
		stdout: stdout,
		lines:  make(chan []byte, 64),
		errCh:  make(chan error, 1),
		close:  make(chan struct{}),
	}
}

// Start begins reading lines from stdout into an internal buffer.
func (t *StdioTransport) Start(ctx context.Context) error {
	t.scan = bufio.NewScanner(t.stdout)
	// Increase max line size to 1 MB.
	t.scan.Buffer(make([]byte, 64*1024), 1024*1024)
	for t.scan.Scan() {
		line := t.scan.Bytes()
		// Copy because scanner reuses its buffer.
		buf := make([]byte, len(line))
		copy(buf, line)
		select {
		case t.lines <- buf:
		case <-ctx.Done():
			return ctx.Err()
		case <-t.close:
			return nil
		}
	}
	if err := t.scan.Err(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		select {
		case t.errCh <- err:
		default:
		}
	}
	return nil
}

func (t *StdioTransport) Send(ctx context.Context, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return fmt.Errorf("transport closed")
	}
	_, err := t.stdin.Write(append(data, '\n'))
	return err
}

func (t *StdioTransport) Recv(ctx context.Context) ([]byte, error) {
	select {
	case line := <-t.lines:
		return line, nil
	case err := <-t.errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.close:
		return nil, io.EOF
	}
}

func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.close)
	var errs []error
	if err := t.stdin.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := t.stdout.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
