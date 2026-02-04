package errutil

import (
	"log/slog"
)

// LogMsg logs the error with a custom message if it is not nil.
func LogMsg(err error, msg string, args ...any) {
	if err != nil {
		allArgs := append([]any{"error", err}, args...)
		slog.Warn(msg, allArgs...)
	}
}
