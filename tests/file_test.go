package logger

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"tests/helpers"

	httpPlugin "github.com/roadrunner-server/http/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/require"
)

const (
	httpAddr = "127.0.0.1:54224"
	fileLog  = "test.log"
)

// removeLog drops a log file before and after a test so a previous run cannot
// satisfy the assertions of the next one.
func removeLog(t *testing.T, path string) {
	t.Helper()

	require.NoError(t, os.RemoveAll(path))
	t.Cleanup(func() { _ = os.Remove(path) })
}

// TestFileOutputCapturesServerAndAccessLogs drives one request and checks both
// the pool's own record and the http access record reached the configured file.
func TestFileOutputCapturesServerAndAccessLogs(t *testing.T) {
	removeLog(t, fileLog)

	helpers.Start(t,
		"configs/.rr-file-logger.yaml",
		[]any{&server.Plugin{}, &httpPlugin.Plugin{}},
		helpers.WithTCPProbe(httpAddr),
	)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+httpAddr+"?hello=world", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() { require.NoError(t, resp.Body.Close()) }()

	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	content := helpers.WaitForFile(t, fileLog, func(s string) bool {
		return strings.Contains(s, "worker is allocated") && strings.Contains(s, `"status":201`)
	})

	require.Contains(t, content, "worker is allocated")
	require.Contains(t, content, `"status":201`)
}

// TestFileOutputRecordsEveryLevel boots the fixture plugin, which logs the same
// attribute at error, info, debug and warn. The config asks for debug, so all
// four records must land.
func TestFileOutputRecordsEveryLevel(t *testing.T) {
	removeLog(t, fileLog)

	helpers.Start(t, "configs/.rr-file-logger.yaml", []any{&server.Plugin{}, &httpPlugin.Plugin{}, &TestPlugin{}})

	content := helpers.WaitForFile(t, fileLog, func(s string) bool {
		return strings.Count(s, "Example field error") >= 4
	})

	require.Equal(t, 4, strings.Count(content, "Example field error"),
		"one record per level was expected, got a different count")
	require.Contains(t, content, "Example marshaller error")
}
