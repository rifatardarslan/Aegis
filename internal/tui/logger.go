package tui

import (
	"fmt"
	"os"
	"sync/atomic"
)

var debugEnabled atomic.Bool

func SetDebug(enabled bool) { debugEnabled.Store(enabled) }

// Only fixed diagnostic categories are emitted; no arguments or disk logs.
func LogDebug(category string, _ ...any) {
	if debugEnabled.Load() {
		switch category {
		case "session closed", "search failed", "transfer failed":
			fmt.Fprintln(os.Stderr, "aegis:", category)
		}
	}
}
