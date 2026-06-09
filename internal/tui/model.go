package tui

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/aegis-p2p/aegis/internal/crypto"
	"github.com/aegis-p2p/aegis/internal/p2p"
	"github.com/aegis-p2p/aegis/internal/transfer"
)

// ── Screens ───────────────────────────────────────────────────────────────────

// Screen identifies the active TUI screen.
type Screen int

const (
	ScreenDashboard Screen = iota // Screen 1: PeerID + connect form
	ScreenVerify                  // Screen 2: Cryptographic fingerprint verification
	ScreenChat                    // Screen 3: Encrypted chat room
)

// ── Shared Messages ───────────────────────────────────────────────────────────

// NavigateMsg requests a transition to the given screen.
type NavigateMsg struct{ To Screen }

// blinkTickMsg fires every 500 ms to drive blinking indicator animations.
// Shared between DashboardModel and VerifyModel.
type blinkTickMsg struct{}

func blinkCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(_ time.Time) tea.Msg {
		return blinkTickMsg{}
	})
}

// ── P2P Messages ──────────────────────────────────────────────────────────────

type p2pInitSuccessMsg struct {
	Node   *p2p.P2PNode
	PeerID string
}

type p2pInitErrorMsg struct {
	Err error
}

type p2pIncomingStreamMsg struct {
	Stream network.Stream
}

type p2pIncomingHandshakeSuccessMsg struct {
	Stream    network.Stream
	Handshake *crypto.HandshakeResult
}

type p2pConnectSuccessMsg struct {
	Stream    network.Stream
	Handshake *crypto.HandshakeResult
}

type p2pConnectErrorMsg struct {
	Err error
}

type p2pDisconnectMsg struct {
	Err error
}

type p2pMessageReceivedMsg struct {
	Text  string
	Index uint64
}

type p2pFileFrameMsg struct {
	Frame transfer.AegisFrame
}

// listenForIncomingStreams waits for an incoming /aegis/1.0.0 stream.
func listenForIncomingStreams(node *p2p.P2PNode) tea.Cmd {
	if node == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case s, ok := <-node.IncomingStreamCh:
			if !ok {
				return nil
			}
			return p2pIncomingStreamMsg{Stream: s}
		}
	}
}

// runResponderHandshakeCmd runs the responder cryptographic handshake in a background command.
func runResponderHandshakeCmd(node *p2p.P2PNode, stream network.Stream) tea.Cmd {
	return func() tea.Msg {
		_ = stream.SetDeadline(time.Now().Add(15 * time.Second))
		res, err := crypto.RunHandshake(stream, node.Identity, false)
		if err != nil {
			_ = stream.Reset()
			return p2pConnectErrorMsg{Err: err}
		}
		_ = stream.SetDeadline(time.Time{}) // Reset stream deadlines for normal operations
		node.Ratchet = crypto.NewRatchetState(res.SharedRootKey, false)
		node.RemotePubKey = res.RemotePubKey
		node.ActiveStream = stream
		crypto.Array32(&res.SharedRootKey) // Zeroize root key copy from RAM
		return p2pIncomingHandshakeSuccessMsg{Stream: stream, Handshake: res}
	}
}

