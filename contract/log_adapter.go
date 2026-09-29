package contract

import (
	"bytes"

	"github.com/welllog/golib/strz"
	"go.uber.org/fx/fxevent"
)

type fxLogger struct {
	logger Logger
}

func (f fxLogger) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	msg := p
	if msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
		if len(msg) > 0 && msg[len(msg)-1] == '\r' {
			msg = msg[:len(msg)-1]
		}
	}

	if bytes.HasPrefix(msg, []byte("[Fx] ERROR")) {
		f.logger.Error(strz.UnsafeString(msg))
	} else {
		f.logger.Debug(strz.UnsafeString(msg))
	}

	return len(p), nil
}

func NewFxLogger(logger Logger) func() fxevent.Logger {
	return func() fxevent.Logger {
		return &fxevent.ConsoleLogger{
			W: fxLogger{
				logger: logger,
			},
		}
	}
}
