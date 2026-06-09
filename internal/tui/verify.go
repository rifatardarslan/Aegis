package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aegis-p2p/aegis/internal/p2p"
)

// ── Model ─────────────────────────────────────────────────────────────────────

// VerifyModel is the model for Screen 2: identity / anti-MitM verification.
type VerifyModel struct {
	width, height int
	localFP       string // SHA-256 fingerprint of local Ed25519 public key (dummy)
	remoteFP      string // SHA-256 fingerprint of remote peer's key (dummy)
	sas           string // Short Authentication String derived from fingerprint XOR
	blinkOn       bool
	p2pNode       *p2p.P2PNode
}

// NewVerify constructs the verify model with placeholder cryptographic data.
func NewVerify() VerifyModel {
	return VerifyModel{
		localFP:  "SHA256:dUMMyLoCaLKeYfINgERpRiNTsPlaCEHoLdErNoDEs=",
		remoteFP: "SHA256:dUMMyRemoteKeYfINgERpRiNTsPlaCEHoLdErPEERs=",
		sas:      "DELTA  ·  ECHO  ·  FOXTROT",
	}
}

func (m VerifyModel) Init() tea.Cmd { return blinkCmd() }

func (m VerifyModel) UpdateSize(w, h int) (VerifyModel, tea.Cmd) {
	m.width, m.height = w, h
	return m, nil
}

func (m VerifyModel) Update(msg tea.Msg) (VerifyModel, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case blinkTickMsg:
		m.blinkOn = !m.blinkOn
		cmds = append(cmds, blinkCmd())
	case tea.KeyMsg:
		switch strings.ToLower(msg.String()) {
		case "y":
			// Both peers independently navigate to chat on their own "Y" press.
			// No unencrypted frame is sent here — all wire communication after handshake
			// must go through the Double Ratchet encrypted channel.
			cmds = append(cmds, func() tea.Msg { return NavigateMsg{To: ScreenChat} })
		case "n", "q", "escape":
			cmds = append(cmds, func() tea.Msg { return NavigateMsg{To: ScreenDashboard} })
		}
	}
	return m, tea.Batch(cmds...)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m VerifyModel) panelWidth() int {
	w := 72
	if m.width >= 110 {
		w = 92
	} else if m.width >= 90 {
		w = 80
	} else if m.width > 0 && m.width < 74 {
		w = m.width - 2
	}
	if w < 54 {
		w = 54
	}
	return w
}

func (m VerifyModel) View() string {
	if m.width == 0 {
		return ""
	}

	iw := m.panelWidth() - 4 // border(2) + padding(2)
	div := HR(iw)

	center := func(s string) string {
		return lipgloss.NewStyle().Width(iw).Align(lipgloss.Center).Render(s)
	}

	headerStyle := lipgloss.NewStyle().
		Background(ColorWarning).
		Foreground(lipgloss.Color("#000000")).
		Bold(true).
		Padding(0, 3).
		Align(lipgloss.Center)

	banner := headerStyle.Width(iw).Render("IDENTITY VERIFICATION REQUIRED")

	fpStyle := PrimaryText

	footer := center(
		KeyHelp("Y", "Confirm & Proceed") + "          " + KeyHelp("N", "Abort & Disconnect"),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		banner,
		"",
		center(WarningText.Render("Verify fingerprints via a secure out-of-band channel. Abort if they mismatch.")),
		"",
		div,
		"",
		SecondaryText.Render("  • YOUR IDENTITY KEY FINGERPRINT:"),
		"    "+fpStyle.Render(m.localFP),
		"",
		div,
		"",
		SecondaryText.Render("  • REMOTE PEER IDENTITY FINGERPRINT:"),
		"    "+fpStyle.Render(m.remoteFP),
		"",
		div,
		"",
		SecondaryText.Render("  • SHORT AUTHENTICATION STRING (SAS):"),
		"",
		center(lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true).
			Background(ColorMuted).
			Padding(0, 4).
			Render(m.sas)),
		"",
		div,
		"",
		footer,
	)

	panel := WarningPanelStyle.Width(m.panelWidth()).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}
