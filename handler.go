package logger

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// RawHandler is an [slog.Handler] that outputs only the log message followed by
// a newline. Structured attributes and groups are discarded. This is used for
// the "raw" logger mode where callers want plain, undecorated output.
type RawHandler struct {
	w     io.Writer
	level slog.Leveler
	mu    sync.Mutex
}

// NewRawHandler returns a [RawHandler] writing to w. Only records at or above
// the given level are emitted.
func NewRawHandler(w io.Writer, level slog.Leveler) *RawHandler {
	return &RawHandler{
		w:     w,
		level: level,
	}
}

func (h *RawHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *RawHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, err := io.WriteString(h.w, r.Message); err != nil {
		return err
	}

	if !strings.HasSuffix(r.Message, "\n") {
		if _, err := io.WriteString(h.w, "\n"); err != nil {
			return err
		}
	}

	return nil
}

// WithAttrs returns the same handler — raw mode discards attributes.
func (h *RawHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

// WithGroup returns the same handler — raw mode discards groups.
func (h *RawHandler) WithGroup(string) slog.Handler { return h }

// newJSONHandler creates a [slog.JSONHandler] configured for production output.
// Keys are remapped: time → ts (Unix epoch nanoseconds), level stays lowercase,
// message → msg.
func newJSONHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				a.Key = "ts"
				a.Value = slog.Int64Value(a.Value.Time().UnixNano())
			case slog.LevelKey:
				a.Value = slog.StringValue(strings.ToLower(a.Value.String()))
			case slog.MessageKey:
				a.Key = "msg"
			}
			return a
		},
	})
}
