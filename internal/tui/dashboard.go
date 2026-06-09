package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aegis-p2p/aegis/internal/crypto"
	"github.com/aegis-p2p/aegis/internal/p2p"
)

// ── Tick Messages ─────────────────────────────────────────────────────────────

type uptimeTickMsg time.Time
type scanStepMsg struct{}
type clearClipboardMsg struct {
	CopiedTime time.Time
}

func uptimeCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return uptimeTickMsg(t) })
}

func scanStepCmd() tea.Cmd {
	return tea.Tick(40*time.Millisecond, func(_ time.Time) tea.Msg { return scanStepMsg{} })
}

func clearClipboardCmd(copiedTime time.Time) tea.Cmd {
	return tea.Tick(15*time.Second, func(_ time.Time) tea.Msg {
		return clearClipboardMsg{CopiedTime: copiedTime}
	})
}

// initP2PCmd derives host key deterministically and builds the P2PNode.
func initP2PCmd(passphrase, customRelay string) tea.Cmd {
	return func() tea.Msg {
		defer crypto.String(&passphrase)
		node, err := p2p.NewP2PNode(passphrase, customRelay)
		if err != nil {
			return p2pInitErrorMsg{Err: err}
		}
		return p2pInitSuccessMsg{
			Node:   node,
			PeerID: node.Host.ID().String(),
		}
	}
}

// connectP2PCmd attempts to open a direct /aegis/1.0.0 connection stream.
func connectP2PCmd(node *p2p.P2PNode, targetPeerID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		stream, res, err := node.Connect(ctx, targetPeerID)
		if err != nil {
			return p2pConnectErrorMsg{Err: err}
		}
		return p2pConnectSuccessMsg{Stream: stream, Handshake: res}
	}
}

func safeWriteClipboard(text string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("clipboard panic: %v", r)
		}
	}()
	return clipboard.WriteAll(text)
}

func safeReadClipboard() (val string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("clipboard panic: %v", r)
		}
	}()
	val, err = clipboard.ReadAll()
	return val, err
}

// ── Phase ─────────────────────────────────────────────────────────────────────

type dashPhase int

const (
	phasePassphrase dashPhase = iota // Entering master passphrase to derive key
	phaseIdle                        // Ready to listen or connect
	phaseScanning                    // Connecting progress bar
)

// ── Model ─────────────────────────────────────────────────────────────────────

// DashboardModel is the model for Screen 1: home / connect form.
type DashboardModel struct {
	width, height int
	peerID        string
	input         textinput.Model
	phase         dashPhase
	scanStep      int
	scanTotal     int
	blinkOn       bool
	startTime     time.Time
	uptime        time.Duration
	customRelay   string
	p2pNode       *p2p.P2PNode
	passphraseErr string
	connectErr    string
	showHelp      bool
	copiedNotify  bool
	copiedTime    time.Time
}

// NewDashboard constructs the dashboard in passphrase entry mode.
func NewDashboard(customRelay string) DashboardModel {
	inp := textinput.New()
	inp.Placeholder = "enter master passphrase..."
	inp.CharLimit = 128
	inp.Width = 50
	inp.Prompt = "  > "
	inp.PromptStyle = PrimaryBold
	inp.TextStyle = PrimaryText
	inp.PlaceholderStyle = MutedText
	inp.EchoMode = textinput.EchoPassword
	inp.Focus()

	return DashboardModel{
		input:       inp,
		phase:       phasePassphrase,
		scanTotal:   35,
		startTime:   time.Now(),
		customRelay: customRelay,
	}
}

func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(uptimeCmd(), blinkCmd())
}

func (m DashboardModel) UpdateSize(w, h int) (DashboardModel, tea.Cmd) {
	m.width, m.height = w, h
	return m, nil
}

