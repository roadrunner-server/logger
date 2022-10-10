package logger

import (
	endure "github.com/roadrunner-server/endure/pkg/container"
	"github.com/roadrunner-server/errors"
	"go.uber.org/zap"
)

// PluginName declares plugin name.
const PluginName = "logs"

type Configurer interface {
	// UnmarshalKey takes a single key and unmarshal it into a Struct.
	UnmarshalKey(name string, out any) error

	// Has checks if config section exists.
	Has(name string) bool
}

// Plugin manages zap logger.
type Plugin struct {
	base     *zap.Logger
	cfg      *Config
	channels ChannelConfig
}

// Init logger service.
func (z *Plugin) Init(cfg Configurer) error {
	const op = errors.Op("config_plugin_init")
	var err error
	// if not configured, configure with default params
	if !cfg.Has(PluginName) {
		z.cfg = &Config{}
		z.cfg.InitDefault()

		z.base, err = z.cfg.BuildLogger()
		if err != nil {
			return errors.E(op, err)
		}

		return nil
	}

	err = cfg.UnmarshalKey(PluginName, &z.cfg)
	if err != nil {
		return errors.E(op, err)
	}

	err = cfg.UnmarshalKey(PluginName, &z.channels)
	if err != nil {
		return errors.E(op, err)
	}

	z.cfg.InitDefault()
	z.base, err = z.cfg.BuildLogger()

	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (z *Plugin) Serve() chan error {
	return make(chan error, 1)
}

func (z *Plugin) Stop() error {
	_ = z.base.Sync()
	return nil
}

// ServiceLogger returns logger dedicated to the specific channel. Similar to Named() but also reads the core params.
func (z *Plugin) ServiceLogger(n endure.Named) (*zap.Logger, error) {
	return z.namedLogger(n.Name())
}

// Provides declares factory methods.
func (z *Plugin) Provides() []any {
	return []any{
		z.ServiceLogger,
	}
}

// Name returns user-friendly plugin name
func (z *Plugin) Name() string {
	return PluginName
}

// namedLogger returns logger bound to the specific channel
func (z *Plugin) namedLogger(name string) (*zap.Logger, error) {
	if cfg, ok := z.channels.Channels[name]; ok {
		l, err := cfg.BuildLogger()
		if err != nil {
			return nil, err
		}
		return l.Named(name), nil
	}

	return z.base.Named(name), nil
}
