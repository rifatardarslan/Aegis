// Package main is the entry point for the Aegis secure P2P terminal messenger.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aegis-p2p/aegis/internal/tui"
)

var (
	// debugMode enables fixed, non-sensitive stderr diagnostic categories.
	debugMode = flag.Bool("debug", false, "enable non-sensitive diagnostic categories on stderr")
	// relay adds a preferred custom circuit relay candidate.
	relay = flag.String("relay", "", "custom relay peer multiaddr")
	// downloads specifies the default directory where received files are saved.
	downloads = flag.String("downloads", "", "default directory for downloaded files")
)

func main() {
	flag.Parse()

	m := tui.NewModel(*relay, *downloads, *debugMode)
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithoutSignalHandler(),
	)

	// Intercept OS interruption/termination signals for secure cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	programDone := make(chan struct{})
	go func() {
		for {
			select {
			case <-programDone:
				return
			case <-sigChan:
				p.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
			}
		}
	}()

	finalModel, err := p.Run()
	close(programDone)
	signal.Stop(sigChan)
	switch tm := finalModel.(type) {
	case tui.Model:
		tm.Close()
	case *tui.Model:
		tm.Close()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "aegis: fatal:", err)
		os.Exit(1)
	}
}
