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
	// debugMode enables verbose diagnostic output — never logs cryptographic material.
	debugMode = flag.Bool("debug", false, "enable debug output (never logs key material)")
	// relay allows overriding the default IPFS bootstrap peers with a custom multiaddr.
	relay = flag.String("relay", "", "custom relay peer multiaddr (phase 2+)")
	// downloads specifies the default directory where received files are saved.
	downloads = flag.String("downloads", "", "default directory for downloaded files")
)

func main() {
	flag.Parse()

	m := tui.NewModel(*relay, *downloads, *debugMode)
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
	)

	// Intercept OS interruption/termination signals for secure cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		p.Quit()
	}()

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "aegis: fatal:", err)
		os.Exit(1)
	}

	if finalModel != nil {
		if tm, ok := finalModel.(tui.Model); ok {
			tm.Close()
		}
	}
}
