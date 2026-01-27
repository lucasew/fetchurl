## 2026-01-26 - Fix ignored viper.BindPFlag errors

### Issue
Calls to `viper.BindPFlag` in `cmd/fetchurl/server.go` return an error which is currently ignored. This can lead to silent failures where CLI flags are not correctly bound to configuration.

### Root Cause
Missing error check on `viper.BindPFlag`.

### Solution
Wrap `viper.BindPFlag` calls in a helper function `mustBindPFlag` that checks for errors. If an error occurs, it logs the error using `slog.Error` and terminates the application with `os.Exit(1)`.

### Pattern
Error Handling / Configuration

## 2026-01-27 - Remove unused parameters in newResponse

### Issue
The `newResponse` helper method in `internal/proxy/server.go` accepts `algo` and `hash` parameters but does not use them.

### Root Cause
Likely a remnant of a previous implementation or copy-paste from another handler.

### Solution
Remove the unused `algo` and `hash` parameters from the `newResponse` function signature and update all call sites.

### Pattern
Clean Code / Unused Parameters
