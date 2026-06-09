package tui

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aegis-p2p/aegis/internal/crypto"
	"github.com/aegis-p2p/aegis/internal/p2p"
	"github.com/aegis-p2p/aegis/internal/transfer"
)

// MaxFileTransferSize is the maximum file size (100 MB) accepted for in-memory file transfers.
// This prevents memory exhaustion attacks from malicious peers.
const MaxFileTransferSize = 100 * 1024 * 1024 // 100 MB

// ── File Transfer State Enums ────────────────────────────────────────────────

type SenderState int

const (
	SenderIdle SenderState = iota
	SenderWaitAccept
	SenderSending
	SenderDone
	SenderRejected
	SenderError
)

type ReceiverState int

const (
	ReceiverIdle ReceiverState = iota
	ReceiverOfferReceived
	ReceiverReceiving
	ReceiverSavePrompt
	ReceiverSaving
	ReceiverDone
	ReceiverError
)

// ── Model Structures ──────────────────────────────────────────────────────────

type MessageStatus int

const (
	StatusSent MessageStatus = iota
	StatusDelivered
	StatusRead
)

type chatMessage struct {
	ID        uint64
	Sender    string // YOU, PEER, or SYSTEM
	Timestamp time.Time
	Text      string
	Status    MessageStatus
}

// ChatModel is the TUI controller for the secure, zero-trust chat room.
type ChatModel struct {
	width, height int
	peerHandle    string
	p2pNode       *p2p.P2PNode
	input         textinput.Model
	chatHistory   []chatMessage

	// File transfer state (Sender)
	fileMode    bool
	filePath    string
	sendState   SenderState

	// File transfer state (Receiver)
	saveMode      bool
	savePath      string
	recvState     ReceiverState
	recvFileBytes []byte

	// Shared transfer metadata
	fileName    string
	fileSize    int64
	fileHash    string
	totalChunks int
	currChunk   int
	transferErr string

	// Usability additions
	defaultDownloads string
	viewport         viewport.Model

	// Typing state
	peerTyping     bool
	lastTypingTime time.Time
	lastTypingSent time.Time

	// Diagnostics state
	rtt            time.Duration
	connectionType string

	// Picker state
	filepicker filepicker.Model
	pickerMode bool
}

// NewChat initializes a clean chat screen in idle mode.
func NewChat(defaultDownloads string) ChatModel {
	inp := textinput.New()
	inp.Placeholder = "type a secure message..."
	inp.CharLimit = 1024
	inp.Width = 80
	inp.Prompt = "  > "
	inp.PromptStyle = PrimaryBold
	inp.TextStyle = PrimaryText
	inp.PlaceholderStyle = MutedText
	inp.Focus()

	return ChatModel{
		input:            inp,
		sendState:        SenderIdle,
		recvState:        ReceiverIdle,
		defaultDownloads: defaultDownloads,
	}
}

type pingTickMsg time.Time

func pingTickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return pingTickMsg(t)
	})
}

func (m ChatModel) Init() tea.Cmd {
	return pingTickCmd()
}

func (m ChatModel) UpdateSize(w, h int) (ChatModel, tea.Cmd) {
	m.width, m.height = w, h

	sidebarW := 34
	chatW := m.width - sidebarW - 1
	if chatW < 20 {
		chatW = 20
	}
	chatHeight := m.height - 8
	if chatHeight < 5 {
		chatHeight = 5
	}
	m.viewport.Width = chatW
	m.viewport.Height = chatHeight
	m.viewport.KeyMap = viewport.DefaultKeyMap()
	m.updateViewportContent()
	return m, nil
}

func (m *ChatModel) updateViewportContent() {
	sidebarW := 34
	chatW := m.width - sidebarW - 1
	if chatW < 20 {
		chatW = 20
	}
	chatHeight := m.height - 8
	if chatHeight < 5 {
		chatHeight = 5
	}
	m.viewport.Width = chatW
	m.viewport.Height = chatHeight
	m.viewport.SetContent(m.renderChatContent(chatW, chatHeight))
	m.viewport.GotoBottom()
}

func (m ChatModel) renderChatContent(chatW, chatHeight int) string {
	var chatLines []string
	for _, msg := range m.chatHistory {
		timeStr := TimestampStyle.Render(msg.Timestamp.Format("15:04:05"))
		var handle string
		if msg.Sender == "YOU" {
			handle = HandleSelfStyle.Render("YOU")
		} else if msg.Sender == "PEER" {
			handle = HandlePeerStyle.Render("PEER")
		} else {
			handle = WarningBold.Render("SYS")
		}

		runes := []rune(msg.Text)
		maxTextW := chatW - 18
		if maxTextW < 10 {
			maxTextW = 10
		}

		var chatMsgLines []string
		for len(runes) > maxTextW {
			chatMsgLines = append(chatMsgLines, MsgStyle.Render(string(runes[:maxTextW])))
			runes = runes[maxTextW:]
		}
		chatMsgLines = append(chatMsgLines, MsgStyle.Render(string(runes)))

		// Append status icon to the last line of YOU messages
		if msg.Sender == "YOU" {
			var statusIcon string
			switch msg.Status {
			case StatusSent:
				statusIcon = SubtleText.Render(" ✓")
			case StatusDelivered:
				statusIcon = SecondaryText.Render(" ✓")
			case StatusRead:
				statusIcon = PrimaryBold.Render(" ✓✓")
			}
			chatMsgLines[len(chatMsgLines)-1] += statusIcon
		}

		for i, line := range chatMsgLines {
			if i > 0 {
				timeStr = "        "
				handle = "    "
			}
			chatLines = append(chatLines, fmt.Sprintf("%s %s › %s", timeStr, handle, line))
		}
	}

	return strings.Join(chatLines, "\n")
}

// ── Cryptographic Send Helper ────────────────────────────────────────────────

