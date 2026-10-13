package tui

import (
	"context"
	"errors"
	"fmt"
	"github.com/aegis-p2p/aegis/internal/crypto"
	"github.com/aegis-p2p/aegis/internal/p2p"
	"github.com/aegis-p2p/aegis/internal/transfer"
	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type chatMessage struct {
	ID                   int
	Sender, Text, Status string
	Timestamp            time.Time
}
type chatTick time.Time
type preparedFile struct {
	Generation uint64
	Upload     *transfer.Upload
	Err        error
}
type saveResult struct {
	Err  error
	Path string
}
type sendResult struct {
	Err       error
	MessageID int
}
type chunkResult struct {
	Err        error
	TransferID string
}

type ChatModel struct {
	width, height                           int
	session                                 *p2p.Session
	engine                                  *transfer.Engine
	input                                   textinput.Model
	viewport                                viewport.Model
	history                                 []chatMessage
	nextMessageID                           int
	downloads                               string
	filepicker                              filepicker.Model
	pickerMode, fileMode, saveMode          bool
	preparing                               bool
	prepareGeneration                       uint64
	overwritePrompt                         bool
	savePath                                string
	saving                                  bool
	closePrompt, closeLost, finishAfterSave bool
	showDetails                             bool
	peerTypingUntil, lastTyping             time.Time
	rtt                                     time.Duration
	lastPing                                int64
}

func NewChat(downloads string, s *p2p.Session) ChatModel {
	input := textinput.New()
	input.CharLimit = 1024
	input.Placeholder = "type a message..."
	input.Prompt = " > "
	input.Focus()
	return ChatModel{input: input, session: s, engine: &transfer.Engine{}, downloads: downloads}
}
func (m ChatModel) Init() tea.Cmd { return tea.Batch(textinput.Blink, m.tick()) }
func (m ChatModel) tick() tea.Cmd {
	s := m.session
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return chatEvent{s, chatTick(t)} })
}
func (m ChatModel) async(work func() any) tea.Cmd {
	s := m.session
	return func() tea.Msg { return chatEvent{s, work()} }
}
func (m ChatModel) send(frame transfer.AegisFrame, id int) tea.Cmd {
	s := m.session
	return m.async(func() any {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := s.Send(ctx, frame)
		return sendResult{err, id}
	})
}
func (m ChatModel) chunk() tea.Cmd {
	engine, s := m.engine, m.session
	transferID := engine.Snapshot().ID
	return m.async(func() any {
		frame, err := engine.NextChunk()
		id := transferID
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err = s.Send(ctx, frame)
			cancel()
		}
		crypto.Bytes(frame.Data)
		return chunkResult{err, id}
	})
}
func (m *ChatModel) add(sender, text, status string, id int) {
	m.history = append(m.history, chatMessage{ID: id, Sender: sender, Text: text, Status: status, Timestamp: time.Now()})
	if len(m.history) > 1000 {
		m.history = append([]chatMessage(nil), m.history[len(m.history)-1000:]...)
	}
	m.refresh()
}
func (m *ChatModel) addSystem(text string) { m.add("SYS", text, "", -1) }
func (m *ChatModel) refresh() {
	atBottom := m.viewport.AtBottom()
	offset := m.viewport.YOffset
	lines := make([]string, 0, len(m.history))
	for _, entry := range m.history {
		text := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, entry.Text)
		sender := entry.Sender
		if sender == "PEER" && m.session != nil && m.session.PeerName != "" {
			sender = m.session.PeerName
		}
		lines = append(lines, fmt.Sprintf("%s %s › %s %s", entry.Timestamp.Format("15:04:05"), sender, text, entry.Status))
	}
	m.viewport.SetContent(lipgloss.NewStyle().Width(max(10, m.viewport.Width)).Render(strings.Join(lines, "\n")))
	if atBottom {
		m.viewport.GotoBottom()
	} else {
		m.viewport.SetYOffset(offset)
	}
}
func (m ChatModel) UpdateSize(w, h int) (ChatModel, tea.Cmd) {
	m.width = w
	m.height = h
	m.viewport.Width = max(10, w-6)
	m.viewport.Height = max(3, h-12)
	m.input.Width = max(10, w-8)
	m.refresh()
	return m, nil
}
func (m *ChatModel) prepare(path string) tea.Cmd {
	if m.preparing || m.engine.Snapshot().Busy {
		m.addSystem("Another transfer is active.")
		return nil
	}
	m.preparing = true
	m.prepareGeneration++
	gen := m.prepareGeneration
	return m.async(func() any { u, err := transfer.Prepare(path); return preparedFile{gen, u, err} })
}
func (m *ChatModel) save(overwrite bool) tea.Cmd {
	m.saving = true
	engine, path, s := m.engine, m.savePath, m.session
	return m.async(func() any {
		frame, err := engine.Save(path, overwrite)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.Send(ctx, frame)
			cancel()
		}
		return saveResult{err, path}
	})
}
func (m *ChatModel) beginSave() {
	m.saveMode = true
	m.overwritePrompt = false
	m.input.CharLimit = 4096
	m.input.Placeholder = "save path or existing directory..."
	value := ""
	if m.downloads != "" {
		value = filepath.Join(m.downloads, m.engine.Snapshot().Name)
	}
	m.input.SetValue(value)
	m.input.Focus()
}
func (m *ChatModel) normalInput() {
	m.fileMode = false
	m.saveMode = false
	m.input.CharLimit = 1024
	m.input.SetValue("")
	m.input.Placeholder = "type a message..."
	m.input.Focus()
}
func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	switch v := msg.(type) {
	case chatTick:
		now := time.Time(v)
		cmds := []tea.Cmd{m.tick()}
		if frame, expired := m.engine.Expire(now); expired {
			m.addSystem("Transfer timed out; you can try again.")
			cmds = append(cmds, m.send(frame, -1))
		}
		if m.lastPing == 0 || now.UnixNano()-m.lastPing >= int64(10*time.Second) {
			m.lastPing = now.UnixNano()
			cmds = append(cmds, m.send(transfer.AegisFrame{Type: transfer.TypePing, Size: m.lastPing}, -1))
		}
		return m, tea.Batch(cmds...)
	case preparedFile:
		if v.Generation != m.prepareGeneration {
			if v.Upload != nil {
				v.Upload.Close()
			}
			return m, nil
		}
		m.preparing = false
		if v.Err != nil {
			m.addSystem("File preparation failed: " + v.Err.Error())
			return m, nil
		}
		frame, err := m.engine.Offer(v.Upload)
		if err != nil {
			m.addSystem(err.Error())
			return m, nil
		}
		m.addSystem("File offer sent: " + frame.Name)
		return m, m.send(frame, -1)
	case saveResult:
		m.saving = false
		m.overwritePrompt = false
		if errors.Is(v.Err, transfer.ErrExists) {
			m.overwritePrompt = true
			return m, nil
		}
		if v.Err != nil {
			m.addSystem("Save failed: " + v.Err.Error() + ". The file is still in memory; retry with Ctrl+S.")
			if m.finishAfterSave {
				m.closePrompt = true
			}
			return m, nil
		}
		m.addSystem("File saved to " + v.Path)
		m.normalInput()
		if m.finishAfterSave {
			return m, func() tea.Msg { return closeConfirmedMsg{} }
		}
		return m, nil
	case sendResult:
		if v.Err != nil {
			m.addSystem("Send failed: " + v.Err.Error())
			LogDebug("transfer failed")
			return m, nil
		}
		if v.MessageID >= 0 {
			for i := range m.history {
				if m.history[i].Sender == "YOU" && m.history[i].ID == v.MessageID && m.history[i].Status == "pending" {
					m.history[i].Status = "sent"
				}
			}
			m.refresh()
		}
		return m, nil
	case chunkResult:
		if v.Err != nil && m.engine.Snapshot().Busy && m.engine.Snapshot().ID == v.TransferID {
			frame := m.engine.Cancel()
			m.addSystem("File transfer failed: " + v.Err.Error())
			return m, m.send(frame, -1)
		}
		return m, nil
	case packetMsg:
		f := v.Frame
		defer crypto.Bytes(f.Data)
		switch f.Type {
		case transfer.TypeMsg:
			m.add("PEER", f.MsgText, "", f.Index)
			return m, m.send(transfer.AegisFrame{Type: transfer.TypeMsgDelivered, Index: f.Index}, -1)
		case transfer.TypeMsgDelivered:
			for i := range m.history {
				if m.history[i].Sender == "YOU" && m.history[i].ID == f.Index {
					m.history[i].Status = "received"
				}
			}
			m.refresh()
			return m, nil
		case transfer.TypeTyping:
			m.peerTypingUntil = time.Now().Add(3 * time.Second)
			return m, nil
		case transfer.TypePing:
			return m, m.send(transfer.AegisFrame{Type: transfer.TypePong, Size: f.Size}, -1)
		case transfer.TypePong:
			if f.Size == m.lastPing {
				m.rtt = time.Duration(time.Now().UnixNano() - f.Size)
			}
			return m, nil
		default:
			before := m.engine.Snapshot()
			responses, next, err := m.engine.Handle(f)
			after := m.engine.Snapshot()
			cmds := []tea.Cmd{}
			for _, response := range responses {
				cmds = append(cmds, m.send(response, -1))
			}
			if err != nil {
				m.addSystem(err.Error())
				if len(responses) == 0 && after.Busy {
					cmds = append(cmds, m.send(m.engine.Cancel(), -1))
				}
			}
			if before.State != after.State {
				m.addSystem("File: " + after.State + " — " + after.Name)
			}
			if next {
				cmds = append(cmds, m.chunk())
			}
			return m, tea.Batch(cmds...)
		}
	case tea.KeyMsg:
		if m.saving {
			return m, nil
		}
		if m.overwritePrompt {
			switch strings.ToLower(v.String()) {
			case "y":
				return m, m.save(true)
			case "n", "esc":
				m.overwritePrompt = false
				m.saveMode = false
				if m.finishAfterSave {
					m.closePrompt = true
				}
				m.normalInput()
			}
			return m, nil
		}
		if m.closePrompt {
			switch strings.ToLower(v.String()) {
			case "s":
				m.closePrompt = false
				m.finishAfterSave = true
				m.beginSave()
			case "d":
				m.engine.Close()
				return m, func() tea.Msg { return closeConfirmedMsg{} }
			}
			return m, nil
		}
		if m.engine.Snapshot().Offer && !m.pickerMode && !m.saveMode && !m.fileMode {
			switch strings.ToLower(v.String()) {
			case "y", "n":
				frame, err := m.engine.Accept(strings.ToLower(v.String()) == "y")
				if err != nil {
					m.addSystem(err.Error())
					return m, nil
				}
				return m, m.send(frame, -1)
			}
		}
		if !m.pickerMode {
			switch v.String() {
			case "ctrl+f":
				if !m.engine.Snapshot().Busy && !m.preparing {
					m.pickerMode = true
					m.filepicker = filepicker.New()
					m.filepicker.CurrentDirectory, _ = os.Getwd()
					return m, m.filepicker.Init()
				}
				return m, nil
			case "ctrl+o":
				if !m.engine.Snapshot().Busy && !m.preparing {
					m.fileMode = true
					m.input.CharLimit = 4096
					m.input.SetValue("")
					m.input.Placeholder = "file path..."
				}
				return m, nil
			case "ctrl+s":
				if m.engine.Snapshot().Unsaved {
					m.beginSave()
				}
				return m, nil
			case "ctrl+x":
				m.prepareGeneration++
				m.preparing = false
				if m.engine.Snapshot().Busy {
					return m, m.send(m.engine.Cancel(), -1)
				}
				return m, nil
			case "f2":
				m.showDetails = !m.showDetails
				return m, nil
			case "esc":
				if m.fileMode || m.saveMode {
					m.normalInput()
					if m.finishAfterSave {
						m.closePrompt = true
					}
					return m, nil
				}
				return m, func() tea.Msg { return closeRequestMsg{} }
			case "pgup", "pgdown", "up", "down":
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(v)
				return m, cmd
			case "enter":
				value := strings.TrimSpace(m.input.Value())
				if value == "" {
					return m, nil
				}
				if m.fileMode {
					m.normalInput()
					return m, m.prepare(strings.Trim(value, "\"'"))
				}
				if m.saveMode {
					m.savePath = value
					return m, m.save(false)
				}
				m.input.SetValue("")
				switch value {
				case "/quit":
					return m, func() tea.Msg { return closeRequestMsg{Quit: true} }
				case "/disconnect":
					return m, func() tea.Msg { return closeRequestMsg{} }
				case "/clear":
					m.history = nil
					m.viewport.SetContent("")
					return m, nil
				case "/help":
					m.addSystem("Ctrl+F browse, Ctrl+O file path, Ctrl+S save, Ctrl+X cancel transfer, F2 fingerprints, Esc disconnect.")
					return m, nil
				case "/status":
					m.addSystem("Encrypted connection via " + m.session.ConnectionType() + ". Peer: " + m.session.PeerID())
					return m, nil
				}
				candidate := strings.Trim(value, "\"'")
				if filepath.IsAbs(candidate) || (len(candidate) > 2 && candidate[1] == ':' && (candidate[2] == '\\' || candidate[2] == '/')) {
					return m, m.prepare(candidate)
				}
				id := m.nextMessageID
				m.nextMessageID++
				m.add("YOU", value, "pending", id)
				return m, m.send(transfer.AegisFrame{Type: transfer.TypeMsg, MsgText: value, Index: id}, id)
			}
			if !m.fileMode && !m.saveMode && v.Type == tea.KeyRunes && time.Since(m.lastTyping) > 2*time.Second {
				m.lastTyping = time.Now()
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(v)
				return m, tea.Batch(cmd, m.send(transfer.AegisFrame{Type: transfer.TypeTyping}, -1))
			}
		}
	}
	if m.pickerMode {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
			m.pickerMode = false
			return m, nil
		}
		var cmd tea.Cmd
		m.filepicker, cmd = m.filepicker.Update(msg)
		if selected, path := m.filepicker.DidSelectFile(msg); selected {
			m.pickerMode = false
			return m, m.prepare(path)
		}
		return m, cmd
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
func (m ChatModel) View() string {
	header := "AEGIS — ENCRYPTED TEMPORARY SESSION"
	if m.session.PeerName != "" {
		header += " · " + m.session.PeerName
	}
	if time.Now().Before(m.peerTypingUntil) {
		header += " · peer typing"
	}
	transferStatus := m.engine.Snapshot()
	status := fmt.Sprintf("%s · %d ms", m.session.ConnectionType(), m.rtt.Milliseconds())
	if transferStatus.State != "" {
		status += " | File: " + transferStatus.State + fmt.Sprintf(" (%d/%d)", transferStatus.Current, transferStatus.Total)
	}
	content := m.viewport.View()
	if m.pickerMode {
		content = m.filepicker.View()
	}
	lines := []string{PrimaryBold.Render(header), MutedText.Render(status), PanelStyle.Width(max(10, m.width-4)).Render(content)}
	if m.preparing {
		lines = append(lines, "Preparing file...")
	}
	if transferStatus.Offer {
		lines = append(lines, WarningBold.Render("Incoming file: "+transferStatus.Name+" — [Y] Accept [N] Reject"))
	}
	if transferStatus.Unsaved {
		lines = append(lines, WarningBold.Render("File is in memory and has NOT been saved. Ctrl+S to save."))
	}
	if m.showDetails {
		lines = append(lines, "Local: "+m.session.LocalFP, "Remote: "+m.session.RemoteFP, "Comparison words: "+m.session.SAS, "Independent comparison is optional. Names do not prove personal identity.")
	}
	if m.overwritePrompt {
		lines = append(lines, WarningBold.Render("Destination exists. [Y] Overwrite [N] Keep existing file"))
	} else if m.closePrompt {
		text := "Unsaved file: [S] Save then close [D] Discard and close"
		if !m.closeLost {
			text += " [Esc] Stay"
		}
		lines = append(lines, WarningBold.Render(text))
	} else {
		lines = append(lines, m.input.View())
	}
	lines = append(lines, KeyHelp("Ctrl+F", "File")+"  "+KeyHelp("Ctrl+S", "Save")+"  "+KeyHelp("Ctrl+X", "Cancel transfer")+"  "+KeyHelp("Esc", "Disconnect"))
	return strings.Join(lines, "\n")
}
func (m *ChatModel) Destroy() {
	if m.engine != nil {
		m.engine.Close()
	}
	m.history = nil
	m.input.SetValue("")
	m.viewport.SetContent("")
	m.prepareGeneration++
}
func disposeChatEvent(v any) {
	if prepared, ok := v.(preparedFile); ok && prepared.Upload != nil {
		prepared.Upload.Close()
	}
}
