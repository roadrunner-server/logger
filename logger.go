package logger

import "log/slog"

// Log implements the [Logger] interface, dispatching named loggers to
// per-channel configurations when available, falling back to the base logger.
type Log struct {
	base     *slog.Logger
	channels ChannelConfig
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
			panic(err)
		}
		return res.Logger.With("logger", name)
	}
	return l.base.With("logger", name)
}
