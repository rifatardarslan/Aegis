// Package tui provides the Bubbletea-based terminal user interface for Aegis.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Color Palette ─────────────────────────────────────────────────────────────
// Premium Nord theme: clean, elegant, and professional nordic tones.

const (
	ColorPrimary   = lipgloss.Color("#88C0D0") // Frost Blue (Main highlights and accent branding)
	ColorSecondary = lipgloss.Color("#81A1C1") // Glacier Blue (Category headers and secondary items)
	ColorWarning   = lipgloss.Color("#EBCB8B") // Nordic Gold (Identity confirmations and warnings)
	ColorDanger    = lipgloss.Color("#BF616A") // Nordic Red (Error highlights and alert text)
	ColorMuted     = lipgloss.Color("#3B4252") // Polar Night Gray (Subtle borders and frames)
	ColorWhite     = lipgloss.Color("#D8DEE9") // Snow Storm (Message body text)
	ColorSubtle    = lipgloss.Color("#4C566A") // Polar Night Light (Timestamps and secondary labels)
	ColorHighlight = lipgloss.Color("#A3BE8C") // Nordic Green (Peer handles and active markers)
)

// ── Text Styles ───────────────────────────────────────────────────────────────

var (
	PrimaryText   = lipgloss.NewStyle().Foreground(ColorPrimary)
	SecondaryText = lipgloss.NewStyle().Foreground(ColorSecondary)
	MutedText     = lipgloss.NewStyle().Foreground(ColorMuted)
	SubtleText    = lipgloss.NewStyle().Foreground(ColorSubtle)
	WarningText   = lipgloss.NewStyle().Foreground(ColorWarning)
	WhiteText     = lipgloss.NewStyle().Foreground(ColorWhite)

	PrimaryBold = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	WarningBold = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
)

// ── Panel Styles ──────────────────────────────────────────────────────────────

var (
	// PanelStyle is the default rounded panel with subtle slate borders.
	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(0, 1)

	// WarningPanelStyle is used for the identity-verification screen.
	WarningPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorWarning).
				Padding(0, 1)

	// DimPanelStyle is used for secondary panels (system-status sidebar).
	DimPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(0, 1)
)

// ── Label / Value Styles ──────────────────────────────────────────────────────

var (
	LabelStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Width(10)
	ValueStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
)

// ── Chat Styles ───────────────────────────────────────────────────────────────

var (
	TimestampStyle  = lipgloss.NewStyle().Foreground(ColorSubtle)
	HandleSelfStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	HandlePeerStyle = lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true)
	MsgStyle        = lipgloss.NewStyle().Foreground(ColorWhite)
)

// ── Misc ──────────────────────────────────────────────────────────────────────

var (
	BannerStyle  = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	DividerStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	KeyStyle     = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	HelpStyle    = lipgloss.NewStyle().Foreground(ColorSubtle)
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// KeyHelp renders a keyboard shortcut hint, e.g. "[ENTER] Send".
func KeyHelp(key, desc string) string {
	return KeyStyle.Render("["+key+"]") + " " + HelpStyle.Render(desc)
}

// HR renders a horizontal divider of the given character width.
func HR(width int) string {
	if width < 1 {
		return ""
	}
	return DividerStyle.Render(strings.Repeat("─", width))
}
