package logger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

const (
	logQueueSize = 1024
	flushTimeout = 5 * time.Second
)

type asyncWriter struct {
	out     io.Writer
	closers []io.Closer
	pending chan []byte
	stop    chan struct{}
	done    chan struct{}
	err     error

	closed  atomic.Bool
	dropped atomic.Uint64
}

func newAsyncWriter(out io.Writer, closers []io.Closer) *asyncWriter {
	w := &asyncWriter{
		out:     out,
		closers: closers,
		pending: make(chan []byte, logQueueSize),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *asyncWriter) Write(p []byte) (int, error) {
	if w.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	select {
	case w.pending <- bytes.Clone(p):
	default:
		w.dropped.Add(1)
	}
	// Shutdown can start while the record is copied.
	if w.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

func (w *asyncWriter) run() {
	defer func() {
		for _, c := range w.closers {
			w.err = errors.Join(w.err, c.Close())
		}
		close(w.done)
	}()
	for {
		var p []byte
		select {
		case p = <-w.pending:
		case <-w.stop:
			select {
			case p = <-w.pending:
			default:
				return
			}
		}
		_, err := w.out.Write(p)
		if w.err == nil {
			w.err = err
		}
	}
}

func (w *asyncWriter) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	return w.shutdown(ctx)
}

func (w *asyncWriter) startShutdown() {
	if !w.closed.Swap(true) {
		close(w.stop)
	}
}

func (w *asyncWriter) shutdown(ctx context.Context) error {
	w.startShutdown()
	var err error
	select {
	case <-w.done:
		err = w.err
	case <-ctx.Done():
		for len(w.pending) > 0 {
			select {
			case <-w.pending:
			default:
			}
		}
		err = ctx.Err()
	}
	if dropped := w.dropped.Load(); dropped > 0 {
		err = errors.Join(err, fmt.Errorf("logger: dropped messages: %d", dropped))
	}
	return err
}

func closeOutputs(ctx context.Context, closers []io.Closer) error {
	for _, c := range closers {
		if w, ok := c.(*asyncWriter); ok {
			w.startShutdown()
		}
	}
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
