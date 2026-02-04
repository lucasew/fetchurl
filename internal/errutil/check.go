package errutil

import (
	"log/slog"
)

// Log logs the error if it is not nil.
// It is intended to be used in defer statements or where an error cannot be handled but should not be silently ignored.
func Log(err error) {
	if err != nil {
		slog.Warn("Ignored error", "error", err)
	}
}

// LogMsg logs the error with a custom message if it is not nil.
func LogMsg(err error, msg string, args ...any) {
	if err != nil {
		allArgs := append([]any{"error", err}, args...)
		slog.Warn(msg, allArgs...)
	}
}
