package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var defaultLevel atomic.Value

func init() {
	defaultLevel.Store(slog.LevelInfo)
	setupLogger()
}

func setupLogger() {
	handler := &ansiHandler{
		writer: os.Stdout,
	}
	slog.SetDefault(slog.New(handler))
}

// SetLevel 动态调整当前输出的日志级别 (debug | info | warn | error)
func SetLevel(levelStr string) {
	switch strings.ToLower(levelStr) {
	case "debug":
		defaultLevel.Store(slog.LevelDebug)
	case "warn":
		defaultLevel.Store(slog.LevelWarn)
	case "error":
		defaultLevel.Store(slog.LevelError)
	default:
		defaultLevel.Store(slog.LevelInfo)
	}
}

type ansiHandler struct {
	writer io.Writer
}

func (h *ansiHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	minLvl, ok := defaultLevel.Load().(slog.Level)
	if !ok {
		return lvl >= slog.LevelInfo
	}
	return lvl >= minLvl
}

func (h *ansiHandler) Handle(_ context.Context, r slog.Record) error {
	var colorPrefix string
	switch r.Level {
	case slog.LevelDebug:
		colorPrefix = "\033[36m[DEBUG]\033[0m" // 灰蓝/青色
	case slog.LevelInfo:
		colorPrefix = "\033[32m[INFO ]\033[0m" // 翠绿
	case slog.LevelWarn:
		colorPrefix = "\033[33m[WARN ]\033[0m" // 暖黄
	case slog.LevelError:
		colorPrefix = "\033[31m[ERROR]\033[0m" // 红色
	default:
		colorPrefix = "[LOG]"
	}

	cstZone := time.FixedZone("CST", 8*3600)
	timeStr := r.Time.In(cstZone).Format("2006-01-02 15:04:05.000")
	var attrs strings.Builder
	r.Attrs(func(a slog.Attr) bool {
		attrs.WriteString(fmt.Sprintf(" %s=%v", a.Key, a.Value.Any()))
		return true
	})

	_, err := fmt.Fprintf(h.writer, "%s %s %s%s\n", timeStr, colorPrefix, r.Message, attrs.String())
	return err
}

func (h *ansiHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *ansiHandler) WithGroup(name string) slog.Handler      { return h }

func Debug(msg string, args ...any) { slog.Debug(msg, args...) }
func Info(msg string, args ...any)  { slog.Info(msg, args...) }
func Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func Error(msg string, args ...any) { slog.Error(msg, args...) }
