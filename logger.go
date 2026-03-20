package logger

import (
	"log/slog"
	"sync"
)

// Log implements the [Logger] interface, dispatching named loggers to
// per-channel configurations when available, falling back to the base logger.
type Log struct {
	base     *slog.Logger
	channels ChannelConfig

	mu    sync.Mutex
	cache map[string]*slog.Logger
}

// NewLogger creates a new [Log] with the given channel overrides and base
// logger.
func NewLogger(channels ChannelConfig, base *slog.Logger) *Log {
	return &Log{
		channels: channels,
		base:     base,
		cache:    make(map[string]*slog.Logger),
	}
}

// NamedLogger returns a logger for the given name. If a channel-specific
// configuration exists, a dedicated logger is built from it; otherwise the base
// logger is returned with a "logger" attribute set to name. Channel loggers are
// cached so that repeated calls for the same name reuse the same logger and
// underlying writers.
func (l *Log) NamedLogger(name string) *slog.Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	if cached, ok := l.cache[name]; ok {
		return cached
	}

	var result *slog.Logger
	if cfg, ok := l.channels.Channels[name]; ok {
		res, err := cfg.BuildLogger()
		if err != nil {
			panic(err)
		}
		result = res.Logger.With("logger", name)
	} else {
		result = l.base.With("logger", name)
	}

	l.cache[name] = result
	return result
}
