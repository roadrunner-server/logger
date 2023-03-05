package logger

import (
	"fmt"
	"strings"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

type customFormatEncoder struct {
	format string
	zapcore.Encoder
}

func (e *customFormatEncoder) Clone() zapcore.Encoder {
	return &customFormatEncoder{Encoder: e.Encoder.Clone()}
}

func (e *customFormatEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	str := e.format
	replace := map[string]string{
		"%level_name%":      entry.Level.CapitalString(),
		"%message%":         entry.Message,
		"%time%":            fmt.Sprint(entry.Time.UnixMilli()),
		"%stack%":           entry.Stack,
		"%caller_file%":     entry.Caller.File,
		"%caller_function%": entry.Caller.Function,
	}

	for s, r := range replace {
		str = strings.Replace(str, s, r, -1)
	}

	var contextValues []string
	for _, v := range fields {
		contextValues = append(contextValues, v.String)
	}

	str = strings.Replace(str, "%context%", strings.Join(contextValues, " "), -1)

	pool := buffer.NewPool()
	buf := pool.Get()
	buf.AppendString(str)
	buf.AppendByte('\n')

	return buf, nil
}