func sendFrame(node *p2p.P2PNode, frame transfer.AegisFrame) error {
	if node == nil || node.ActiveStream == nil || node.Ratchet == nil {
		return fmt.Errorf("no active connection stream")
	}

	// 1. Marshal to JSON plaintext
	plaintext, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("failed to marshal frame: %w", err)
	}
	defer crypto.Bytes(plaintext)

	// 2. Generate symmetric AAD from Peer IDs
	peerA := node.Host.ID().String()
	peerB := node.ActiveStream.Conn().RemotePeer().String()
	var aad []byte
	if peerA < peerB {
		aad = []byte(peerA + ":" + peerB)
	} else {
		aad = []byte(peerB + ":" + peerA)
	}

	// 3. Encrypt using Double Ratchet
	ciphertext, err := node.Ratchet.EncryptMessage(plaintext, aad)
	if err != nil {
		return fmt.Errorf("ratchet encryption failed: %w", err)
	}

	// 4. Sign using Ed25519
	sig := ed25519.Sign(node.Identity.PrivateKey, ciphertext)

	// 5. Pack [Signature (64B)][Ciphertext]
	payload := make([]byte, 64+len(ciphertext))
	copy(payload[:64], sig)
	copy(payload[64:], ciphertext)

	// 6. Write frame length-prefixed
	if err := p2p.WriteFrame(node.ActiveStream, payload); err != nil {
		return fmt.Errorf("failed to write wire packet: %w", err)
	}

	return nil
}

// ── Chunk Sending Command ────────────────────────────────────────────────────

type p2pChunkSentSuccessMsg struct {
	Index int
}

type p2pChunkSendErrorMsg struct {
	Err error
}

func sendNextChunkCmd(node *p2p.P2PNode, filePath string, chunkIndex int, fileName string) tea.Cmd {
	return func() tea.Msg {
		file, err := os.Open(filePath)
		if err != nil {
			return p2pChunkSendErrorMsg{Err: err}
		}
		defer file.Close()

		const ChunkSize = 256 * 1024 // 256 KB chunks
		_, err = file.Seek(int64(chunkIndex)*ChunkSize, io.SeekStart)
		if err != nil {
			return p2pChunkSendErrorMsg{Err: err}
		}

		buf := make([]byte, ChunkSize)
		defer crypto.Bytes(buf)
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return p2pChunkSendErrorMsg{Err: err}
		}

		frame := transfer.AegisFrame{
			Type:  transfer.TypeChunk,
			Name:  fileName,
			Index: chunkIndex,
			Data:  buf[:n],
		}

		if err := sendFrame(node, frame); err != nil {
			return p2pChunkSendErrorMsg{Err: err}
		}

		return p2pChunkSentSuccessMsg{Index: chunkIndex}
	}
}

// ── Update Logic ─────────────────────────────────────────────────────────────

type clearTypingMsg struct {
	Time time.Time
}

func triggerClearTypingCmd(t time.Time) tea.Cmd {
	return tea.Tick(2600*time.Millisecond, func(_ time.Time) tea.Msg {
		return clearTypingMsg{Time: t}
	})
}

