package logger

import (
	"regexp"
	"testing"

	"tests/helpers"

	"github.com/stretchr/testify/require"
)

const customFormatLog = "custom-format.log"

// customFormatLine matches the shape the config asks for:
//
//	%time% [%level%] %message% %attrs%
//
// with time_format "2006-01-02 15:04:05".
var customFormatLine = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \[(INFO|WARN|ERROR)] `)

// TestCustomFormatIsApplied asserts the rendered lines actually follow the
// configured layout. The old test booted this config and asserted nothing about
// the output, so any format string would have passed.
func TestCustomFormatIsApplied(t *testing.T) {
	removeLog(t, customFormatLog)

	helpers.Start(t, "configs/.rr-custom-format.yaml", []any{&TestPlugin{}})

	content := helpers.WaitForFile(t, customFormatLog, func(s string) bool {
		return customFormatLine.MatchString(firstLine(s))
	})

	line := firstLine(content)
	require.Regexp(t, customFormatLine, line, "line does not follow the configured format")
	require.NotContains(t, line, `"level":`, "a custom format must not fall back to json")
}

// TestCustomFormatHonoursLevel keeps debug records out: the config asks for
// info, and the fixture logs at debug as well.
func TestCustomFormatHonoursLevel(t *testing.T) {
	removeLog(t, customFormatLog)

	helpers.Start(t, "configs/.rr-custom-format.yaml", []any{&TestPlugin{}})

	content := helpers.WaitForFile(t, customFormatLog, func(s string) bool {
		return customFormatLine.MatchString(firstLine(s))
	})

	require.NotContains(t, content, "[DEBUG]", "debug records must be filtered out at info level")
}

func firstLine(s string) string {
	for i := range len(s) {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
