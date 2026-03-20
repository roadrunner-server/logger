package logger

import (
	"context"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FormatHandlerOptions configures a [FormatHandler].
type FormatHandlerOptions struct {
	// Level is the minimum enabled logging level.
	Level slog.Leveler
	// Format is the format string with %placeholder% tokens.
	Format string
	// TimeLayout is the Go time layout used for the %time% placeholder.
	// Defaults to [time.RFC3339] when empty.
	TimeLayout string
	// LineEnding is appended after each formatted record. Defaults to "\n".
	// Set to a non-nil pointer to an empty string to suppress the line ending.
	LineEnding *string
}

// FormatHandler is an [slog.Handler] that renders log records according to a
// user-defined format string containing %placeholder% tokens such as %time%,
// %level%, %message%, %attrs%, %logger%, %source_file%, %source_line%, and
// %source_func%.
//
// Unknown placeholders are left in the output verbatim.
type FormatHandler struct {
	w          io.Writer
	level      slog.Leveler
	format     string
	timeLayout string
	lineEnding string
	preAttrs   []slog.Attr
	groups     []string
	grpPfx     string // cached dot-joined prefix from groups ("g1.g2." or "")
	mu         *sync.Mutex

	// Optimization flags set once at construction to avoid scanning the format
	// string on every Handle call.
	needsSource bool
	needsAttrs  bool
	needsLogger bool
}

// NewFormatHandler returns a [FormatHandler] that writes to w using the given
// options. The format string is scanned once to determine which placeholders
// are present so that expensive operations (source lookup, attr rendering) can
// be skipped when not needed.
func NewFormatHandler(w io.Writer, opts *FormatHandlerOptions) *FormatHandler {
	le := "\n"
	if opts.LineEnding != nil {
		le = *opts.LineEnding
	}

	tl := time.RFC3339
	if opts.TimeLayout != "" {
		tl = opts.TimeLayout
	}

	f := opts.Format

	return &FormatHandler{
		w:           w,
		level:       opts.Level,
		format:      f,
		timeLayout:  tl,
		lineEnding:  le,
		mu:          &sync.Mutex{},
		needsSource: strings.Contains(f, "%source_file%") || strings.Contains(f, "%source_line%") || strings.Contains(f, "%source_func%"),
		needsAttrs:  strings.Contains(f, "%attrs%"),
		needsLogger: strings.Contains(f, "%logger%"),
	}
}

// Enabled reports whether the handler is enabled for the given level.
func (h *FormatHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle formats and writes a single log record.
func (h *FormatHandler) Handle(_ context.Context, r slog.Record) error {
	// Time
	var timeStr string
	if !r.Time.IsZero() {
		timeStr = r.Time.Format(h.timeLayout)
	}

	// Source (only resolved when needed).
	var sourceFile, sourceLine, sourceFunc string
	if h.needsSource && r.PC != 0 {
		frame, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		sourceFile = frame.File
		sourceLine = strconv.Itoa(frame.Line)
		if idx := strings.LastIndex(frame.Function, "."); idx >= 0 {
			sourceFunc = frame.Function[idx+1:]
		} else {
			sourceFunc = frame.Function
		}
	}

	// Attrs — collect pre-attached attrs and record attrs.
	var loggerName string
	var attrsStr string

	if h.needsAttrs || h.needsLogger {
		var sb strings.Builder
		first := true

		appendAttr := func(key string, v slog.Value) {
			// Extract logger name and exclude from %attrs%.
			if h.needsLogger && key == "logger" {
				loggerName = v.String()
				return
			}

			if h.needsAttrs {
				if !first {
					sb.WriteByte(' ')
				}
				sb.WriteString(key)
				sb.WriteByte('=')
				sb.WriteString(formatAttrValue(v))
				first = false
			}
		}

		// Pre-attached attrs already have their group prefix baked in.
		for _, a := range h.preAttrs {
			a.Value = a.Value.Resolve()
			if a.Equal(slog.Attr{}) {
				continue
			}
			appendAttr(a.Key, a.Value)
		}

		// Record attrs get the current full group prefix.
		r.Attrs(func(a slog.Attr) bool {
			a.Value = a.Value.Resolve()
			if !a.Equal(slog.Attr{}) {
				appendAttr(h.grpPfx+a.Key, a.Value)
			}
			return true
		})

		attrsStr = sb.String()
	}

	line := strings.NewReplacer(
		"%time%", timeStr,
		"%level%", r.Level.String(),
		"%message%", r.Message,
		"%attrs%", attrsStr,
		"%logger%", loggerName,
		"%source_file%", sourceFile,
		"%source_line%", sourceLine,
		"%source_func%", sourceFunc,
	).Replace(h.format) + h.lineEnding

	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := io.WriteString(h.w, line)
	return err
}

// WithAttrs returns a new [FormatHandler] that includes the given attributes in
// every subsequent record. The current group prefix is baked into attr keys so
// that groups added later do not retroactively affect them.
func (h *FormatHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	c := h.clone()
	for _, a := range attrs {
		c.preAttrs = append(c.preAttrs, slog.Attr{
			Key:   h.grpPfx + a.Key,
			Value: a.Value,
		})
	}
	return c
}

// WithGroup returns a new [FormatHandler] that qualifies subsequent attributes
// with the given group name using dot-separated keys.
func (h *FormatHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := h.clone()
	c.groups = append(c.groups, name)
	c.grpPfx = strings.Join(c.groups, ".") + "."
	return c
}

// clone creates a shallow copy of the handler, sharing the mutex and writer but
// with independent slices for preAttrs and groups.
func (h *FormatHandler) clone() *FormatHandler {
	return &FormatHandler{
		w:           h.w,
		level:       h.level,
		format:      h.format,
		timeLayout:  h.timeLayout,
		lineEnding:  h.lineEnding,
		preAttrs:    append([]slog.Attr(nil), h.preAttrs...),
		groups:      append([]string(nil), h.groups...),
		grpPfx:      h.grpPfx,
		mu:          h.mu, // shared across clones
		needsSource: h.needsSource,
		needsAttrs:  h.needsAttrs,
		needsLogger: h.needsLogger,
	}
}

// formatAttrValue converts an [slog.Value] to its string representation.
// Group values are rendered as nested dot-separated key=value pairs.
func formatAttrValue(v slog.Value) string {
	if v.Kind() == slog.KindGroup {
		var sb strings.Builder
		for i, a := range v.Group() {
			if i > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(a.Key)
			sb.WriteByte('=')
			sb.WriteString(formatAttrValue(a.Value))
		}
		return sb.String()
	}
	return v.String()
}
