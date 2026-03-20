package logger

import (
	"context"
	"log/slog"
	"strings"

	"github.com/roadrunner-server/errors"
)

type Configurer interface {
	// UnmarshalKey takes a single key and unmarshal it into a Struct.
	UnmarshalKey(name string, out any) error
	// Has checks if a config section exists.
	Has(name string) bool
}

type Logger interface {
	NamedLogger(name string) *slog.Logger
}

type TestPlugin struct {
	config Configurer
	log    *slog.Logger
}

func (p1 *TestPlugin) Init(cfg Configurer, log Logger) error {
	p1.config = cfg
	p1.log = log.NamedLogger("test")
	return nil
}

func (p1 *TestPlugin) Serve() chan error {
	errCh := make(chan error, 1)
	p1.log.Error("error", slog.Any("error", errors.E(errors.Str("test"))))
	p1.log.Info("error", slog.Any("error", errors.E(errors.Str("test"))))
	p1.log.Debug("error", slog.Any("error", errors.E(errors.Str("test"))))
	p1.log.Warn("error", slog.Any("error", errors.E(errors.Str("test"))))

	p1.log.Error("error", slog.String("error", "Example field error"))
	p1.log.Info("error", slog.String("error", "Example field error"))
	p1.log.Debug("error", slog.String("error", "Example field error"))
	p1.log.Warn("error", slog.String("error", "Example field error"))

	p1.log.Error("error", slog.Any("object", map[string]string{"error": "Example marshaller error"}))
	p1.log.Info("error", slog.Any("object", map[string]string{"error": "Example marshaller error"}))
	p1.log.Debug("error", slog.Any("object", map[string]string{"error": "Example marshaller error"}))
	p1.log.Warn("error", slog.Any("object", map[string]string{"error": "Example marshaller error"}))

	p1.log.Error("error", slog.String("test", ""))
	p1.log.Info("error", slog.String("test", ""))
	p1.log.Debug("error", slog.String("test", ""))
	p1.log.Warn("error", slog.String("test", ""))

	// test the `raw` mode
	messageJSON := []byte(`{"field": "value"}`)
	p1.log.Debug(strings.TrimRight(string(messageJSON), " \n\t"))

	return errCh
}

func (p1 *TestPlugin) Stop(context.Context) error {
	return nil
}

func (p1 *TestPlugin) Name() string {
	return "logger_plugin"
}
