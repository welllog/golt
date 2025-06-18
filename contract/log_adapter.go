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

	if bytes.HasPrefix(p, []byte("[Fx] ERROR")) {
		f.logger.Error(strz.UnsafeString(p[:len(p)-1]))
	} else {
		f.logger.Debug(strz.UnsafeString(p[:len(p)-1]))
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