// listenForMessages reads incoming frames from the active stream, verifies signatures, and decrypts payloads.
func listenForMessages(node *p2p.P2PNode) tea.Cmd {
	return func() tea.Msg {
		if node == nil || node.ActiveStream == nil || node.Ratchet == nil || node.RemotePubKey == nil {
			LogDebug("listenForMessages returning empty p2pDisconnectMsg: node=%v, activeStream=%v, ratchet=%v, remotePubKey=%v", node != nil, node != nil && node.ActiveStream != nil, node != nil && node.Ratchet != nil, node != nil && node.RemotePubKey != nil)
			return p2pDisconnectMsg{}
		}

		// 1. Read the length-prefixed frame
		payload, err := p2p.ReadFrame(node.ActiveStream)
		if err != nil {
			LogDebug("ReadFrame failed: %v", err)
			return p2pDisconnectMsg{Err: err}
		}

		// Taşkın koruması hız sınırı kontrolü
		if node.Limiter != nil && !node.Limiter.Allow() {
			return p2pDisconnectMsg{Err: fmt.Errorf("security alert: packet flood detected from peer")}
		}

		// 2. Unpack signature and ciphertext
		if len(payload) < 64 {
			return p2pDisconnectMsg{Err: fmt.Errorf("received packet too short: %d bytes", len(payload))}
		}
		sig := payload[:64]
		ciphertext := payload[64:]

		// 3. Verify remote Ed25519 signature
		if !ed25519.Verify(node.RemotePubKey, ciphertext, sig) {
			return p2pDisconnectMsg{Err: fmt.Errorf("cryptographic signature verification failed (MitM alert)")}
		}

		// 4. Generate symmetric AAD
		peerA := node.Host.ID().String()
		peerB := node.ActiveStream.Conn().RemotePeer().String()
		var aad []byte
		if peerA < peerB {
			aad = []byte(peerA + ":" + peerB)
		} else {
			aad = []byte(peerB + ":" + peerA)
		}

		// 5. Decrypt using Double Ratchet
		plaintext, err := node.Ratchet.DecryptMessage(ciphertext, aad)
		if err != nil {
			LogDebug("DecryptMessage failed: %v", err)
			return p2pDisconnectMsg{Err: fmt.Errorf("failed to decrypt message: %w", err)}
		}
		defer crypto.Bytes(plaintext)

		// 6. Unmarshal AegisFrame JSON
		var frame transfer.AegisFrame
		if err := json.Unmarshal(plaintext, &frame); err != nil {
			return p2pDisconnectMsg{Err: fmt.Errorf("failed to parse decrypted frame JSON: %w", err)}
		}

		if frame.Type == transfer.TypeMsg {
			// TUI dondurma taşkın koruması kontrolü
			if len(frame.MsgText) > 8*1024 {
				return p2pDisconnectMsg{Err: fmt.Errorf("security alert: message text exceeds secure limit of 8 KB")}
			}
			return p2pMessageReceivedMsg{
				Text:  frame.MsgText,
				Index: uint64(frame.Index),
			}
		}
		return p2pFileFrameMsg{Frame: frame}
	}
}

// ── Root Model ────────────────────────────────────────────────────────────────

// Model is the root Bubbletea model. It owns all sub-screen models and routes
// messages between them based on the currently active screen.
type Model struct {
	screen           Screen
	width            int
	height           int
	dashboard        DashboardModel
	verify           VerifyModel
	chat             ChatModel
	p2pNode          *p2p.P2PNode
	customRelay      string
	defaultDownloads string
	debug            bool
}

// NewModel constructs the root model with all sub-screens initialised.
func NewModel(customRelay string, defaultDownloads string, debug bool) Model {
	return Model{
		screen:           ScreenDashboard,
		dashboard:        NewDashboard(customRelay),
		verify:           NewVerify(),
		chat:             NewChat(defaultDownloads),
		customRelay:      customRelay,
		defaultDownloads: defaultDownloads,
		debug:            debug,
	}
}

// Init returns the startup commands for the initial screen (dashboard).
func (m Model) Init() tea.Cmd {
	return m.dashboard.Init()
}

