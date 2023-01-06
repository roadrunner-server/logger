package logger

import (
	"go.uber.org/zap"
)

type Log struct {
	base     *zap.Logger
	channels ChannelConfig
}

func NewLogger(channels ChannelConfig, base *zap.Logger) *Log {
	return &Log{
		channels: channels,
		base:     base,
	}
}

func (l *Log) NamedLogger(name string) *zap.Logger {
	if cfg, ok := l.channels.Channels[name]; ok {
		ll, err := cfg.BuildLogger()
		if err != nil {
			panic(err)
		}
		return ll.Named(name)
	}

	return l.base.Named(name)
}
