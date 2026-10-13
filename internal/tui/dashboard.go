package tui

import (
	"fmt"
	"github.com/aegis-p2p/aegis/internal/p2p"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"time"
)

type dashPhase int

const (
	phaseName dashPhase = iota
	phaseInitializing
	phaseIdle
	phaseSearching
	phaseRequest
	phaseConnecting
)

type DashboardModel struct {
	width, height     int
	input             textinput.Model
	phase             dashPhase
	name              string
	status, errorText string
	node              *p2p.P2PNode
	results           []p2p.SearchResult
	selected          int
	incoming          *p2p.IncomingRequest
	advanced          bool
	discoverable      bool
}

func NewDashboard(_ string) DashboardModel {
	input := textinput.New()
	input.Placeholder = "choose a temporary name..."
	input.CharLimit = 32
	input.Focus()
	input.Prompt = " > "
	return DashboardModel{input: input, phase: phaseName, discoverable: true}
}
func (m DashboardModel) Init() tea.Cmd { return textinput.Blink }
func (m DashboardModel) UpdateSize(w, h int) (DashboardModel, tea.Cmd) {
	m.width = w
	m.height = h
	m.input.Width = max(10, min(70, w-10))
	return m, nil
}
func (m *DashboardModel) ready() {
	m.phase = phaseIdle
	m.incoming = nil
	m.input.CharLimit = 32
	m.input.Placeholder = "search a temporary name..."
	m.input.SetValue("")
	m.input.Focus()
}
func (m DashboardModel) View() string {
	w := max(20, min(90, m.width-4))
	lines := []string{PrimaryBold.Render("AEGIS — TEMPORARY SESSIONS"), ""}
	if w >= 70 {
		lines = append([]string{RenderBanner(), ""}, lines...)
	}
	if m.phase == phaseName {
		lines = append(lines, "Choose a name for this application session.", "No account or password is needed.", "", m.input.View(), "", KeyHelp("Enter", "Start")+"  "+KeyHelp("Esc", "Quit"))
	} else {
		lines = append(lines, "Your name: "+PrimaryBold.Render(m.name))
		visibility := "off"
		if m.discoverable {
			visibility = "on"
		}
		lines = append(lines, "Findable by name: "+visibility)
		if m.node != nil && m.advanced {
			lines = append(lines, "Address: "+m.node.GetBestAddress())
		}
		switch m.phase {
		case phaseInitializing:
			lines = append(lines, "Preparing an ephemeral identity...")
		case phaseSearching:
			lines = append(lines, "Searching local peers and the internet...", KeyHelp("Esc", "Cancel search"))
		case phaseRequest:
			if m.incoming != nil {
				lines = append(lines, "", WarningBold.Render("CONNECTION REQUEST"), fmt.Sprintf("%s · %s", m.incoming.Name, shorten(m.incoming.PeerID, 18)), "", KeyHelp("Y", "Accept")+"  "+KeyHelp("N", "Reject"))
			}
		case phaseConnecting:
			lines = append(lines, "Connecting / waiting for acceptance...", KeyHelp("Esc", "Cancel"))
		case phaseIdle:
			lines = append(lines, "", m.input.View())
			for i, result := range m.results {
				cursor := "  "
				if i == m.selected {
					cursor = "> "
				}
				availability := "available"
				if !result.Profile.Available {
					availability = "busy"
				}
				lines = append(lines, fmt.Sprintf("%s%s · %s (%s)", cursor, result.Profile.Name, shorten(result.Profile.PeerID, 18), availability))
			}
			lines = append(lines, "", KeyHelp("Enter", "Search / request selected person"), KeyHelp("↑/↓", "Select")+"  "+KeyHelp("Esc", "Clear results / quit"), KeyHelp("F2", "Findability")+"  "+KeyHelp("F3", "Advanced connection"))
		}
	}
	if m.status != "" {
		lines = append(lines, "", m.status)
	}
	if m.errorText != "" {
		lines = append(lines, "", WarningText.Render(m.errorText))
	}
	lines = append(lines, "", "Names are temporary and are not proof of a person's identity.")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, PanelStyle.Width(w).Render(strings.Join(lines, "\n")))
}
func shorten(s string, maximum int) string {
	r := []rune(s)
	if len(r) <= maximum {
		return s
	}
	h := max(1, maximum/2-2)
	return string(r[:h]) + "..." + string(r[len(r)-h:])
}
func fmtDur(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}
