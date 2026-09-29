package contract

import (
	"fmt"
	"testing"

	"github.com/welllog/golib/testz"
)

type mockLogger struct {
	debugMsgs []string
	errorMsgs []string
}

func (m *mockLogger) Debug(args ...any) {
	m.debugMsgs = append(m.debugMsgs, fmt.Sprint(args...))
}

func (m *mockLogger) Debugf(format string, args ...any) {}

func (m *mockLogger) Info(args ...any) {}

func (m *mockLogger) Infof(format string, args ...any) {}

func (m *mockLogger) Warn(args ...any) {}

func (m *mockLogger) Warnf(format string, args ...any) {}

func (m *mockLogger) Error(args ...any) {
	m.errorMsgs = append(m.errorMsgs, fmt.Sprint(args...))
}

func (m *mockLogger) Errorf(format string, args ...any) {}

func TestFxLogger_Write(t *testing.T) {
	l := &mockLogger{}
	fl := fxLogger{logger: l}

	// Case 1: Empty slice
	n, err := fl.Write([]byte{})
	testz.Nil(t, err)
	testz.Equal(t, 0, n)
	testz.Equal(t, 0, len(l.debugMsgs))
	testz.Equal(t, 0, len(l.errorMsgs))

	// Case 2: String ending with \n
	n, err = fl.Write([]byte("info message\n"))
	testz.Nil(t, err)
	testz.Equal(t, 13, n)
	testz.Equal(t, "info message", l.debugMsgs[0])

	// Case 3: String ending with \r\n
	n, err = fl.Write([]byte("windows message\r\n"))
	testz.Nil(t, err)
	testz.Equal(t, 17, n)
	testz.Equal(t, "windows message", l.debugMsgs[1])

	// Case 4: String without trailing newline - must NOT truncate last character!
	n, err = fl.Write([]byte("partial text"))
	testz.Nil(t, err)
	testz.Equal(t, 12, n)
	testz.Equal(t, "partial text", l.debugMsgs[2])

	// Case 5: Error message
	n, err = fl.Write([]byte("[Fx] ERROR failed to start\n"))
	testz.Nil(t, err)
	testz.Equal(t, 27, n)
	testz.Equal(t, "[Fx] ERROR failed to start", l.errorMsgs[0])
}
