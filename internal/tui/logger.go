package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LogDebug writes a formatted log message to the tmp/debug.log file.
func LogDebug(format string, args ...interface{}) {
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	logDir := filepath.Join(wd, "tmp")
	_ = os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, "debug.log")

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(f, "%s: %s\n", time.Now().Format("2006-01-02 15:04:05.000"), msg)
}
