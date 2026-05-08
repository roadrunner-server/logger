package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	fmtMessage      = "%message%"
	fmtMessageAttrs = "%message% %attrs%"
)

func TestFormatHandler_BasicPlaceholders(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:      slog.LevelDebug,
		Format:     "%time% [%level%] %message%",
		TimeLayout: "15:04:05",
	})

	ts := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	r := slog.NewRecord(ts, slog.LevelInfo, "hello world", 0)

	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "12:00:00 [INFO] hello world\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatHandler_Attrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: fmtMessageAttrs,
	})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "req", 0)
	r.AddAttrs(slog.String("method", "GET"), slog.Int("status", 200))

	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := "req method=GET status=200"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: fmtMessageAttrs,
	})

	child := h.WithAttrs([]slog.Attr{slog.String("pid", "1234")})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "start", 0)
	r.AddAttrs(slog.String("worker", "w1"))

	err := child.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := "start pid=1234 worker=w1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Verify parent handler does not have child's pre-attached attrs.
	buf.Reset()
	r2 := slog.NewRecord(time.Time{}, slog.LevelInfo, "parent", 0)
	err = h.Handle(t.Context(), r2)
	if err != nil {
		t.Fatal(err)
	}

	got = strings.TrimSpace(buf.String())
	want = "parent"
	if got != want {
		t.Errorf("parent got %q, want %q", got, want)
	}
}

func TestFormatHandler_WithAttrsEmpty(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: fmtMessage,
	})

	// WithAttrs with empty slice should return the same handler.
	child := h.WithAttrs(nil)
	if child != h {
		t.Error("WithAttrs(nil) should return the same handler")
	}
}

func TestFormatHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: fmtMessageAttrs,
	})

	child := h.WithGroup("http")

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "req", 0)
	r.AddAttrs(slog.String("method", "POST"))

	err := child.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := "req http.method=POST"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatHandler_WithGroupEmpty(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: fmtMessage,
	})

	child := h.WithGroup("")
	if child != h {
		t.Error("WithGroup(\"\") should return the same handler")
	}
}

func TestFormatHandler_NestedGroups(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: fmtMessageAttrs,
	})

	child := h.WithGroup("http").WithGroup("request")

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "req", 0)
	r.AddAttrs(slog.String("method", "GET"))

	err := child.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := "req http.request.method=GET"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatHandler_SourcePlaceholders(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: "%source_file%:%source_line% %source_func% %message%",
	})

	// Use slog.Logger so PC is populated.
	l := slog.New(h)
	l.Info("test source")

	got := buf.String()
	if !strings.Contains(got, "format_handler_test.go") {
		t.Errorf("expected source file, got %q", got)
	}
	if !strings.Contains(got, "test source") {
		t.Errorf("expected message, got %q", got)
	}
	// The function name should be present.
	if !strings.Contains(got, "TestFormatHandler_SourcePlaceholders") {
		t.Errorf("expected function name, got %q", got)
	}
}

func TestFormatHandler_LoggerPlaceholder(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: "[%logger%] %message% %attrs%",
	})

	// Simulate NamedLogger attaching the "logger" attr.
	l := slog.New(h).With("logger", "http")
	l.Info("request", "status", 200)

	got := strings.TrimSpace(buf.String())
	// %logger% should be "http", and "logger" should NOT appear in %attrs%.
	if !strings.HasPrefix(got, "[http] request") {
		t.Errorf("expected logger prefix, got %q", got)
	}
	if strings.Contains(got, "logger=http") {
		t.Error("logger attr should be excluded from %attrs%")
	}
	if !strings.Contains(got, "status=200") {
		t.Errorf("expected status attr, got %q", got)
	}
}

func TestFormatHandler_CustomTimeFormat(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:      slog.LevelDebug,
		Format:     "%time% %message%",
		TimeLayout: "2006-01-02",
	})

	ts := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	r := slog.NewRecord(ts, slog.LevelInfo, "test", 0)

	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := "2026-03-20 test"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatHandler_DefaultTimeLayout(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: "%time%",
	})

	ts := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	r := slog.NewRecord(ts, slog.LevelInfo, "", 0)

	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := ts.Format(time.RFC3339)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatHandler_CustomLineEnding(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:      slog.LevelDebug,
		Format:     fmtMessage,
		LineEnding: new("\r\n"),
	})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "test\r\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatHandler_SkipLineEnding(t *testing.T) {
	// Build via Config with SkipLineEnding to test the end-to-end path
	// from config through to handler output.
	var buf bytes.Buffer
	cfg := &Config{
		Format:         fmtMessage,
		SkipLineEnding: true,
		Level:          levelDebug,
	}
	cfg.InitDefault()

	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:      slog.LevelDebug,
		Format:     cfg.Format,
		LineEnding: new(cfg.lineEnding()),
	})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "test"
	if got != want {
		t.Errorf("got %q, want %q (skip_line_ending should produce no trailing newline)", got, want)
	}
}

