package helpers

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/roadrunner-server/config/v6"
	"github.com/roadrunner-server/endure/v2"
	"github.com/roadrunner-server/logger/v6"
	"github.com/stretchr/testify/require"
)

const (
	// defaultConfigVersion is the config schema version used by the test configs.
	defaultConfigVersion = "2024.2.0"
	// probeTimeout caps how long Start waits for the server to answer the probe.
	probeTimeout = time.Second * 15
	probeTick    = time.Millisecond * 20
	probeDial    = time.Second
	// fileTimeout bounds the poll for a log file to reach its expected contents.
	fileTimeout = time.Second * 15
	fileTick    = time.Millisecond * 50
)

// bootCfg holds the options applied to a container before it is started.
type bootCfg struct {
	version  string
	logLevel slog.Level
	probe    func(ctx context.Context) bool
}

// Option customizes the container built by Start.
type Option func(*bootCfg)

// WithConfigVersion overrides the config schema version.
func WithConfigVersion(v string) Option {
	return func(b *bootCfg) { b.version = v }
}

// WithLogLevel sets the endure container log level (debug by default).
func WithLogLevel(l slog.Level) Option {
	return func(b *bootCfg) { b.logLevel = l }
}

// WithTCPProbe makes Start return only once addr accepts a connection. The
// listener binds after the worker pool is allocated, so this proves readiness
// without sending a request through the pool.
func WithTCPProbe(addr string) Option {
	return func(b *bootCfg) {
		b.probe = func(ctx context.Context) bool {
			d := net.Dialer{Timeout: probeDial}
			conn, err := d.DialContext(ctx, "tcp", addr)
			if err != nil {
				return false
			}

			_ = conn.Close()
			return true
		}
	}
}

// WithProbe makes Start return only once a GET to url gets a response. This
// reaches the worker pool, so tests asserting exact log counts want
// WithTCPProbe instead.
func WithProbe(url string) Option {
	return func(b *bootCfg) {
		b.probe = func(ctx context.Context) bool {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return false
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return false
			}

			_ = resp.Body.Close()
			return true
		}
	}
}

// WaitForFile polls path until it exists and its contents satisfy want, then
// returns the contents. The logger flushes asynchronously, so reading the file
// straight after a request races the writer.
func WaitForFile(t *testing.T, path string, want func(string) bool) string {
	t.Helper()

	var content string
	require.Eventually(t, func() bool {
		b, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		content = string(b)
		return want(content)
	}, fileTimeout, fileTick, "%s never reached the expected contents; last read:\n%s", path, content)

	return content
}

// Start registers the plugins, boots the container and waits for the probe, if
// any, to answer. Errors arriving on the container channel are reported through
// t.Errorf and stop the container, but they do not abort the test.
//
// The returned stop is idempotent and also registered with t.Cleanup, so tests
// asserting on shutdown behavior can stop the container mid-test.
func Start(t *testing.T, cfgPath string, plugins []any, opts ...Option) func() {
	t.Helper()

	cont, bc := newContainer(t, cfgPath, plugins, opts)
	require.NoError(t, cont.Init())

	ch, err := cont.Serve()
	require.NoError(t, err)

	stopCont := sync.OnceValue(cont.Stop)
	done := make(chan struct{})
	wg := &sync.WaitGroup{}

	wg.Go(func() {
		for {
			select {
			case res := <-ch:
				if res == nil {
					return
				}
				t.Errorf("plugin %s reported an error: %v", res.VertexID, res.Error)
				if errS := stopCont(); errS != nil {
					t.Errorf("container stop: %v", errS)
				}
			case <-done:
				if errS := stopCont(); errS != nil {
					t.Errorf("container stop: %v", errS)
				}
				return
			}
		}
	})

	// The drain goroutine calls t.Errorf, so it has to be joined while the test
	// is still running.
	stop := sync.OnceFunc(func() {
		close(done)
		wg.Wait()
	})
	t.Cleanup(stop)

	if bc.probe != nil {
		require.Eventually(t, func() bool { return bc.probe(t.Context()) }, probeTimeout, probeTick, "server did not become ready")
	}

	return stop
}

// newContainer builds the container and registers the config, the logger and
// the caller's plugins. The container is not initialized yet.
func newContainer(t *testing.T, cfgPath string, plugins []any, opts []Option) (*endure.Endure, *bootCfg) {
	t.Helper()

	bc := &bootCfg{version: defaultConfigVersion, logLevel: slog.LevelDebug}
	for _, o := range opts {
		o(bc)
	}

	all := make([]any, 0, 2+len(plugins))
	all = append(all, &config.Plugin{Version: bc.version, Path: cfgPath}, &logger.Plugin{})

	cont := endure.New(bc.logLevel)
	require.NoError(t, cont.RegisterAll(append(all, plugins...)...))

	return cont, bc
}
