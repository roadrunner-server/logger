package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/roadrunner-server/errors"
)

const (
	levelDebug = "debug"
	levelInfo  = "info"
)

// ChannelConfig configures loggers per channel.
type ChannelConfig struct {
	// Dedicated channels per logger. By default logger allocated via named logger.
	Channels map[string]*Config `mapstructure:"channels"`
}

// Config holds the logger configuration for a single channel.
type Config struct {
	// Mode configures logger based on some default template (development,
	// production, raw, off, none).
	Mode Mode `mapstructure:"mode"`

	// Level is the minimum enabled logging level.
	Level string `mapstructure:"level"`

	// Format is a custom format string with %placeholder% tokens (e.g.
	// "%time% [%level%] %message% %attrs%"). When set, it overrides Mode for
	// handler selection.
	Format string `mapstructure:"format"`

	// TimeFormat is a Go time layout used for the %time% placeholder in a
	// custom format string. Defaults to [time.RFC3339].
	TimeFormat string `mapstructure:"time_format"`

	// LineEnding for log entries. Default: "\n".
	LineEnding string `mapstructure:"line_ending"`

	// SkipLineEnding determines if the logger should skip appending the default
	// line ending to each log entry.
	SkipLineEnding bool `mapstructure:"skip_line_ending"`

	// Encoding sets the logger's encoding. Valid values are "json" and
	// "console".
	Encoding string `mapstructure:"encoding"`

	// Output is a list of URLs or file paths to write logging output to.
	Output []string `mapstructure:"output"`

	// ErrorOutput is a list of URLs to write internal logger errors to. The
	// default is standard error.
	ErrorOutput []string `mapstructure:"err_output"`
}

// BuildResult holds the logger and any resources that need cleanup.
type BuildResult struct {
	Logger  *slog.Logger
	Closers []io.Closer
}

// BuildLogger creates an [*slog.Logger] from the configuration.
func (cfg *Config) BuildLogger() (*BuildResult, error) {
	const op = errors.Op("build_logger")

	level := parseLevel(cfg.Level)

	mode := Mode(strings.ToLower(string(cfg.Mode)))

	switch mode {
	case off, none:
		return &BuildResult{Logger: slog.New(slog.DiscardHandler)}, nil
	case production, development, raw:
		// handled below
	default:
		// Unknown mode — fall through to development-like behavior.
	}

	w, closers, err := cfg.resolveOutputWriter()
	if err != nil {
		return nil, errors.E(op, err)
	}

	// Custom format: build a FormatHandler instead of mode-based handlers.
	if cfg.Format != "" {
		return &BuildResult{
			Logger: slog.New(NewFormatHandler(w, &FormatHandlerOptions{
				Level:      level,
				Format:     cfg.Format,
				TimeLayout: cfg.TimeFormat,
				LineEnding: new(cfg.lineEnding()),
			})),
			Closers: closers,
		}, nil
	}

	var handler slog.Handler
	switch mode {
	case production:
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	case raw:
		handler = NewRawHandler(w, level)
	case off, none:
		// Already handled above; included for exhaustive linter.
		handler = slog.DiscardHandler
	case development:
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	default:
		// Unknown mode — fall through to development-like behavior.
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	}

	return &BuildResult{
		Logger:  slog.New(handler),
		Closers: closers,
	}, nil
}

// InitDefault sets default values for empty fields.
func (cfg *Config) InitDefault() {
	if cfg.Mode == "" {
		cfg.Mode = development
	}
	if cfg.Level == "" {
		cfg.Level = levelDebug
	}
}

// resolveOutputWriter builds an [io.Writer] from the Output configuration.
// Supported values: "stderr", "stdout", or a file path. Multiple outputs are
// combined with [io.MultiWriter]. Returns the writer and any closers that must
// be closed when the logger is shut down.
func (cfg *Config) resolveOutputWriter() (io.Writer, []io.Closer, error) {
	if len(cfg.Output) == 0 {
		return os.Stderr, nil, nil
	}

	writers := make([]io.Writer, 0, len(cfg.Output))
	var closers []io.Closer

	for _, out := range cfg.Output {
		switch strings.ToLower(strings.TrimSpace(out)) {
		case "stderr":
			writers = append(writers, os.Stderr)
		case "stdout":
			writers = append(writers, os.Stdout)
		default:
			f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				// Close any files we already opened before returning.
				for _, c := range closers {
					_ = c.Close()
				}
				return nil, nil, err
			}
			writers = append(writers, f)
			closers = append(closers, f)
		}
	}

	if len(writers) == 1 {
		return writers[0], closers, nil
	}
	return io.MultiWriter(writers...), closers, nil
}

// lineEnding returns the effective line ending for log entries, respecting the
// SkipLineEnding and LineEnding configuration fields.
func (cfg *Config) lineEnding() string {
	if cfg.SkipLineEnding {
		return ""
	}
	if cfg.LineEnding != "" {
		return cfg.LineEnding
	}
	return "\n"
}

// parseLevel converts a level string to the corresponding [slog.Level].
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case levelDebug:
		return slog.LevelDebug
	case levelInfo:
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}
