package logger

import (
	"errors"
	"io"
	"log/slog"
	"sync"
)

// Log implements the [Logger] interface, dispatching named loggers to
// per-channel configurations when available, falling back to the base logger.
type Log struct {
	base     *slog.Logger
	channels ChannelConfig

	mu      sync.Mutex
	closers []io.Closer
}

// NewLogger creates a new [Log] with the given channel overrides and base
// logger.
func NewLogger(channels ChannelConfig, base *slog.Logger) *Log {
	return &Log{
		channels: channels,
		base:     base,
	}
}

// NamedLogger returns a logger for the given name. If a channel-specific
// configuration exists, a dedicated logger is built from it; otherwise the base
// logger is returned with a "logger" attribute set to name.
func (l *Log) NamedLogger(name string) *slog.Logger {
	if cfg, ok := l.channels.Channels[name]; ok {
		res, err := cfg.BuildLogger()
		if err != nil {
			l.base.Error("failed to build channel logger, falling back to base", "channel", name, "error", err)
			return l.base.With("logger", name)
		}

		l.mu.Lock()
		l.closers = append(l.closers, res.Closers...)
		l.mu.Unlock()

		return res.Logger.With("logger", name)
	}
	return l.base.With("logger", name)
}

// Close releases all resources (file handles, file writers) opened by
// channel-specific loggers created through [NamedLogger].
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var errs []error
	for _, c := range l.closers {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	l.closers = nil

	return errors.Join(errs...)
}
