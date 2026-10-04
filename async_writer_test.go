package logger

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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
		records  int
		wantTail string
	}{
		{name: "copied bytes", size: 10, records: 1, wantTail: "tail\n"},
		{name: "full queue", size: 10, records: 1024},
		{name: "large record", size: (1 << 20) + 1, records: 1, wantTail: "tail\n"},
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
				for range tt.records {
					if n, err := w.Write(p); n != len(p) || err != nil {
						t.Fatalf("Write = %d, %v", n, err)
					}
				}
				clear(p)
				if _, err := w.Write([]byte("tail\n")); err != nil {
					t.Fatal(err)
				}
				var got bytes.Buffer
				go func() { _, _ = io.Copy(&got, r) }()
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				want := "first\n" + strings.Repeat("x", tt.size*tt.records) + tt.wantTail
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
				for range 1024 {
					_, _ = w.Write([]byte("queued\n"))
				}
				_, _ = w.Write([]byte("dropped\n"))
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				start := time.Now()
				if err := tt.stop(ctx, w); !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("shutdown = %v, want deadline exceeded", err)
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
			for range 64 {
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
	if got := out.String(); got != strings.Repeat("record\n", 1024) {
		t.Errorf("concurrent output lost or corrupted records: got %d bytes", len(got))
	}
}

func TestAsyncWriterConcurrentShutdown(t *testing.T) {
	var out bytes.Buffer
	w := newAsyncWriter(&out, nil)
	var accepted atomic.Int64
	var wg sync.WaitGroup
	ready := make(chan struct{}, 16)
	start := make(chan struct{})
	for range 16 {
		wg.Go(func() {
			_, _ = w.Write([]byte("record\n"))
			accepted.Add(1)
			ready <- struct{}{}
			<-start
			for range 32 {
				if _, err := w.Write([]byte("record\n")); err == nil {
					accepted.Add(1)
				} else if !errors.Is(err, io.ErrClosedPipe) {
					t.Error(err)
				}
			}
		})
	}
	for range 16 {
		<-ready
	}
	close(start)
	if err := w.Close(); err != nil {
		t.Error(err)
	}
	wg.Wait()
	if got := strings.Count(out.String(), "record\n"); int64(got) < accepted.Load() {
		t.Errorf("drained %d records, accepted %d", got, accepted.Load())
	}
}

func TestShutdownStartsAllOutputs(t *testing.T) {
	tests := []struct {
		name string
		stop func(context.Context, *asyncWriter, *asyncWriter) error
	}{
		{name: "channel outputs", stop: func(_ context.Context, blocked, healthy *asyncWriter) error {
			return (&Log{closers: []io.Closer{blocked, healthy}}).Close()
		}},
		{name: "root and channel outputs", stop: func(ctx context.Context, blocked, healthy *asyncWriter) error {
			return (&Plugin{closers: []io.Closer{healthy}, logs: []*Log{{closers: []io.Closer{blocked}}}}).Stop(ctx)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r1, out1 := io.Pipe()
				r2, out2 := io.Pipe()
				defer func() { _ = r1.Close() }()
				defer func() { _ = r2.Close() }()
				blocked := newAsyncWriter(out1, []io.Closer{out1})
				healthy := newAsyncWriter(out2, []io.Closer{out2})
				_, _ = blocked.Write([]byte("blocked\n"))
				_, _ = healthy.Write([]byte("healthy\n"))
				drained := make(chan string, 1)
				go func() {
					data, _ := io.ReadAll(r2)
					drained <- string(data)
				}()
				done := make(chan error, 1)
				go func() { done <- tt.stop(t.Context(), blocked, healthy) }()
				synctest.Wait()
				select {
				case got := <-drained:
					if got != "healthy\n" {
						t.Errorf("healthy output = %q", got)
					}
				default:
					t.Error("healthy output shutdown waited for the blocked output")
				}
				_, _ = io.ReadAll(r1)
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		})
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