func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	var cmds []tea.Cmd

	if m.pickerMode {
		// Only intercept if the message is NOT one of our custom P2P/TUI messages.
		// This ensures network and custom timer events fall through to the main switch
		// and don't get lost, while key presses, window sizes, and filepicker internal
		// commands (like readDirMsg) are correctly routed to the filepicker.
		switch msg.(type) {
		case clearTypingMsg, pingTickMsg, p2pMessageReceivedMsg, p2pChunkSentSuccessMsg, p2pChunkSendErrorMsg, p2pFileFrameMsg:
			// Let these fall through to the main switch!
		default:
			// Intercept Esc key to cancel filepicker mode
			if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEsc {
				m.pickerMode = false
				return m, nil
			}

			var fpCmd tea.Cmd
			m.filepicker, fpCmd = m.filepicker.Update(msg)

			// Check if a file was selected
			if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
				m.pickerMode = false

				fi, err := os.Stat(path)
				if err != nil {
					m.sendState = SenderError
					m.transferErr = fmt.Sprintf("file not found: %s", err)
					return m, nil
				}

				m.filePath = path
				m.fileName = filepath.Base(path)
				m.fileSize = fi.Size()

				if m.fileSize == 0 {
					m.sendState = SenderError
					m.transferErr = "empty files not supported"
					return m, nil
				}

				if m.fileSize > MaxFileTransferSize {
					m.sendState = SenderError
					m.transferErr = fmt.Sprintf("file too large (%d MB max)", MaxFileTransferSize/(1024*1024))
					return m, nil
				}

				const ChunkSize = 256 * 1024
				m.totalChunks = int((m.fileSize + ChunkSize - 1) / ChunkSize)

				hash, err := transfer.HashFile(path)
				if err != nil {
					m.sendState = SenderError
					m.transferErr = err.Error()
					return m, nil
				}
				m.fileHash = hash

				offerFrame := transfer.AegisFrame{
					Type:   transfer.TypeFileOffer,
					Name:   m.fileName,
					Size:   m.fileSize,
					Chunks: m.totalChunks,
					SHA256: m.fileHash,
				}

				if err := sendFrame(m.p2pNode, offerFrame); err != nil {
					m.sendState = SenderError
					m.transferErr = err.Error()
					return m, nil
				}

				m.sendState = SenderWaitAccept
				m.currChunk = 0
				return m, nil
			}

			return m, fpCmd
		}
	}

	switch msg := msg.(type) {
	case clearTypingMsg:
		if m.peerTyping && time.Since(m.lastTypingTime) >= 2500*time.Millisecond {
			m.peerTyping = false
		}
		return m, nil

	case pingTickMsg:
		if m.p2pNode != nil && m.p2pNode.ActiveStream != nil {
			pingFrame := transfer.AegisFrame{
				Type: transfer.TypePing,
				Size: time.Now().UnixNano(),
			}
			if err := sendFrame(m.p2pNode, pingFrame); err != nil {
				return m, func() tea.Msg { return p2pDisconnectMsg{Err: err} }
			}
			m.connectionType = m.p2pNode.GetConnectionType()
		}
		return m, pingTickCmd()
	case tea.KeyMsg:
		// Intercept Y/N keys directly if we are prompting for a file accept/reject
		if m.recvState == ReceiverOfferReceived {
			switch strings.ToLower(msg.String()) {
			case "y":
				LogDebug("Receiver: user pressed Y. m.fileSize=%d", m.fileSize)
				m.recvState = ReceiverReceiving
				m.currChunk = 0
				m.recvFileBytes = make([]byte, 0, m.fileSize)

				acceptFrame := transfer.AegisFrame{
					Type: transfer.TypeFileAccept,
				}
				if err := sendFrame(m.p2pNode, acceptFrame); err != nil {
					LogDebug("Receiver: failed to send TypeFileAccept frame: %v", err)
					m.recvState = ReceiverError
					m.transferErr = err.Error()
					m.chatHistory = append(m.chatHistory, chatMessage{
						Sender:    "SYSTEM",
						Timestamp: time.Now(),
						Text:      fmt.Sprintf("✗ Error accepting file offer: %v", err),
					})
				} else {
					LogDebug("Receiver: successfully sent TypeFileAccept frame.")
					m.chatHistory = append(m.chatHistory, chatMessage{
						Sender:    "SYSTEM",
						Timestamp: time.Now(),
						Text:      fmt.Sprintf("✓ Accepted file offer for '%s'. Receiving...", m.fileName),
					})
				}
				m.updateViewportContent()
				m.input.Focus()
				return m, nil

			case "n":
				LogDebug("Receiver: user pressed N. Rejecting file offer.")
				m.recvState = ReceiverIdle

				rejectFrame := transfer.AegisFrame{
					Type: transfer.TypeFileReject,
				}
				_ = sendFrame(m.p2pNode, rejectFrame)
				m.chatHistory = append(m.chatHistory, chatMessage{
					Sender:    "SYSTEM",
					Timestamp: time.Now(),
					Text:      fmt.Sprintf("✗ Rejected file offer for '%s'.", m.fileName),
				})
				m.updateViewportContent()
				m.input.Focus()
				return m, nil
			}
		}

		// Scroll viewport with key Up, Down, PageUp, PageDown if not in file or save modes
		if !m.fileMode && !m.saveMode {
			switch msg.Type {
			case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown:
				var vpCmd tea.Cmd
				m.viewport, vpCmd = m.viewport.Update(msg)
				return m, vpCmd
			}

			// Send typing notification on typing activity
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeySpace {
				if time.Since(m.lastTypingSent) > 2*time.Second {
					m.lastTypingSent = time.Now()
					typingFrame := transfer.AegisFrame{
						Type: transfer.TypeTyping,
					}
					_ = sendFrame(m.p2pNode, typingFrame)
				}
			}
		}

		// Handle control shortcuts
		switch msg.Type {
		case tea.KeyCtrlF:
			if !m.fileMode && !m.saveMode && m.sendState == SenderIdle && m.recvState == ReceiverIdle {
				m.pickerMode = true
				m.filepicker = filepicker.New()
				m.filepicker.CurrentDirectory, _ = os.Getwd()
				m.filepicker.Styles.Cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(ColorPrimary).Bold(true)
				m.filepicker.Styles.Directory = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
				m.filepicker.Styles.File = lipgloss.NewStyle().Foreground(ColorWhite)
				m.filepicker.Styles.Permission = lipgloss.NewStyle().Foreground(ColorSubtle)
				m.filepicker.Styles.Selected = lipgloss.NewStyle().Foreground(ColorPrimary).Underline(true)
				return m, m.filepicker.Init()
			}
			return m, nil

		case tea.KeyCtrlO:
			if !m.fileMode && !m.saveMode && m.sendState == SenderIdle && m.recvState == ReceiverIdle {
				m.fileMode = true
				m.input.Placeholder = "enter absolute file path..."
				m.input.SetValue("")
				m.input.Focus()
			}
			return m, nil

		case tea.KeyCtrlS:
			if m.recvState == ReceiverSavePrompt {
				m.saveMode = true
				m.input.Placeholder = "enter save path on disk..."
				if m.defaultDownloads != "" {
					m.input.SetValue(filepath.Join(m.defaultDownloads, m.fileName))
				} else {
					m.input.SetValue("")
				}
			}
			return m, nil

		case tea.KeyEsc:
			if m.fileMode {
				m.fileMode = false
				m.input.Placeholder = "type a secure message..."
				m.input.SetValue("")
			} else if m.saveMode {
				m.saveMode = false
				m.input.Placeholder = "type a secure message..."
				m.input.SetValue("")
			}
			return m, nil

		case tea.KeyEnter:
			if m.fileMode {
				filePath := strings.TrimSpace(m.input.Value())
				m.input.SetValue("")
				m.input.Placeholder = "type a secure message..."
				m.fileMode = false

				if filePath == "" {
					return m, nil
				}

				fi, err := os.Stat(filePath)
				if err != nil {
					m.sendState = SenderError
					m.transferErr = fmt.Sprintf("file not found: %s", err)
					return m, nil
				}

				m.filePath = filePath
				m.fileName = filepath.Base(filePath)
				m.fileSize = fi.Size()

				if m.fileSize == 0 {
					m.sendState = SenderError
					m.transferErr = "empty files not supported"
					return m, nil
				}

				if m.fileSize > MaxFileTransferSize {
					m.sendState = SenderError
					m.transferErr = fmt.Sprintf("file too large (%d MB max)", MaxFileTransferSize/(1024*1024))
					return m, nil
				}

				const ChunkSize = 256 * 1024
				m.totalChunks = int((m.fileSize + ChunkSize - 1) / ChunkSize)

				// Hashing the file in a streaming way
				hash, err := transfer.HashFile(filePath)
				if err != nil {
					m.sendState = SenderError
					m.transferErr = err.Error()
					return m, nil
				}
				m.fileHash = hash

				offerFrame := transfer.AegisFrame{
					Type:   transfer.TypeFileOffer,
					Name:   m.fileName,
					Size:   m.fileSize,
					Chunks: m.totalChunks,
					SHA256: m.fileHash,
				}

				if err := sendFrame(m.p2pNode, offerFrame); err != nil {
					m.sendState = SenderError
					m.transferErr = err.Error()
					return m, nil
				}

				m.sendState = SenderWaitAccept
				m.currChunk = 0
				return m, nil

			} else if m.saveMode {
				savePath := strings.TrimSpace(m.input.Value())
				m.input.SetValue("")
				m.input.Placeholder = "type a secure message..."
				m.saveMode = false

				if savePath == "" {
					return m, nil
				}

				// Sanitize and resolve the save path to prevent path traversal
				savePath = filepath.Clean(savePath)
				absPath, err := filepath.Abs(savePath)
				if err != nil {
					m.recvState = ReceiverError
					m.transferErr = "invalid save path"
					m.chatHistory = append(m.chatHistory, chatMessage{
						Sender:    "SYSTEM",
						Timestamp: time.Now(),
						Text:      "✗ Save failed: invalid save path",
					})
					m.updateViewportContent()
					return m, nil
				}
				savePath = absPath

				// Auto-append filename if save path is an existing directory
				fi, err := os.Stat(savePath)
				if err == nil && fi.IsDir() {
					savePath = filepath.Join(savePath, m.fileName)
				}

				// Ensure parent directory exists
				dir := filepath.Dir(savePath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					m.recvState = ReceiverError
					m.transferErr = fmt.Sprintf("failed to create directory: %v", err)
					m.chatHistory = append(m.chatHistory, chatMessage{
						Sender:    "SYSTEM",
						Timestamp: time.Now(),
						Text:      fmt.Sprintf("✗ File save failed: %v", err),
					})
					m.updateViewportContent()
					return m, nil
				}

				m.savePath = savePath

				m.recvState = ReceiverSaving
				err = transfer.SaveFile(savePath, m.recvFileBytes)
				if err != nil {
					m.recvState = ReceiverError
					m.transferErr = err.Error()
					m.chatHistory = append(m.chatHistory, chatMessage{
						Sender:    "SYSTEM",
						Timestamp: time.Now(),
						Text:      fmt.Sprintf("✗ File save failed: %v", err),
					})
					m.updateViewportContent()
					return m, nil
				}

				// RAM Security: zeroize received bytes
				crypto.Bytes(m.recvFileBytes)
				m.recvFileBytes = nil

				m.recvState = ReceiverDone
				m.chatHistory = append(m.chatHistory, chatMessage{
					Sender:    "SYSTEM",
					Timestamp: time.Now(),
					Text:      fmt.Sprintf("✓ File '%s' saved securely to: %s", m.fileName, savePath),
				})
				m.updateViewportContent()
				return m, nil

			} else {
				val := strings.TrimSpace(m.input.Value())
				if val != "" {
					m.input.SetValue("")

					// 1. Process valid slash commands
					isCmd := false
					if strings.HasPrefix(val, "/") {
						parts := strings.Fields(val)
						cmdName := strings.ToLower(parts[0])
						switch cmdName {
						case "/clear", "/quit", "/disconnect", "/status", "/help":
							isCmd = true
						}
					}

					if isCmd {
						parts := strings.Fields(val)
						cmdName := strings.ToLower(parts[0])
						switch cmdName {
						case "/clear":
							m.chatHistory = nil
							m.updateViewportContent()
							return m, nil

						case "/quit", "/disconnect":
							return m, func() tea.Msg { return p2pDisconnectMsg{Err: fmt.Errorf("session closed by user")} }

						case "/status":
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      "=== AEGIS SECURE CORE DIAGNOSTICS ===",
							})
							if m.p2pNode != nil && m.p2pNode.Ratchet != nil {
								r := m.p2pNode.Ratchet
								m.chatHistory = append(m.chatHistory, chatMessage{
									Sender:    "SYSTEM",
									Timestamp: time.Now(),
									Text:      fmt.Sprintf("• PeerID: %s", m.p2pNode.Host.ID().String()),
								})
								m.chatHistory = append(m.chatHistory, chatMessage{
									Sender:    "SYSTEM",
									Timestamp: time.Now(),
									Text:      fmt.Sprintf("• Connection: %s (Latency: %d ms)", m.connectionType, m.rtt.Milliseconds()),
								})
								m.chatHistory = append(m.chatHistory, chatMessage{
									Sender:    "SYSTEM",
									Timestamp: time.Now(),
									Text:      fmt.Sprintf("• Double Ratchet send seq: %d, recv seq: %d", r.SendMsgNum, r.RecvMsgNum),
								})
								m.chatHistory = append(m.chatHistory, chatMessage{
									Sender:    "SYSTEM",
									Timestamp: time.Now(),
									Text:      fmt.Sprintf("• Ephemeral Identity FP: %s", crypto.Fingerprint(m.p2pNode.Identity.PublicKey)),
								})
							} else {
								m.chatHistory = append(m.chatHistory, chatMessage{
									Sender:    "SYSTEM",
									Timestamp: time.Now(),
									Text:      "• Secure Core not initialized.",
								})
							}
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      "=====================================",
							})
							m.updateViewportContent()
							return m, nil

						case "/help":
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      "=== AVAILABLE SLASH COMMANDS ===",
							})
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      "• /clear - Clears local screen history from memory",
							})
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      "• /status - Displays secure core diagnostics report",
							})
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      "• /quit /disconnect - Securely disconnects session",
							})
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      "• /help - Shows this help menu",
							})
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      "================================",
							})
							m.updateViewportContent()
							return m, nil
						}
					}

					// 2. Check if the value is a local file path (e.g. pasted or drag-and-dropped)
					filePath := val
					if len(filePath) >= 2 {
						if (filePath[0] == '"' && filePath[len(filePath)-1] == '"') || (filePath[0] == '\'' && filePath[len(filePath)-1] == '\'') {
							filePath = filePath[1 : len(filePath)-1]
						}
					}
					filePath = strings.TrimSpace(filePath)

					if fi, err := os.Stat(filePath); err == nil {
						if fi.IsDir() {
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      fmt.Sprintf("✗ Failed to send: '%s' is a directory. Only file transfers are supported.", filePath),
							})
							m.updateViewportContent()
							return m, nil
						}

						if fi.Size() == 0 {
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      "✗ Failed to send: empty files not supported",
							})
							m.updateViewportContent()
							return m, nil
						}
						if fi.Size() > MaxFileTransferSize {
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      fmt.Sprintf("✗ Failed to send: file too large (%d MB max)", MaxFileTransferSize/(1024*1024)),
							})
							m.updateViewportContent()
							return m, nil
						}

						// Automatically switch to sender mode and initiate file offer!
						m.filePath = filePath
						m.fileName = filepath.Base(filePath)
						m.fileSize = fi.Size()
						const ChunkSize = 256 * 1024
						m.totalChunks = int((m.fileSize + ChunkSize - 1) / ChunkSize)

						hash, err := transfer.HashFile(filePath)
						if err != nil {
							m.sendState = SenderError
							m.transferErr = err.Error()
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      fmt.Sprintf("✗ Failed to hash file: %v", err),
							})
							m.updateViewportContent()
							return m, nil
						}
						m.fileHash = hash

						offerFrame := transfer.AegisFrame{
							Type:   transfer.TypeFileOffer,
							Name:   m.fileName,
							Size:   m.fileSize,
							Chunks: m.totalChunks,
							SHA256: m.fileHash,
						}

						if err := sendFrame(m.p2pNode, offerFrame); err != nil {
							m.sendState = SenderError
							m.transferErr = err.Error()
							m.chatHistory = append(m.chatHistory, chatMessage{
								Sender:    "SYSTEM",
								Timestamp: time.Now(),
								Text:      fmt.Sprintf("✗ Failed to send file offer: %v", err),
							})
							m.updateViewportContent()
							return m, nil
						}

						m.sendState = SenderWaitAccept
						m.currChunk = 0
						m.chatHistory = append(m.chatHistory, chatMessage{
							Sender:    "SYSTEM",
							Timestamp: time.Now(),
							Text:      fmt.Sprintf("📤 Initiating secure transfer for: %s", filePath),
						})
						m.updateViewportContent()
						return m, nil
					}

					if m.p2pNode == nil || m.p2pNode.Ratchet == nil {
						m.chatHistory = append(m.chatHistory, chatMessage{
							Sender:    "SYSTEM",
							Timestamp: time.Now(),
							Text:      "Failed to send: no active secure connection",
						})
						m.updateViewportContent()
						return m, nil
					}

					msgID := m.p2pNode.Ratchet.SendMsgNum
					frame := transfer.AegisFrame{
						Type:    transfer.TypeMsg,
						MsgText: val,
						Index:   int(msgID),
					}
					err := sendFrame(m.p2pNode, frame)
					if err != nil {
						m.chatHistory = append(m.chatHistory, chatMessage{
							Sender:    "SYSTEM",
							Timestamp: time.Now(),
							Text:      "Failed to send: " + err.Error(),
						})
					} else {
						m.chatHistory = append(m.chatHistory, chatMessage{
							ID:        msgID,
							Sender:    "YOU",
							Timestamp: time.Now(),
							Text:      val,
							Status:    StatusSent,
						})
					}
					m.updateViewportContent()
				}
				return m, nil
			}
		}

	case p2pMessageReceivedMsg:
		m.chatHistory = append(m.chatHistory, chatMessage{
			ID:        msg.Index,
			Sender:    "PEER",
			Timestamp: time.Now(),
			Text:      msg.Text,
		})
		m.updateViewportContent()

		// Automatically reply with TypeMsgDelivered
		deliveredFrame := transfer.AegisFrame{
			Type:  transfer.TypeMsgDelivered,
			Index: int(msg.Index),
		}
		_ = sendFrame(m.p2pNode, deliveredFrame)

		// Automatically reply with TypeMsgRead (active chat session)
		readFrame := transfer.AegisFrame{
			Type:  transfer.TypeMsgRead,
			Index: int(msg.Index),
		}
		_ = sendFrame(m.p2pNode, readFrame)

		return m, listenForMessages(m.p2pNode)

	case p2pChunkSentSuccessMsg:
		// Wait for recipient TypeChunkAck to maintain standard window-size 1 transfer
		return m, nil

	case p2pChunkSendErrorMsg:
		m.sendState = SenderError
		m.transferErr = msg.Err.Error()
		return m, nil

	case p2pFileFrameMsg:
		frame := msg.Frame
		switch frame.Type {
		case transfer.TypeTyping:
			m.peerTyping = true
			m.lastTypingTime = time.Now()
			return m, tea.Batch(
				listenForMessages(m.p2pNode),
				triggerClearTypingCmd(m.lastTypingTime),
			)

		case transfer.TypeMsgDelivered:
			for i := len(m.chatHistory) - 1; i >= 0; i-- {
				if m.chatHistory[i].Sender == "YOU" && m.chatHistory[i].ID == uint64(frame.Index) {
					if m.chatHistory[i].Status < StatusDelivered {
						m.chatHistory[i].Status = StatusDelivered
						m.updateViewportContent()
					}
					break
				}
			}
			return m, listenForMessages(m.p2pNode)

		case transfer.TypeMsgRead:
			for i := len(m.chatHistory) - 1; i >= 0; i-- {
				if m.chatHistory[i].Sender == "YOU" && m.chatHistory[i].ID == uint64(frame.Index) {
					if m.chatHistory[i].Status < StatusRead {
						m.chatHistory[i].Status = StatusRead
						m.updateViewportContent()
					}
					break
				}
			}
			return m, listenForMessages(m.p2pNode)

		case transfer.TypePing:
			pongFrame := transfer.AegisFrame{
				Type: transfer.TypePong,
				Size: frame.Size,
			}
			_ = sendFrame(m.p2pNode, pongFrame)
			return m, listenForMessages(m.p2pNode)

		case transfer.TypePong:
			now := time.Now().UnixNano()
			elapsed := time.Duration(now - frame.Size)
			m.rtt = elapsed
			return m, listenForMessages(m.p2pNode)

		case transfer.TypeFileOffer:
			LogDebug("Receiver received TypeFileOffer: name=%s, size=%d, chunks=%d, hash=%s", frame.Name, frame.Size, frame.Chunks, frame.SHA256)
			if m.recvState == ReceiverIdle {
				// Sanitize incoming file name to prevent path traversal cross-platform
				sanitized := strings.ReplaceAll(frame.Name, "\\", "/")
				safeName := filepath.Base(sanitized)
				if safeName == "." || safeName == ".." || safeName == "" || strings.Contains(safeName, "/") || strings.Contains(safeName, "\\") {
					LogDebug("Receiver: rejected malicious file name: %s", frame.Name)
					return m, listenForMessages(m.p2pNode)
				}

				// Enforce maximum file size to prevent memory exhaustion
				if frame.Size <= 0 || frame.Size > MaxFileTransferSize {
					LogDebug("Receiver: rejected invalid or oversized file: size=%d", frame.Size)
					rejectFrame := transfer.AegisFrame{
						Type: transfer.TypeFileReject,
					}
					_ = sendFrame(m.p2pNode, rejectFrame)
					return m, listenForMessages(m.p2pNode)
				}

				m.recvState = ReceiverOfferReceived
				m.fileName = safeName
				m.fileSize = frame.Size
				m.totalChunks = frame.Chunks
				m.fileHash = frame.SHA256
				m.currChunk = 0
				m.recvFileBytes = nil
				m.input.Blur() // Temporarily unfocus text entry for acceptance keys
				LogDebug("Receiver: state set to ReceiverOfferReceived, prompt displayed.")
				m.chatHistory = append(m.chatHistory, chatMessage{
					Sender:    "SYSTEM",
					Timestamp: time.Now(),
					Text:      fmt.Sprintf("📥 Incoming file offer: '%s' (%d bytes). Accept with Y / Reject with N.", safeName, frame.Size),
				})
				m.updateViewportContent()
			} else {
				LogDebug("Receiver: ignored TypeFileOffer because state is not ReceiverIdle (state=%v)", m.recvState)
			}
			return m, listenForMessages(m.p2pNode)

		case transfer.TypeFileAccept:
			LogDebug("Sender received TypeFileAccept. Current state=%v", m.sendState)
			if m.sendState == SenderWaitAccept {
				m.sendState = SenderSending
				m.currChunk = 0
				LogDebug("Sender starting transfer of %s...", m.fileName)
				m.chatHistory = append(m.chatHistory, chatMessage{
					Sender:    "SYSTEM",
					Timestamp: time.Now(),
					Text:      fmt.Sprintf("✓ Peer accepted file offer. Sending '%s'...", m.fileName),
				})
				m.updateViewportContent()
				return m, tea.Batch(
					listenForMessages(m.p2pNode),
					sendNextChunkCmd(m.p2pNode, m.filePath, 0, m.fileName),
				)
			}
			return m, listenForMessages(m.p2pNode)

		case transfer.TypeFileReject:
			if m.sendState == SenderWaitAccept {
				m.sendState = SenderRejected
				m.chatHistory = append(m.chatHistory, chatMessage{
					Sender:    "SYSTEM",
					Timestamp: time.Now(),
					Text:      fmt.Sprintf("✗ Peer rejected file offer for '%s'.", m.fileName),
				})
				m.updateViewportContent()
			}
			return m, listenForMessages(m.p2pNode)

		case transfer.TypeChunk:
			LogDebug("Receiver received TypeChunk: index=%d, size=%d, current expected index=%d, state=%v", frame.Index, len(frame.Data), m.currChunk, m.recvState)
			if m.recvState == ReceiverReceiving {
				if frame.Index == m.currChunk {
					// Verify cumulative received size doesn't exceed declared file size
					if int64(len(m.recvFileBytes))+int64(len(frame.Data)) > m.fileSize {
						LogDebug("Receiver: error, received data size exceeds declared size")
						m.recvState = ReceiverError
						m.transferErr = "received data exceeds declared file size"
						m.chatHistory = append(m.chatHistory, chatMessage{
							Sender:    "SYSTEM",
							Timestamp: time.Now(),
							Text:      "✗ Received data exceeds declared file size.",
						})
						m.updateViewportContent()
						crypto.Bytes(m.recvFileBytes)
						m.recvFileBytes = nil
						return m, listenForMessages(m.p2pNode)
					}

					m.recvFileBytes = append(m.recvFileBytes, frame.Data...)
					crypto.Bytes(frame.Data)
					m.currChunk++

					LogDebug("Receiver: appending chunk %d. Sending TypeChunkAck...", frame.Index)
					ackFrame := transfer.AegisFrame{
						Type:  transfer.TypeChunkAck,
						Index: frame.Index,
					}
					if err := sendFrame(m.p2pNode, ackFrame); err != nil {
						LogDebug("Receiver: failed to send TypeChunkAck: %v", err)
					}
				} else {
					LogDebug("Receiver: ignored out-of-order chunk %d (expected %d)", frame.Index, m.currChunk)
				}
			} else {
				LogDebug("Receiver: ignored TypeChunk because state is not ReceiverReceiving")
			}
			return m, listenForMessages(m.p2pNode)

		case transfer.TypeChunkAck:
			LogDebug("Sender received TypeChunkAck: index=%d, state=%v", frame.Index, m.sendState)
			if m.sendState == SenderSending {
				if frame.Index == m.currChunk {
					m.currChunk++
					if m.currChunk >= m.totalChunks {
						LogDebug("Sender: all chunks acknowledged. Sending TypeFileDone...")
						// Send completion packet
						doneFrame := transfer.AegisFrame{
							Type:   transfer.TypeFileDone,
							SHA256: m.fileHash,
						}
						_ = sendFrame(m.p2pNode, doneFrame)
						m.sendState = SenderDone
						m.chatHistory = append(m.chatHistory, chatMessage{
							Sender:    "SYSTEM",
							Timestamp: time.Now(),
							Text:      fmt.Sprintf("✓ File '%s' sent successfully to peer.", m.fileName),
						})
						m.updateViewportContent()
					} else {
						LogDebug("Sender: sending chunk %d...", m.currChunk)
						// Send next chunk immediately
						return m, tea.Batch(
							listenForMessages(m.p2pNode),
							sendNextChunkCmd(m.p2pNode, m.filePath, m.currChunk, m.fileName),
						)
					}
				} else {
					LogDebug("Sender: ignored out-of-order TypeChunkAck %d (expected %d)", frame.Index, m.currChunk)
				}
			}
			return m, listenForMessages(m.p2pNode)

		case transfer.TypeFileDone:
			LogDebug("Receiver received TypeFileDone. Verifying SHA-256 hash...")
			if m.recvState == ReceiverReceiving {
				hash := sha256.Sum256(m.recvFileBytes)
				hashStr := hex.EncodeToString(hash[:])
				if hashStr == m.fileHash {
					LogDebug("Receiver: hash verification succeeded. Prompting user to save...")
					m.recvState = ReceiverSavePrompt
					m.chatHistory = append(m.chatHistory, chatMessage{
						Sender:    "SYSTEM",
						Timestamp: time.Now(),
						Text:      fmt.Sprintf("✓ File '%s' received and integrity verified. Press Ctrl+S to save it.", m.fileName),
					})
					m.updateViewportContent()
				} else {
					LogDebug("Receiver: hash verification failed! Expected: %s, Got: %s", m.fileHash, hashStr)
					m.recvState = ReceiverError
					m.transferErr = fmt.Sprintf("SHA-256 mismatch!\nExpected: %s\nGot: %s", m.fileHash, hashStr)
					m.chatHistory = append(m.chatHistory, chatMessage{
						Sender:    "SYSTEM",
						Timestamp: time.Now(),
						Text:      fmt.Sprintf("✗ File '%s' transfer failed: SHA-256 integrity check mismatch.", m.fileName),
					})
					m.updateViewportContent()
					crypto.Bytes(m.recvFileBytes)
					m.recvFileBytes = nil
				}
			} else {
				LogDebug("Receiver: ignored TypeFileDone because state is not ReceiverReceiving")
			}
			return m, listenForMessages(m.p2pNode)
		}
	}

	if m.input.Focused() {
		var ic tea.Cmd
		m.input, ic = m.input.Update(msg)
		cmds = append(cmds, ic)
	}

	return m, tea.Batch(cmds...)
}

