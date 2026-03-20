package logger

import (
	"context"
	"io"
	"log/slog"

	"github.com/roadrunner-server/endure/v2/dep"
	"github.com/roadrunner-server/errors"
)

// PluginName declares plugin name.
const PluginName = "logs"

// Configurer provides access to the application configuration.
type Configurer interface {
	// UnmarshalKey takes a single key and unmarshal it into a Struct.
	UnmarshalKey(name string, out any) error
	// Has checks if a config section exists.
	Has(name string) bool
}

// Logger is the main logger interface to provide a named (per-plugin) logger.
type Logger interface {
	NamedLogger(name string) *slog.Logger
}

// Plugin manages the slog-based logger.
type Plugin struct {
	base     *slog.Logger
	cfg      *Config
	channels ChannelConfig
	closers  []io.Closer
	logs     []*Log
}

// Init logger service.
func (p *Plugin) Init(cfg Configurer) error {
	const op = errors.Op("config_plugin_init")
	var err error

	// if not configured, configure with default params
	if !cfg.Has(PluginName) {
		p.cfg = &Config{}
		p.cfg.InitDefault()

		res, buildErr := p.cfg.BuildLogger()
		if buildErr != nil {
			return errors.E(op, buildErr)
		}

		p.base = res.Logger
		p.closers = res.Closers

		return nil
	}

	err = cfg.UnmarshalKey(PluginName, &p.cfg)
	if err != nil {
		return errors.E(op, err)
	}

	err = cfg.UnmarshalKey(PluginName, &p.channels)
	if err != nil {
		return errors.E(op, err)
	}

	p.cfg.InitDefault()

	res, buildErr := p.cfg.BuildLogger()
	if buildErr != nil {
		return errors.E(op, buildErr)
	}

	p.base = res.Logger
	p.closers = res.Closers

	return nil
}

// Serve starts the plugin (no-op for the logger).
func (p *Plugin) Serve() chan error {
	return make(chan error, 1)
}

// Stop gracefully shuts down the plugin, closing any file handles opened for
// log output — both root-level and per-channel closers.
func (p *Plugin) Stop(context.Context) error {
	for _, l := range p.logs {
		_ = l.Close()
	}
	for _, c := range p.closers {
		_ = c.Close()
	}
	return nil
}

// Provides declares the services this plugin exports.
func (p *Plugin) Provides() []*dep.Out {
	return []*dep.Out{
		dep.Bind((*Logger)(nil), p.ServiceLogger),
	}
}

// ServiceLogger returns a logger dedicated to the specific channel.
func (p *Plugin) ServiceLogger() *Log {
	l := NewLogger(p.channels, p.base)
	p.logs = append(p.logs, l)
	return l
}

// Name returns a user-friendly plugin name.
func (p *Plugin) Name() string {
	return PluginName
}
