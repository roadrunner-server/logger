package logger

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/roadrunner-server/config/v6"
	"github.com/roadrunner-server/endure/v2"
	httpPlugin "github.com/roadrunner-server/http/v6"
	"github.com/roadrunner-server/logger/v6"
	"github.com/roadrunner-server/rpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger(t *testing.T) {
	container := endure.New(slog.LevelDebug)

	vp := &config.Plugin{}
	vp.Path = "configs/.rr.yaml"

	err := container.RegisterAll(
		vp,
		&TestPlugin{},
		&logger.Plugin{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	})

	stopCh <- struct{}{}
	wg.Wait()
}

func TestLoggerRawErr(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2025.1.11",
		Path:    "configs/.rr-raw-mode.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&logger.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	stopCh := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	})

	assert.EventuallyWithT(t, func(ct *assert.CollectT) {
		req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:34999", nil)
		assert.NoError(ct, reqErr)

		resp, doErr := http.DefaultClient.Do(req)
		assert.NoError(ct, doErr)

		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}, 10*time.Second, 100*time.Millisecond)

	stopCh <- struct{}{}
	wg.Wait()
}

func TestLoggerNoConfig(t *testing.T) {
	container := endure.New(slog.LevelDebug)

	vp := &config.Plugin{}
	vp.Path = "configs/.rr-no-logger.yaml"

	err := container.RegisterAll(
		vp,
		&TestPlugin{},
		&logger.Plugin{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	})

	stopCh <- struct{}{}
	wg.Wait()
}

// TestLoggerNoConfig2 verifies no panic when plugins are disabled due to
// missing dependencies.
func TestLoggerNoConfig2(t *testing.T) {
	container := endure.New(slog.LevelDebug)

	vp := &config.Plugin{}
	vp.Path = "configs/.rr-no-logger2.yaml"

	err := container.RegisterAll(
		vp,
		&rpc.Plugin{},
		&logger.Plugin{},
		&httpPlugin.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	})

	stopCh <- struct{}{}
	wg.Wait()
}

func TestFileLogger(t *testing.T) {
	container := endure.New(slog.LevelDebug)

	vp := &config.Plugin{}
	vp.Path = "configs/.rr-file-logger.yaml"

	err := container.RegisterAll(
		vp,
		&rpc.Plugin{},
		&logger.Plugin{},
		&httpPlugin.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	})

	time.Sleep(time.Second * 2)
	t.Run("HTTPEchoReq", httpEcho)

	f, err := os.ReadFile("test.log")
	if err != nil {
		t.Fatal(err)
	}

	assert.Contains(t, string(f), "worker constructed")
	assert.Contains(t, string(f), "201 GET")

	_ = os.Remove("test.log")

	stopCh <- struct{}{}
	wg.Wait()
}

func TestLoggerCustomFormat(t *testing.T) {
	container := endure.New(slog.LevelDebug)

	vp := &config.Plugin{}
	vp.Path = "configs/.rr-custom-format.yaml"

	err := container.RegisterAll(
		vp,
		&TestPlugin{},
		&logger.Plugin{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	})

	stopCh <- struct{}{}
	wg.Wait()
}

func httpEcho(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:54224?hello=world", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, http.StatusCreated, r.StatusCode)

	err = r.Body.Close()
	assert.NoError(t, err)
}

func TestMarshalObjectLogging(t *testing.T) {
	container := endure.New(slog.LevelDebug)

	vp := &config.Plugin{}
	vp.Path = "configs/.rr-file-logger.yaml"

	err := container.RegisterAll(
		vp,
		&TestPlugin{},
		&logger.Plugin{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	})

	time.Sleep(time.Second * 2)

	f, err := os.ReadFile("test.log")
	if err != nil {
		t.Fatal(err)
	}

	assert.Contains(t, string(f), "Example field error")
	assert.Equal(t, 4, strings.Count(string(f), "Example field error"))

	stopCh <- struct{}{}
	wg.Wait()
	_ = os.Remove("test.log")
}
