package logger

import (
	"testing"

	"tests/helpers"

	httpPlugin "github.com/roadrunner-server/http/v6"
	"github.com/roadrunner-server/server/v6"
)

// The configs below exercise the paths where the logs section is absent or
// minimal. The plugin has to fall back to its defaults rather than fail, so the
// assertion is that the container boots and the fixture plugin can log through
// it — Start fails the test if any plugin reports an error.

// TestBootsWithRawMode covers mode: raw, which writes the message verbatim with
// no envelope.
func TestBootsWithRawMode(t *testing.T) {
	helpers.Start(t, "configs/.rr.yaml", []any{&TestPlugin{}})
}

// TestBootsWithRawModeAndHTTP pairs raw mode with a real worker pool, so the
// handler formats records coming from another plugin too.
func TestBootsWithRawModeAndHTTP(t *testing.T) {
	helpers.Start(t,
		"configs/.rr-raw-mode.yaml",
		[]any{&server.Plugin{}, &httpPlugin.Plugin{}},
		helpers.WithTCPProbe("127.0.0.1:34999"),
	)
}

// TestBootsWithoutLogsSection covers a config carrying no logs block at all.
func TestBootsWithoutLogsSection(t *testing.T) {
	helpers.Start(t, "configs/.rr-no-logger.yaml", []any{&TestPlugin{}})
}

// TestBootsWithEmptyLogsSection covers a logs block present but empty.
func TestBootsWithEmptyLogsSection(t *testing.T) {
	helpers.Start(t, "configs/.rr-no-logger2.yaml", []any{&TestPlugin{}})
}
