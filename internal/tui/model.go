package tui

import (
	"context"
	"github.com/aegis-p2p/aegis/internal/p2p"
	"github.com/aegis-p2p/aegis/internal/transfer"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

type Screen int

const (
	ScreenDashboard Screen = iota
	ScreenChat
)

type initResult struct {
	Generation uint64
	Node       *p2p.P2PNode
	Err        error
}
type nodeEvent struct {
	Node  *p2p.P2PNode
	Event p2p.Event
}
type searchResult struct {
	Node      *p2p.P2PNode
	Operation uint64
	Results   []p2p.SearchResult
	Err       error
}
type connectResult struct {
	Node      *p2p.P2PNode
	Operation uint64
	Session   *p2p.Session
	Err       error
}
type packetMsg struct {
	Session *p2p.Session
	Frame   transfer.AegisFrame
	Err     error
}
type chatEvent struct {
	Session *p2p.Session
	Value   any
}
type closeRequestMsg struct{ Quit bool }
type closeConfirmedMsg struct{}

func listenNode(n *p2p.P2PNode) tea.Cmd {
	return func() tea.Msg {
		select {
		case e := <-n.Events:
			return nodeEvent{n, e}
		case <-n.Done():
			return nil
		}
	}
}
func listenMessages(s *p2p.Session) tea.Cmd {
	return func() tea.Msg { f, err := s.Read(); return packetMsg{s, f, err} }
}

// Model owns UI lifetime. Async results carry their node, operation or session
// identity, so work from a previous session can never change a new one.
type Model struct {
	screen                 Screen
	width, height          int
	dashboard              DashboardModel
	chat                   ChatModel
	node                   *p2p.P2PNode
	session                *p2p.Session
	customRelay, downloads string
	generation, operation  uint64
	operationCancel        context.CancelFunc
	closing                bool
	quitAfter              bool
	lost                   bool
}

func NewModel(relay, downloads string, debug bool) Model {
	SetDebug(debug)
	return Model{dashboard: NewDashboard(relay), customRelay: relay, downloads: downloads}
}
func (m Model) Init() tea.Cmd { return m.dashboard.Init() }
func (m *Model) initialize(name string) tea.Cmd {
	m.generation++
	gen := m.generation
	relay := m.customRelay
	name = strings.TrimSpace(name)
	m.dashboard.name = name
	m.dashboard.phase = phaseInitializing
	m.dashboard.errorText = ""
	return func() tea.Msg { n, err := p2p.NewP2PNode(name, relay); return initResult{gen, n, err} }
}
func (m *Model) cancelOperation() {
	if m.operationCancel != nil {
		m.operationCancel()
		m.operationCancel = nil
	}
	m.operation++
}
func (m *Model) finishClose() (tea.Model, tea.Cmd) {
	m.cancelOperation()
	if m.node != nil {
		_ = m.node.Close()
	}
	m.chat.Destroy()
	m.node = nil
	m.session = nil
	m.closing = false
	m.lost = false
	if m.quitAfter {
		return m, tea.Quit
	}
	name := m.dashboard.name
	visible := m.dashboard.discoverable
	m.screen = ScreenDashboard
	m.dashboard = NewDashboard(m.customRelay)
	name = strings.TrimSpace(name)
	m.dashboard.name = name
	m.dashboard.discoverable = visible
	m.dashboard, _ = m.dashboard.UpdateSize(m.width, m.height)
	return m, m.initialize(name)
}
func (m *Model) requestClose(quit, lost bool) (tea.Model, tea.Cmd) {
	m.quitAfter = quit
	m.lost = lost
	if m.screen == ScreenChat && m.chat.engine.Snapshot().Unsaved {
		m.closing = true
		m.chat.closePrompt = true
		m.chat.closeLost = lost
		return m, nil
	}
	return m, m.closeCmd()
}
func (m *Model) closeCmd() tea.Cmd { return func() tea.Msg { return closeConfirmedMsg{} } }
func (m *Model) startChat(s *p2p.Session) tea.Cmd {
	m.cancelOperation()
	m.session = s
	m.screen = ScreenChat
	m.dashboard.incoming = nil
	m.chat = NewChat(m.downloads, s)
	m.chat, _ = m.chat.UpdateSize(m.width, m.height)
	return tea.Batch(m.chat.Init(), listenMessages(s))
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.dashboard, _ = m.dashboard.UpdateSize(v.Width, v.Height)
		if m.chat.engine != nil {
			m.chat, _ = m.chat.UpdateSize(v.Width, v.Height)
		}
		return m, nil
	case initResult:
		if v.Generation != m.generation {
			if v.Node != nil {
				_ = v.Node.Close()
			}
			return m, nil
		}
		if v.Err != nil {
			m.dashboard.phase = phaseName
			m.dashboard.errorText = v.Err.Error()
			return m, nil
		}
		m.node = v.Node
		m.dashboard.node = v.Node
		m.node.SetDiscoverable(m.dashboard.discoverable)
		m.dashboard.ready()
		return m, listenNode(m.node)
	case nodeEvent:
		if v.Node != m.node {
			if v.Event.Session != nil {
				v.Event.Session.Close()
			}
			if v.Event.Request != nil {
				v.Event.Request.Decide(false)
			}
			return m, nil
		}
		next := listenNode(m.node)
		if v.Event.Request != nil {
			m.cancelOperation()
			m.dashboard.incoming = v.Event.Request
			m.dashboard.phase = phaseRequest
			m.dashboard.results = nil
			return m, next
		}
		if v.Event.Session != nil {
			return m, tea.Batch(next, m.startChat(v.Event.Session))
		}
		if v.Event.Err != nil && m.screen == ScreenDashboard {
			m.dashboard.ready()
			m.dashboard.errorText = v.Event.Err.Error()
		}
		return m, next
	case searchResult:
		if v.Node != m.node || v.Operation != m.operation {
			return m, nil
		}
		m.dashboard.phase = phaseIdle
		m.dashboard.results = v.Results
		m.dashboard.selected = 0
		m.dashboard.errorText = ""
		if v.Err != nil {
			LogDebug("search failed")
			m.dashboard.errorText = v.Err.Error()
		} else if len(v.Results) == 0 {
			m.dashboard.status = "No live matching person found. Try again after discovery is ready."
		} else {
			m.dashboard.status = "Select a person and press Enter to send a request."
		}
		return m, nil
	case connectResult:
		if v.Node != m.node || v.Operation != m.operation {
			if v.Session != nil {
				v.Session.Close()
			}
			return m, nil
		}
		if v.Err != nil {
			m.dashboard.ready()
			m.dashboard.errorText = v.Err.Error()
			return m, nil
		}
		return m, m.startChat(v.Session)
	case packetMsg:
		if v.Session != m.session {
			cryptoFrameClear(v.Frame)
			return m, nil
		}
		if v.Err != nil {
			LogDebug("session closed")
			m.chat.addSystem("Connection ended: " + v.Err.Error())
			m.session = nil
			go v.Session.Close()
			return m.requestClose(false, true)
		}
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(v)
		return m, tea.Batch(cmd, listenMessages(v.Session))
	case chatEvent:
		if v.Session != m.chat.session || m.screen != ScreenChat {
			disposeChatEvent(v.Value)
			return m, nil
		}
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(v.Value)
		return m, cmd
	case closeRequestMsg:
		return m.requestClose(v.Quit, false)
	case closeConfirmedMsg:
		return m.finishClose()
	case tea.KeyMsg:
		if v.Type == tea.KeyCtrlC {
			return m.requestClose(true, false)
		}
		if m.screen == ScreenDashboard {
			return m.updateDashboard(v)
		}
		if m.closing && v.String() == "esc" && !m.lost {
			m.closing = false
			m.chat.closePrompt = false
			m.chat.finishAfterSave = false
			return m, nil
		}
	}
	if m.screen == ScreenChat {
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(msg)
		return m, cmd
	}
	return m, nil
}
func (m Model) updateDashboard(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := &m.dashboard
	switch d.phase {
	case phaseName:
		if key.String() == "esc" {
			m.quitAfter = true
			return m, tea.Quit
		}
		if key.String() == "enter" {
			name := d.input.Value()
			if _, err := p2p.NormalizeName(name); err != nil {
				d.errorText = err.Error()
				return m, nil
			}
			return m, m.initialize(name)
		}
	case phaseInitializing:
		return m, nil
	case phaseRequest:
		if key.String() == "y" || key.String() == "Y" {
			d.incoming.Decide(true)
			d.phase = phaseConnecting
			return m, nil
		}
		if key.String() == "n" || key.String() == "N" || key.String() == "esc" {
			d.incoming.Decide(false)
			d.ready()
			return m, nil
		}
		return m, nil
	case phaseSearching, phaseConnecting:
		if key.String() == "esc" {
			m.cancelOperation()
			if d.phase == phaseConnecting {
				if d.incoming != nil {
					d.incoming.Decide(false)
				}
				m.quitAfter = false
				return m, m.closeCmd()
			}
			d.ready()
			d.status = "Cancelled."
		}
		return m, nil
	case phaseIdle:
		switch key.String() {
		case "f2":
			d.discoverable = !d.discoverable
			m.node.SetDiscoverable(d.discoverable)
			return m, nil
		case "f3":
			d.advanced = !d.advanced
			d.results = nil
			d.input.SetValue("")
			if d.advanced {
				d.input.CharLimit = 2048
				d.input.Placeholder = "Peer ID or full multiaddress..."
			} else {
				d.input.CharLimit = 32
				d.input.Placeholder = "search a temporary name..."
			}
			return m, nil
		case "up":
			d.selected = max(0, d.selected-1)
			return m, nil
		case "down":
			d.selected = min(max(0, len(d.results)-1), d.selected+1)
			return m, nil
		case "esc":
			if len(d.results) > 0 {
				d.results = nil
				d.status = ""
				return m, nil
			}
			m.quitAfter = true
			return m, m.closeCmd()
		case "enter":
			m.cancelOperation()
			op := m.operation
			n := m.node
			ctx, cancel := context.WithCancel(context.Background())
			m.operationCancel = cancel
			if d.advanced || len(d.results) > 0 {
				target := d.input.Value()
				if !d.advanced {
					result := d.results[d.selected]
					if !result.Profile.Available {
						d.errorText = "This person is busy."
						cancel()
						return m, nil
					}
					target = result.Profile.PeerID
				}
				d.phase = phaseConnecting
				d.errorText = ""
				return m, func() tea.Msg { s, err := n.Connect(ctx, target); return connectResult{n, op, s, err} }
			}
			name := d.input.Value()
			if _, err := p2p.NormalizeName(name); err != nil {
				cancel()
				d.errorText = err.Error()
				return m, nil
			}
			d.phase = phaseSearching
			d.errorText = ""
			d.status = ""
			return m, func() tea.Msg { results, err := n.Search(ctx, name); return searchResult{n, op, results, err} }
		}
	}
	if len(d.results) > 0 {
		d.results = nil
		d.status = ""
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(key)
	return m, cmd
}
func (m Model) View() string {
	if m.width == 0 {
		return "initializing aegis..."
	}
	if m.screen == ScreenChat {
		return m.chat.View()
	}
	return m.dashboard.View()
}
func (m *Model) Close() {
	m.cancelOperation()
	if m.node != nil {
		_ = m.node.Close()
	}
	if m.chat.engine != nil {
		m.chat.Destroy()
	}
}
func cryptoFrameClear(f transfer.AegisFrame) {
	for i := range f.Data {
		f.Data[i] = 0
	}
}