// Update processes all incoming messages and routes them to the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Terminal resize ───────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.UpdateSize(msg.Width, msg.Height)
		cmds = append(cmds, cmd)
		m.verify, cmd = m.verify.UpdateSize(msg.Width, msg.Height)
		cmds = append(cmds, cmd)
		m.chat, cmd = m.chat.UpdateSize(msg.Width, msg.Height)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	// ── Global quit ───────────────────────────────────────────────────────────
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			if m.p2pNode != nil {
				_ = m.p2pNode.Close()
			}
			return m, tea.Quit
		}

	// ── P2P Events ────────────────────────────────────────────────────────────
	case p2pInitSuccessMsg:
		m.p2pNode = msg.Node
		m.dashboard.p2pNode = msg.Node
		m.dashboard.peerID = msg.PeerID
		m.dashboard.phase = phaseIdle
		m.dashboard.input.SetValue("")
		m.dashboard.input.Placeholder = "enter remote Peer ID or Multiaddr..."
		m.dashboard.input.EchoMode = textinput.EchoNormal
		return m, listenForIncomingStreams(m.p2pNode)

	case p2pInitErrorMsg:
		m.dashboard.passphraseErr = msg.Err.Error()
		m.dashboard.phase = phasePassphrase
		return m, nil

	case p2pIncomingStreamMsg:
		m.dashboard.phase = phaseScanning
		m.dashboard.scanStep = 21
		return m, tea.Batch(
			scanStepCmd(),
			runResponderHandshakeCmd(m.p2pNode, msg.Stream),
		)

	case p2pIncomingHandshakeSuccessMsg:
		m.p2pNode.RemotePubKey = msg.Handshake.RemotePubKey
		m.screen = ScreenVerify
		m.verify = NewVerify()
		m.verify.p2pNode = m.p2pNode
		m.verify.localFP = crypto.Fingerprint(msg.Handshake.LocalIdentity.PublicKey)
		m.verify.remoteFP = crypto.Fingerprint(msg.Handshake.RemotePubKey)
		m.verify.sas = crypto.SAS(msg.Handshake.LocalIdentity.PublicKey, msg.Handshake.RemotePubKey)
		if m.width > 0 {
			m.verify, _ = m.verify.UpdateSize(m.width, m.height)
		}
		return m, m.verify.Init()

	case p2pConnectSuccessMsg:
		m.p2pNode.RemotePubKey = msg.Handshake.RemotePubKey
		m.screen = ScreenVerify
		m.verify = NewVerify()
		m.verify.p2pNode = m.p2pNode
		m.verify.localFP = crypto.Fingerprint(msg.Handshake.LocalIdentity.PublicKey)
		m.verify.remoteFP = crypto.Fingerprint(msg.Handshake.RemotePubKey)
		m.verify.sas = crypto.SAS(msg.Handshake.LocalIdentity.PublicKey, msg.Handshake.RemotePubKey)
		if m.width > 0 {
			m.verify, _ = m.verify.UpdateSize(m.width, m.height)
		}
		return m, m.verify.Init()

	case p2pConnectErrorMsg:
		m.dashboard.connectErr = msg.Err.Error()
		m.dashboard.phase = phaseIdle
		return m, listenForIncomingStreams(m.p2pNode)

	case p2pDisconnectMsg:
		LogDebug("p2pDisconnectMsg received in Model.Update: err=%v", msg.Err)
		if m.p2pNode != nil {
			if m.p2pNode.ActiveStream != nil {
				_ = m.p2pNode.ActiveStream.Close()
				m.p2pNode.ActiveStream = nil
			}
			if m.p2pNode.Ratchet != nil {
				m.p2pNode.Ratchet.Destroy()
				m.p2pNode.Ratchet = nil
			}
			m.p2pNode.RemotePubKey = nil
		}
		m.chat.Destroy()
		m.screen = ScreenDashboard
		m.dashboard = NewDashboard(m.customRelay)
		m.dashboard.p2pNode = m.p2pNode
		if m.p2pNode != nil {
			m.dashboard.peerID = m.p2pNode.Host.ID().String()
			m.dashboard.phase = phaseIdle
			m.dashboard.input.SetValue("")
			m.dashboard.input.Placeholder = "enter remote Peer ID or Multiaddr..."
			m.dashboard.input.EchoMode = textinput.EchoNormal
		}
		if msg.Err != nil {
			m.dashboard.connectErr = fmt.Sprintf("disconnected: %v", msg.Err)
		} else {
			m.dashboard.connectErr = "session disconnected"
		}
		if m.width > 0 {
			m.dashboard, _ = m.dashboard.UpdateSize(m.width, m.height)
		}
		return m, tea.Batch(m.dashboard.Init(), listenForIncomingStreams(m.p2pNode))

	// ── Screen navigation ─────────────────────────────────────────────────────
	case NavigateMsg:
		m.screen = msg.To
		switch msg.To {
		case ScreenDashboard:
			if m.p2pNode != nil {
				if m.p2pNode.ActiveStream != nil {
					_ = m.p2pNode.ActiveStream.Close()
					m.p2pNode.ActiveStream = nil
				}
				if m.p2pNode.Ratchet != nil {
					m.p2pNode.Ratchet.Destroy()
					m.p2pNode.Ratchet = nil
				}
				m.p2pNode.RemotePubKey = nil
			}
			m.chat.Destroy()
			m.dashboard = NewDashboard(m.customRelay)
			m.dashboard.p2pNode = m.p2pNode
			if m.p2pNode != nil {
				m.dashboard.peerID = m.p2pNode.Host.ID().String()
				m.dashboard.phase = phaseIdle
				m.dashboard.input.SetValue("")
				m.dashboard.input.Placeholder = "enter remote Peer ID or Multiaddr..."
				m.dashboard.input.EchoMode = textinput.EchoNormal
			}
			if m.width > 0 {
				m.dashboard, _ = m.dashboard.UpdateSize(m.width, m.height)
			}
			var cmd tea.Cmd
			if m.p2pNode != nil {
				cmd = listenForIncomingStreams(m.p2pNode)
			}
			return m, tea.Batch(m.dashboard.Init(), cmd)

		case ScreenVerify:
			m.verify = NewVerify()
			m.verify.p2pNode = m.p2pNode
			if m.p2pNode != nil && m.p2pNode.ActiveStream != nil {
				m.verify.remoteFP = m.p2pNode.ActiveStream.Conn().RemotePeer().String()
			}
			if m.width > 0 {
				m.verify, _ = m.verify.UpdateSize(m.width, m.height)
			}
			return m, m.verify.Init()

		case ScreenChat:
			m.chat = NewChat(m.defaultDownloads)
			m.chat.p2pNode = m.p2pNode
			if m.p2pNode != nil && m.p2pNode.ActiveStream != nil {
				m.chat.peerHandle = m.p2pNode.ActiveStream.Conn().RemotePeer().String()
			}
			if m.width > 0 {
				m.chat, _ = m.chat.UpdateSize(m.width, m.height)
			}
			return m, tea.Batch(m.chat.Init(), listenForMessages(m.p2pNode))
		}
		return m, nil
	}

	// ── Delegate to the active screen ─────────────────────────────────────────
	var cmd tea.Cmd
	switch m.screen {
	case ScreenDashboard:
		m.dashboard, cmd = m.dashboard.Update(msg)
	case ScreenVerify:
		m.verify, cmd = m.verify.Update(msg)
	case ScreenChat:
		m.chat, cmd = m.chat.Update(msg)
	}
	return m, cmd
}

// View renders the currently active screen.
func (m Model) View() string {
	if m.width == 0 {
		return PrimaryText.Render("  initializing aegis...")
	}
	switch m.screen {
	case ScreenDashboard:
		return m.dashboard.View()
	case ScreenVerify:
		return m.verify.View()
	case ScreenChat:
		return m.chat.View()
	}
	return ""
}

// Close gracefully terminates the node and zeroizes all active key materials and buffers.
func (m *Model) Close() {
	if m.p2pNode != nil {
		_ = m.p2pNode.Close()
	}
	m.chat.Destroy()
}

