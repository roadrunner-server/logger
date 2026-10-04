package logger

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestBuildLoggerBlockedOutput(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "text", cfg: Config{Mode: development}},
		{name: "json", cfg: Config{Mode: production}},
		{name: "raw", cfg: Config{Mode: raw}},
		{name: "custom", cfg: Config{Format: "%message%"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = w.Close() }()
			defer func() { _ = r.Close() }()

			stderr := os.Stderr
			os.Stderr = w
			res, err := tt.cfg.BuildLogger()
			os.Stderr = stderr
			if err != nil {
				t.Fatal(err)
			}

			done := make(chan struct{})
			go func() {
				defer close(done)
				message := strings.Repeat("x", 64<<10)
				for range 32 {
					res.Logger.Info(message)
				}
			}()

			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("log calls blocked on an unread stderr pipe")
			}
			_ = r.Close()
			<-done
			for _, c := range res.Closers {
				_ = c.Close()
			}
		})
	}
}

func TestAsyncWriterQueue(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		wantSize int
		wantTail string
		wantDrop bool
	}{
		{name: "copied bytes", size: 10, wantSize: 10, wantTail: "tail\n"},
		{name: "full queue", size: 1 << 20, wantSize: 1 << 20, wantDrop: true},
		{name: "oversized record", size: (1 << 20) + 1, wantTail: "tail\n", wantDrop: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r, out := io.Pipe()
				defer func() { _ = r.Close() }()
				w := newAsyncWriter(out, []io.Closer{out})
				if _, err := w.Write([]byte("first\n")); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				p := bytes.Repeat([]byte("x"), tt.size)
				if n, err := w.Write(p); n != len(p) || err != nil {
					t.Fatalf("Write = %d, %v", n, err)
				}
				clear(p)
				if _, err := w.Write([]byte("tail\n")); err != nil {
					t.Fatal(err)
				}
				var got bytes.Buffer
				go func() { _, _ = io.Copy(&got, r) }()
				err := w.Close()
				if tt.wantDrop {
					if err == nil || !strings.Contains(err.Error(), "dropped messages: 1") {
						t.Fatalf("Close = %v, want one reported drop", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				want := "first\n" + strings.Repeat("x", tt.wantSize) + tt.wantTail
				if got.String() != want {
					t.Errorf("output differs from the accepted records: got %d bytes, want %d", got.Len(), len(want))
				}
			})
		})
	}
}

func TestAsyncWriterShutdown(t *testing.T) {
	tests := []struct {
		name string
		stop func(context.Context, *asyncWriter) error
		wait time.Duration
	}{
		{name: "writer timeout", wait: 5 * time.Second, stop: func(_ context.Context, w *asyncWriter) error {
			return w.Close()
		}},
		{name: "channel timeout", wait: 5 * time.Second, stop: func(_ context.Context, w *asyncWriter) error {
			return (&Log{closers: []io.Closer{w}}).Close()
		}},
		{name: "plugin deadline", wait: time.Second, stop: func(ctx context.Context, w *asyncWriter) error {
			return (&Plugin{closers: []io.Closer{w}}).Stop(ctx)
		}},
		{name: "plugin channel deadline", wait: time.Second, stop: func(ctx context.Context, w *asyncWriter) error {
			return (&Plugin{logs: []*Log{{closers: []io.Closer{w}}}}).Stop(ctx)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r, out := io.Pipe()
				defer func() { _ = r.Close() }()
				w := newAsyncWriter(out, []io.Closer{out})
				_, _ = w.Write([]byte("first\n"))
				synctest.Wait()
				_, _ = w.Write([]byte("queued\n"))
				_, _ = w.Write(bytes.Repeat([]byte("x"), (1<<20)+1))
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				start := time.Now()
				err := tt.stop(ctx, w)
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("shutdown = %v, want deadline exceeded", err)
				}
				if err == nil || !strings.Contains(err.Error(), "dropped messages: 1") {
					t.Errorf("shutdown = %v, want one reported drop", err)
				}
				if elapsed := time.Since(start); elapsed != tt.wait {
					t.Errorf("shutdown took %v, want %v", elapsed, tt.wait)
				}
				if _, err := w.Write([]byte("late\n")); !errors.Is(err, io.ErrClosedPipe) {
					t.Errorf("write after shutdown = %v, want closed pipe", err)
				}
				got, err := io.ReadAll(r)
				if err != nil || string(got) != "first\n" {
					t.Errorf("output after timeout = %q, %v", got, err)
				}
			})
		})
	}
}

func TestAsyncWriterConcurrentDrain(t *testing.T) {
	var out bytes.Buffer
	w := newAsyncWriter(&out, nil)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				if _, err := w.Write([]byte("record\n")); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != strings.Repeat("record\n", 1600) {
		t.Errorf("concurrent output lost or corrupted records: got %d bytes", len(got))
	}
}

func TestPluginReportsOutputError(t *testing.T) {
	r, out := io.Pipe()
	want := errors.New("output failed")
	_ = r.CloseWithError(want)
	w := newAsyncWriter(out, []io.Closer{out})
	_, _ = w.Write([]byte("record\n"))
	p := &Plugin{closers: []io.Closer{w}}
	if err := p.Stop(t.Context()); !errors.Is(err, want) {
		t.Errorf("Stop = %v, want output error", err)
	}
}
