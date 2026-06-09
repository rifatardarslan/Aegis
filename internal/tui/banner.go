package tui

import (
	"fmt"
	"strings"
)

// AegisASCII is the large block-character title banner displayed on the dashboard.
const AegisASCII = `    ___    ______  ______  ____  _____
   /   |  / ____/ / ____/ /  _/ / ___/
  / /| | / __/   / / __   / /   \__ \ 
 / ___ |/ /___  / /_/ / _/ /   ___/ / 
/_/  |_/_____/  \____/ /___/  /____/  `

// AegisTagline is the subtitle shown below the ASCII art.
const AegisTagline = "ZERO-TRUST  ·  MEMORY-ONLY  ·  P2P SECURE TERMINAL"

// RenderBanner returns the full ASCII art banner with BannerStyle applied.
func RenderBanner() string {
	return BannerStyle.Render(AegisASCII)
}

// ScanBar renders an animated progress bar for the connection-scanning animation.
//
//	step     – current step (0 ≤ step ≤ total)
//	total    – maximum step count
//	barWidth – interior character width of the bar
func ScanBar(step, total, barWidth int) string {
	if barWidth < 4 {
		barWidth = 4
	}
	if total <= 0 {
		total = 1
	}

	filled := (step * barWidth) / total
	if filled > barWidth {
		filled = barWidth
	}

	var bar string
	if filled >= barWidth {
		bar = strings.Repeat("═", barWidth)
	} else {
		bar = strings.Repeat("═", filled) + "▶"
		rem := barWidth - filled - 1
		if rem > 0 {
			bar += strings.Repeat(" ", rem)
		}
	}

	pct := (step * 100) / total
	if pct > 100 {
		pct = 100
	}

	return PrimaryText.Render("[") +
		PrimaryBold.Render(bar) +
		PrimaryText.Render(fmt.Sprintf("] %3d%%", pct))
}
