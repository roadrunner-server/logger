package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	logBufferSize = 1 << 20
	flushTimeout  = 5 * time.Second
)

type asyncWriter struct {
	out     io.Writer
	closers []io.Closer
	wake    chan struct{}
	done    chan struct{}
	err     error

	mu      sync.Mutex
	pending []byte
	closed  bool
	dropped uint64
}

func newAsyncWriter(out io.Writer, closers []io.Closer) *asyncWriter {
	w := &asyncWriter{out: out, closers: closers, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go w.run()
	return w
}

func (w *asyncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, io.ErrClosedPipe
	}
	if len(p) > logBufferSize-len(w.pending) {
		w.dropped++
		return len(p), nil
	}
	w.pending = append(w.pending, p...)
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (w *asyncWriter) run() {
	defer close(w.done)
	var batch []byte
	for {
		<-w.wake
		w.mu.Lock()
		batch, w.pending = w.pending, batch[:0]
		closed := w.closed
		if closed {
			w.pending = nil
		}
		w.mu.Unlock()

		if len(batch) > 0 {
			_, err := w.out.Write(batch)
			if w.err == nil {
				w.err = err
			}
		}
		if closed {
			break
		}
	}
	for _, c := range w.closers {
		w.err = errors.Join(w.err, c.Close())
	}
}

func (w *asyncWriter) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	return w.shutdown(ctx)
}

func (w *asyncWriter) shutdown(ctx context.Context) error {
	w.mu.Lock()
	w.closed = true
	dropped := w.dropped
	select {
	case w.wake <- struct{}{}:
	default:
	}
	w.mu.Unlock()
	var err error
	select {
	case <-w.done:
		err = w.err
	case <-ctx.Done():
		w.mu.Lock()
		w.pending = nil
		w.mu.Unlock()
		err = ctx.Err()
	}
	if dropped > 0 {
		err = errors.Join(err, fmt.Errorf("logger: dropped messages: %d", dropped))
	}
	return err
}

func closeOutputs(ctx context.Context, closers []io.Closer) error {
	var errs []error
	for _, c := range closers {
		if w, ok := c.(*asyncWriter); ok {
			errs = append(errs, w.shutdown(ctx))
		} else {
			errs = append(errs, c.Close())
		}
	}
	return errors.Join(errs...)
}
