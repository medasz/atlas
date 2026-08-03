package logger

import (
	"testing"
)

func TestLoggerOutput(t *testing.T) {
	SetLevel("debug")
	Debug("test debug msg", "key", "val")
	Info("test info msg", "target", "127.0.0.1", "port", 80)
	Warn("test warn msg", "err", "timeout")
	Error("test error msg", "code", 500)
}