func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case uptimeTickMsg:
		m.uptime = time.Since(m.startTime)
		if m.copiedNotify && time.Since(m.copiedTime) > 3*time.Second {
			m.copiedNotify = false
		}
		cmds = append(cmds, uptimeCmd())

	case blinkTickMsg:
		m.blinkOn = !m.blinkOn
		cmds = append(cmds, blinkCmd())

	case scanStepMsg:
		if m.phase == phaseScanning && m.scanStep < m.scanTotal {
			m.scanStep++
			cmds = append(cmds, scanStepCmd())
		}

	case clearClipboardMsg:
		if m.copiedNotify && m.copiedTime.Equal(msg.CopiedTime) {
			currentText, err := safeReadClipboard()
			if err == nil && (currentText == m.peerID || (m.p2pNode != nil && currentText == m.p2pNode.GetBestAddress())) {
				_ = safeWriteClipboard("") // Clear pano securely!
			}
			m.copiedNotify = false
		}
		return m, nil

	case tea.KeyMsg:
		// Universal help and close controls
		switch msg.String() {
		case "alt+h", "f5", "F5", "ctrl+b":
			m.showHelp = !m.showHelp
			return m, nil
		case "?":
			if m.phase == phaseIdle && m.input.Value() == "" {
				m.showHelp = !m.showHelp
				return m, nil
			}
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
		}

		if m.showHelp {
			if msg.String() == "esc" {
				m.showHelp = false
				return m, nil
			}
			return m, nil
		}

		switch m.phase {
		case phasePassphrase:
			switch msg.String() {
			case "enter":
				val := strings.TrimSpace(m.input.Value())
				if val != "" {
					m.passphraseErr = ""
					m.input.SetValue("")
					m.input.Placeholder = "initializing node..."
					m.input.EchoMode = textinput.EchoNormal
					return m, initP2PCmd(val, m.customRelay)
				}
			case "esc":
				return m, tea.Quit
			}

		case phaseIdle:
			switch msg.String() {
			case "enter":
				val := strings.TrimSpace(m.input.Value())
				if val != "" && m.p2pNode != nil {
					m.phase = phaseScanning
					m.scanStep = 0
					m.connectErr = ""
					return m, tea.Batch(scanStepCmd(), connectP2PCmd(m.p2pNode, val))
				}
			case "esc":
				if m.p2pNode != nil {
					_ = m.p2pNode.Close()
				}
				return m, tea.Quit
			case "alt+c", "f4", "F4", "ctrl+y", "ctrl+k", "ç", "©", "alt+ç", "alt+©":
				if m.p2pNode != nil {
					if err := safeWriteClipboard(m.p2pNode.GetBestAddress()); err == nil {
						m.copiedNotify = true
						m.copiedTime = time.Now()
						return m, clearClipboardCmd(m.copiedTime)
					}
				} else if m.peerID != "" {
					if err := safeWriteClipboard(m.peerID); err == nil {
						m.copiedNotify = true
						m.copiedTime = time.Now()
						return m, clearClipboardCmd(m.copiedTime)
					}
				}
				return m, nil // Consume key
			case "alt+v", "ctrl+v":
				text, err := safeReadClipboard()
				if err == nil {
					text = strings.TrimSpace(text)
					m.input.SetValue(text)
				}
				return m, nil // Consume key completely
			}
		}
	}

	if !m.showHelp && (m.phase == phasePassphrase || m.phase == phaseIdle) {
		var ic tea.Cmd
		m.input, ic = m.input.Update(msg)
		cmds = append(cmds, ic)
	}

	return m, tea.Batch(cmds...)
}

// ── View ──────────────────────────────────────────────────────────────────────

const dashPanelW = 76 // outer panel width (chars)