func TestFormatHandler_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelWarn,
		Format: fmtMessage,
	})

	if h.Enabled(t.Context(), slog.LevelDebug) {
		t.Error("debug should not be enabled at warn level")
	}
	if h.Enabled(t.Context(), slog.LevelInfo) {
		t.Error("info should not be enabled at warn level")
	}
	if !h.Enabled(t.Context(), slog.LevelWarn) {
		t.Error("warn should be enabled at warn level")
	}
	if !h.Enabled(t.Context(), slog.LevelError) {
		t.Error("error should be enabled at warn level")
	}
}

func TestFormatHandler_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: fmtMessageAttrs,
	})

	const n = 100
	var wg sync.WaitGroup

	for i := range n {
		wg.Go(func() {
			r := slog.NewRecord(time.Time{}, slog.LevelInfo, "msg", 0)
			r.AddAttrs(slog.Int("i", i))
			_ = h.Handle(t.Context(), r)
		})
	}

	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != n {
		t.Errorf("expected %d lines, got %d", n, len(lines))
	}
}

func TestFormatHandler_EmptyFormat(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: "",
	})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	// Empty format produces just a line ending.
	got := buf.String()
	if got != "\n" {
		t.Errorf("got %q, want %q", got, "\n")
	}
}

func TestFormatHandler_NoPlaceholders(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: "static text",
	})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "ignored", 0)
	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "static text" {
		t.Errorf("got %q, want %q", got, "static text")
	}
}

func TestFormatHandler_UnknownPlaceholders(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: "%message% %unknown%",
	})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := "test %unknown%"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatHandler_ZeroTime(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: "%time% %message%",
	})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	err := h.Handle(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := "test"
	if got != want {
		t.Errorf("got %q, want %q (zero time should produce empty string)", got, want)
	}
}

func TestFormatHandler_ConfigLineEnding(t *testing.T) {
	cfg := &Config{
		Format:     fmtMessage,
		LineEnding: "\r\n",
		Level:      levelInfo,
	}
	cfg.InitDefault()

	res, err := cfg.BuildLogger()
	if err != nil {
		t.Fatal(err)
	}

	// The logger should be usable.
	res.Logger.Info("test")

	for _, c := range res.Closers {
		_ = c.Close()
	}
}

func TestFormatHandler_ConfigSkipLineEnding(t *testing.T) {
	cfg := &Config{
		Format:         fmtMessage,
		SkipLineEnding: true,
		Level:          levelInfo,
	}
	cfg.InitDefault()

	le := cfg.lineEnding()
	if le != "" {
		t.Errorf("expected empty line ending with skip, got %q", le)
	}
}

func TestFormatHandler_ConfigFormatOverridesMode(t *testing.T) {
	cfg := &Config{
		Mode:   production,
		Format: "%level% %message%",
		Level:  levelInfo,
	}
	cfg.InitDefault()

	res, err := cfg.BuildLogger()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the handler is a FormatHandler, not JSON.
	_, ok := res.Logger.Handler().(*FormatHandler)
	if !ok {
		t.Errorf("expected *FormatHandler, got %T", res.Logger.Handler())
	}
}

func TestFormatHandler_AllPlaceholders(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:      slog.LevelDebug,
		Format:     "%time% %level% %message% %logger% %attrs% %source_file% %source_line% %source_func%",
		TimeLayout: "15:04",
	})

	l := slog.New(h).With("logger", "test-ch")
	l.Info("hello", "key", "val")

	got := buf.String()
	if !strings.Contains(got, "INFO") {
		t.Errorf("expected level, got %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("expected message, got %q", got)
	}
	if !strings.Contains(got, "test-ch") {
		t.Errorf("expected logger name, got %q", got)
	}
	if !strings.Contains(got, "key=val") {
		t.Errorf("expected attrs, got %q", got)
	}
	if !strings.Contains(got, "format_handler_test.go") {
		t.Errorf("expected source file, got %q", got)
	}
}

// Verify that the handler satisfies the slog.Handler interface.
var _ slog.Handler = (*FormatHandler)(nil)

func TestFormatHandler_GroupWithPreAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewFormatHandler(&buf, &FormatHandlerOptions{
		Level:  slog.LevelDebug,
		Format: fmtMessageAttrs,
	})

	// Pre-attach an attr, then add a group.
	child := h.WithAttrs([]slog.Attr{slog.String("pid", "1")}).WithGroup("http")

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "req", 0)
	r.AddAttrs(slog.String("method", "GET"))

	err := child.Handle(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	// pid is pre-group so gets prefixed by the group; method is in the group.
	want := "req pid=1 http.method=GET"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