// ── View Rendering ───────────────────────────────────────────────────────────

func (m ChatModel) View() string {
	if m.width == 0 {
		return ""
	}

	sidebarW := 34
	chatW := m.width - sidebarW - 1
	if chatW < 20 {
		chatW = 20
	}
	chatHeight := m.height - 8
	if chatHeight < 5 {
		chatHeight = 5
	}

	// 1. Build Left Chat Box
	var leftBox string
	if m.pickerMode {
		leftBox = PanelStyle.Width(chatW).Height(chatHeight).Render(m.filepicker.View())
	} else {
		leftBox = PanelStyle.Width(chatW).Height(chatHeight).Render(m.viewport.View())
	}

	// 2. Build Right Sidebar lines
	var sidebarLines []string
	sidebarLines = append(sidebarLines, PrimaryBold.Render("  AEGIS SECURE CORE"))
	sidebarLines = append(sidebarLines, HR(sidebarW-4))

	if m.p2pNode != nil && m.p2pNode.Ratchet != nil {
		r := m.p2pNode.Ratchet
		sidebarLines = append(sidebarLines, fmt.Sprintf(" %s %s", LabelStyle.Render("SEND NUM"), ValueStyle.Render(fmt.Sprintf("%d", r.SendMsgNum))))
		sidebarLines = append(sidebarLines, fmt.Sprintf(" %s %s", LabelStyle.Render("RECV NUM"), ValueStyle.Render(fmt.Sprintf("%d", r.RecvMsgNum))))
	} else {
		sidebarLines = append(sidebarLines, "  ratchet not initialized")
	}

	// Dynamic network diagnostics
	sidebarLines = append(sidebarLines, "")
	sidebarLines = append(sidebarLines, PrimaryBold.Render("  NET DIAGNOSTICS"))
	sidebarLines = append(sidebarLines, HR(sidebarW-4))

	var rttStr string
	if m.rtt == 0 {
		rttStr = "measuring..."
	} else {
		rttStr = fmt.Sprintf("%d ms", m.rtt.Milliseconds())
	}

	connType := m.connectionType
	if connType == "" {
		connType = "Direct"
	}

	var connTypeStyled string
	if connType == "Relay" {
		connTypeStyled = WarningBold.Render("RELAYED")
	} else {
		connTypeStyled = PrimaryBold.Render("DIRECT")
	}

	sidebarLines = append(sidebarLines, fmt.Sprintf(" %s %s", LabelStyle.Render("LATENCY"), ValueStyle.Render(rttStr)))
	sidebarLines = append(sidebarLines, fmt.Sprintf(" %s %s", LabelStyle.Render("CHANNEL"), connTypeStyled))

	sidebarLines = append(sidebarLines, "")
	sidebarLines = append(sidebarLines, PrimaryBold.Render("  FILE TRANSFER"))
	sidebarLines = append(sidebarLines, HR(sidebarW-4))

	if m.sendState != SenderIdle {
		sidebarLines = append(sidebarLines, " "+SecondaryText.Render("OUTGOING TRANSFER"))
		sidebarLines = append(sidebarLines, fmt.Sprintf("  %s", shorten(m.fileName, 20)))
		sidebarLines = append(sidebarLines, fmt.Sprintf("  Size: %d bytes", m.fileSize))

		switch m.sendState {
		case SenderWaitAccept:
			sidebarLines = append(sidebarLines, "  "+WarningBold.Render("WAITING FOR PEER..."))
		case SenderSending:
			sidebarLines = append(sidebarLines, "  "+PrimaryBold.Render("SENDING CHUNKS"))
			sidebarLines = append(sidebarLines, "  "+ScanBar(m.currChunk, m.totalChunks, sidebarW-12))
		case SenderDone:
			sidebarLines = append(sidebarLines, "  "+PrimaryText.Render("✓ TRANSFER COMPLETE"))
		case SenderRejected:
			sidebarLines = append(sidebarLines, "  "+WarningBold.Render("✗ PEER REJECTED OFFER"))
		case SenderError:
			sidebarLines = append(sidebarLines, "  "+WarningBold.Render("✗ ERROR:"))
			sidebarLines = append(sidebarLines, "  "+WarningText.Render(shorten(m.transferErr, 25)))
		}
	} else if m.recvState != ReceiverIdle {
		sidebarLines = append(sidebarLines, " "+SecondaryText.Render("INCOMING TRANSFER"))
		sidebarLines = append(sidebarLines, fmt.Sprintf("  %s", shorten(m.fileName, 20)))
		sidebarLines = append(sidebarLines, fmt.Sprintf("  Size: %d bytes", m.fileSize))

		switch m.recvState {
		case ReceiverOfferReceived:
			sidebarLines = append(sidebarLines, "  "+WarningBold.Render("! FILE OFFER RECEIVED"))
			sidebarLines = append(sidebarLines, "")
			sidebarLines = append(sidebarLines, "  "+WarningBold.Render("ACCEPT? [Y/N]"))
		case ReceiverReceiving:
			sidebarLines = append(sidebarLines, "  "+PrimaryBold.Render("RECEIVING CHUNKS"))
			sidebarLines = append(sidebarLines, "  "+ScanBar(m.currChunk, m.totalChunks, sidebarW-12))
		case ReceiverSavePrompt:
			sidebarLines = append(sidebarLines, "  "+PrimaryText.Render("✓ INTEGRITY VERIFIED"))
			sidebarLines = append(sidebarLines, "  "+WarningBold.Render("PRESS CTRL+S TO SAVE"))
		case ReceiverSaving:
			sidebarLines = append(sidebarLines, "  "+SecondaryText.Render("SAVING TO DISK..."))
		case ReceiverDone:
			sidebarLines = append(sidebarLines, "  "+PrimaryText.Render("✓ FILE SAVED SECURELY"))
			sidebarLines = append(sidebarLines, "")
			sidebarLines = append(sidebarLines, "  Location:")
			sidebarLines = append(sidebarLines, "  "+SubtleText.Render(shorten(m.savePath, 28)))
		case ReceiverError:
			sidebarLines = append(sidebarLines, "  "+WarningBold.Render("✗ ERROR:"))
			sidebarLines = append(sidebarLines, "  "+WarningText.Render(shorten(m.transferErr, 25)))
		}
	} else {
		sidebarLines = append(sidebarLines, "  No active transfers.")
		sidebarLines = append(sidebarLines, "")
		sidebarLines = append(sidebarLines, "  "+KeyHelp("Ctrl+F", "Send File"))
	}

	for len(sidebarLines) < chatHeight {
		sidebarLines = append(sidebarLines, "")
	}
	if len(sidebarLines) > chatHeight {
		sidebarLines = sidebarLines[:chatHeight]
	}
	sidebarContent := strings.Join(sidebarLines, "\n")

	// Render rounded side-by-side boxes
	rightBox := DimPanelStyle.Width(sidebarW).Height(chatHeight).Render(sidebarContent)
	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)

	// 3. Render Header
	headerStyle := lipgloss.NewStyle().
		Background(ColorPrimary).
		Foreground(lipgloss.Color("#000000")).
		Bold(true).
		Width(m.width).
		Align(lipgloss.Center)

	headerText := "AEGIS // SECURE ZERO-TRUST CHAT"
	if m.peerTyping {
		headerText = "AEGIS // SECURE ZERO-TRUST CHAT  [ PEER yazıyor... ]"
	}
	header := headerStyle.Render(headerText)

	// 4. Text Input field
	inputField := m.input.View()

	// Keyboard shortcut legend line
	var legend string
	if m.pickerMode {
		legend = "  " + KeyHelp("ENTER", "Select") + "   " + KeyHelp("↑/↓", "Navigate") + "   " + KeyHelp("← / h", "Parent Dir") + "   " + KeyHelp("→ / l", "Open Dir") + "   " + KeyHelp("ESC", "Cancel")
	} else if m.fileMode {
		legend = "  " + KeyHelp("ENTER", "Confirm Send Path") + "   " + KeyHelp("ESC", "Cancel")
	} else if m.saveMode {
		legend = "  " + KeyHelp("ENTER", "Confirm Save Path") + "   " + KeyHelp("ESC", "Cancel")
	} else if m.recvState == ReceiverOfferReceived {
		legend = "  " + KeyHelp("Y", "Accept File Offer") + "   " + KeyHelp("N", "Reject Offer")
	} else {
		legend = "  " + KeyHelp("ENTER", "Send Msg") + "   " + KeyHelp("Ctrl+F", "Browse File") + "   " + KeyHelp("Ctrl+O", "Enter Path") + "   " + KeyHelp("Ctrl+C", "Disconnect")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		mainRow,
		"",
		"  "+inputField,
		"",
		legend,
	)
}

// Destroy cleans up the active file buffers in RAM securely.
func (m *ChatModel) Destroy() {
	if m.recvFileBytes != nil {
		crypto.Bytes(m.recvFileBytes)
		m.recvFileBytes = nil
	}
}