func (m DashboardModel) View() string {
	if m.width == 0 {
		return ""
	}
	var panel string
	if m.showHelp {
		panel = m.helpView()
	} else {
		switch m.phase {
		case phasePassphrase:
			panel = m.passphraseView()
		case phaseScanning:
			panel = m.scanView()
		case phaseIdle:
			panel = m.idleView()
		}
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func (m DashboardModel) helpView() string {
	iw := m.panelWidth() - 4
	div := HR(iw)

	center := func(s string) string {
		return lipgloss.NewStyle().Width(iw).Align(lipgloss.Center).Render(s)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		center(RenderBanner()),
		center(PrimaryBold.Render("🛡️  AEGIS SECURE CORE — OPERATIONAL MANUAL")),
		"",
		div,
		"",
		PrimaryBold.Render("  🚀 QUICK START GUIDE"),
		MutedText.Render("  1. Enter your master passphrase to initialize your node."),
		MutedText.Render("  2. Share your Peer ID with a friend, or paste theirs below."),
		MutedText.Render("  3. Check and compare the fingerprint & SAS words for safety."),
		MutedText.Render("  4. Start chatting and sharing files securely!"),
		"",
		PrimaryBold.Render("  🛡️ SECURITY MATRIX"),
		SecondaryText.Render("  • Zero-Storage Guarantee"),
		MutedText.Render("    All keys, history, and buffers exist only in volatile RAM."),
		MutedText.Render("    Closing the app zeroizes all sensitive buffers immediately."),
		"",
		SecondaryText.Render("  • Double Ratchet Engine"),
		MutedText.Render("    Every single message uses a fresh symmetric key derived via"),
		MutedText.Render("    HKDF-SHA256, providing Perfect Forward Secrecy (PFS)."),
		"",
		SecondaryText.Render("  • Mutual Identity Auth"),
		MutedText.Render("    X25519 key exchange signed with ephemeral Ed25519 keys"),
		MutedText.Render("    prevents MITM (Man-in-the-Middle) eavesdropping."),
		"",
		PrimaryBold.Render("  ⌨️ KEYBOARD SHORTCUT LEGEND"),
		"  " + KeyHelp("Ctrl+F", "Browse Files") + "      " + KeyHelp("Ctrl+O", "Enter File Path"),
		"  " + KeyHelp("Ctrl+S", "Save Received File") + "  " + KeyHelp("Alt+C", "Copy ID/Addr"),
		"  " + KeyHelp("Alt+H", "Toggle Help Panel") + "   " + KeyHelp("ESC", "Quit/Disconnect"),
		"",
		div,
		"",
		center(HelpStyle.Render("Press ESC or Alt+H to return to dashboard")),
	)

	return PanelStyle.Width(m.panelWidth()).Render(content)
}

func (m DashboardModel) passphraseView() string {
	iw := m.panelWidth() - 4
	div := HR(iw)

	center := func(s string) string {
		return lipgloss.NewStyle().Width(iw).Align(lipgloss.Center).Render(s)
	}

	var errMsg string
	if m.passphraseErr != "" {
		errMsg = "  " + WarningBold.Render("ERROR: "+m.passphraseErr) + "\n\n"
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		center(RenderBanner()),
		center(SecondaryText.Render(AegisTagline)),
		center(MutedText.Render("version 1.0.0  ·  volatile key initialization")),
		"",
		div,
		"",
		"  "+SecondaryText.Render("ENTER MASTER PASSPHRASE"),
		"  "+MutedText.Render("Derived deterministically via Argon2id. Zero disk writes."),
		"",
		m.input.View(),
		"",
		errMsg,
		div,
		"",
		"  "+KeyHelp("ENTER", "Initialize")+"  "+KeyHelp("Alt+H", "Help")+"  "+KeyHelp("ESC", "Quit"),
	)

	return PanelStyle.Width(m.panelWidth()).Render(content)
}

func (m DashboardModel) idleView() string {
	iw := m.panelWidth() - 4
	div := HR(iw)

	center := func(s string) string {
		return lipgloss.NewStyle().Width(iw).Align(lipgloss.Center).Render(s)
	}

	networkStatus := "awaiting connection"
	if m.customRelay != "" {
		networkStatus = fmt.Sprintf("listening (relay: %s)", shorten(m.customRelay, 25))
	} else {
		networkStatus = "listening (IPFS public bootstrap)"
	}

	peerIDVal := ValueStyle.Render(shorten(m.peerID, 40))
	if m.copiedNotify {
		peerIDVal += " " + PrimaryBold.Render("✓ COPIED")
	}

	addrVal := "initializing..."
	addrNote := ""
	if m.p2pNode != nil {
		addrVal = m.p2pNode.GetBestAddress()
		if !strings.Contains(addrVal, "/p2p-circuit") {
			if m.uptime > 90*time.Second {
				addrNote = " " + WarningText.Render("← no relay (limited connectivity)")
			} else {
				addrNote = " " + MutedText.Render("← relay connecting...")
			}
		}
	}

	info := lipgloss.JoinVertical(lipgloss.Left,
		" "+LabelStyle.Render("PEER ID")+"  "+peerIDVal,
		" "+LabelStyle.Render("ADDRESS")+"  "+SubtleText.Render(addrVal)+addrNote,
		" "+LabelStyle.Render("STATUS")+"   "+m.statusBadge(),
		" "+LabelStyle.Render("UPTIME")+"   "+PrimaryText.Render(fmtDur(m.uptime)),
		" "+LabelStyle.Render("NETWORK")+"  "+MutedText.Render(networkStatus),
	)

	footer := "  " + KeyHelp("ENTER", "Connect") +
		"  " + KeyHelp("Alt+C", "Copy Addr") +
		"  " + KeyHelp("Alt+H", "Help") +
		"  " + KeyHelp("ESC", "Quit")

	var errMsg string
	if m.connectErr != "" {
		errMsg = "  " + WarningBold.Render("ERROR: "+m.connectErr) + "\n\n"
	}

	var relayWarn string
	if m.p2pNode != nil && !m.p2pNode.HasRelayAddress() {
		if m.uptime > 90*time.Second {
			relayWarn = "  " + WarningText.Render("⚠ No relay — cross-network connections will fail") + "\n"
		} else {
			relayWarn = "  " + MutedText.Render("• Relay connecting... cross-network connections may fail until ready") + "\n"
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		center(RenderBanner()),
		center(SecondaryText.Render(AegisTagline)),
		center(MutedText.Render("version 1.0.0  ·  p2p secure mesh")),
		"",
		div,
		"",
		info,
		"",
		div,
		"",
		" "+SecondaryText.Render("CONNECT TO PEER"),
		m.input.View(),
		"",
		relayWarn,
		errMsg,
		div,
		"",
		footer,
	)

	return PanelStyle.Width(m.panelWidth()).Render(content)
}

func (m DashboardModel) scanView() string {
	iw := m.panelWidth() - 4
	div := HR(iw)

	steps := []string{
		"  " + check(m.scanStep > 6) + "  Resolving peer via Kademlia DHT",
		"  " + check(m.scanStep > 13) + "  Attempting NAT hole-punch (DCUtR)",
		"  " + check(m.scanStep > 21) + "  Opening authenticated QUIC stream",
		"  " + check(m.scanStep > 29) + "  Exchanging ephemeral identity keys",
		"  " + check(m.scanStep >= m.scanTotal) + "  Initiating handshake protocol",
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		WarningBold.Render("  ⟳  ESTABLISHING SECURE CHANNEL"),
		WarningText.Render(fmt.Sprintf("  Connecting to: %s", shorten(m.input.Value(), 30))),
		"",
		div,
		"",
		"  "+ScanBar(m.scanStep, m.scanTotal, iw-12),
		"",
		div,
		"",
		strings.Join(steps, "\n"),
		"",
		div,
		"",
		"  "+HelpStyle.Render("press ctrl+c to abort"),
	)

	return WarningPanelStyle.Width(m.panelWidth()).Render(content)
}

func (m DashboardModel) statusBadge() string {
	if m.phase == phaseScanning {
		return WarningBold.Render("[ CONNECTING ]")
	}
	if m.blinkOn {
		return PrimaryBold.Render("[ LISTENING ●]")
	}
	return PrimaryBold.Render("[ LISTENING   ]")
}

// ── Package-level helpers ─────────────────────────────────────────────────────

func check(done bool) string {
	if done {
		return PrimaryText.Render("✓")
	}
	return MutedText.Render("○")
}

func shorten(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	h := max/2 - 2
	return string(runes[:h]) + "..." + string(runes[len(runes)-h:])
}

func fmtDur(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func (m DashboardModel) panelWidth() int {
	w := 76
	if m.width >= 110 {
		w = 96
	} else if m.width >= 90 {
		w = 84
	} else if m.width > 0 && m.width < 78 {
		w = m.width - 2
	}
	if w < 54 {
		w = 54
	}
	return w
}
