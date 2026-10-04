package logger

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	err     error
}

func newAsyncWriter(out io.Writer, closers []io.Closer) *asyncWriter {
	ctx, cancel := context.WithCancel(context.Background())
	w := &asyncWriter{
		out:     out,
		closers: closers,
		pending: make(chan []byte, logQueueSize),
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *asyncWriter) Write(p []byte) (int, error) {
	if w.ctx.Err() != nil {
		return 0, io.ErrClosedPipe
	}
	select {
	case w.pending <- bytes.Clone(p):
	default:
	}
	// Shutdown can start while the record is copied.
	if w.ctx.Err() != nil {
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
		case <-w.ctx.Done():
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

func (w *asyncWriter) shutdown(ctx context.Context) error {
	w.cancel()
	select {
	case <-w.done:
		return w.err
	case <-ctx.Done():
		for len(w.pending) > 0 {
			select {
			case <-w.pending:
			default:
			}
		}
		return ctx.Err()
	}
}

func closeOutputs(ctx context.Context, closers []io.Closer) error {
	for _, c := range closers {
		c.(*asyncWriter).cancel()
	}
	errs := make([]error, len(closers))
	for i, c := range closers {
		errs[i] = c.(*asyncWriter).shutdown(ctx)
	}
	return errors.Join(errs...)
}
