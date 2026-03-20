package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/roadrunner-server/errors"
	"gopkg.in/natefinch/lumberjack.v2"
)

// ChannelConfig configures loggers per channel.
type ChannelConfig struct {
	// Dedicated channels per logger. By default logger allocated via named logger.
	Channels map[string]*Config `mapstructure:"channels"`
}

// FileLoggerConfig represents configuration for the file logger backed by
// lumberjack for log rotation.
type FileLoggerConfig struct {
	// LogOutput is the file to write logs to. Uses <processname>-lumberjack.log
	// in os.TempDir() if empty.
	LogOutput string `mapstructure:"log_output"`

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int `mapstructure:"max_size"`

	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.  Note that a day is defined as 24
	// hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	MaxAge int `mapstructure:"max_age"`

	// MaxBackups is the maximum number of old log files to retain. The default
	// is to retain all old log files (though MaxAge may still cause them to get
	// deleted).
	MaxBackups int `mapstructure:"max_backups"`

	// Compress determines if the rotated log files should be compressed using
	// gzip. The default is not to perform compression.
	Compress bool `mapstructure:"compress"`
}

// InitDefaults fills zero-value fields with sensible defaults.
func (fl *FileLoggerConfig) InitDefaults() *FileLoggerConfig {
	if fl.LogOutput == "" {
		fl.LogOutput = os.TempDir()
	}
	if fl.MaxSize == 0 {
		fl.MaxSize = 100
	}
	if fl.MaxAge == 0 {
		fl.MaxAge = 24
	}
	if fl.MaxBackups == 0 {
		fl.MaxBackups = 10
	}
	return fl
}

// Config holds the logger configuration for a single channel.
type Config struct {
	// Mode configures logger based on some default template (development,
	// production, raw, off, none).
	Mode Mode `mapstructure:"mode"`

	// Level is the minimum enabled logging level.
	Level string `mapstructure:"level"`

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

	// FileLogger options for lumberjack-based file rotation.
	FileLogger *FileLoggerConfig `mapstructure:"file_logger_options"`
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

	// File Logger: use lumberjack writer with JSON handler.
	if cfg.FileLogger != nil {
		cfg.FileLogger.InitDefaults()

		lj := &lumberjack.Logger{
			Filename:   cfg.FileLogger.LogOutput,
			MaxSize:    cfg.FileLogger.MaxSize,
			MaxAge:     cfg.FileLogger.MaxAge,
			MaxBackups: cfg.FileLogger.MaxBackups,
			Compress:   cfg.FileLogger.Compress,
		}

		closers = append(closers, lj)

		return &BuildResult{
			Logger:  slog.New(newJSONHandler(lj, level)),
			Closers: closers,
		}, nil
	}

	var handler slog.Handler
	switch mode {
	case production:
		handler = newJSONHandler(w, level)
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
		cfg.Level = "debug"
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

// parseLevel converts a level string to the corresponding [slog.Level].
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}
