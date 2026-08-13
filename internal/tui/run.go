package tui

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
)

// Options configures the zeroclaw chat TUI.
type Options struct {
	Conversation string
	Port         int
	Token        string
}

// Run starts the zeroclaw chat TUI and returns a process-style exit code.
func Run(ctx context.Context, opts Options) int {
	if !term.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(os.Stderr, "zeroclaw chat needs a terminal (stdin is not a TTY)")
		return 2
	}
	m := newModel(ctx, opts)
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	)
	// Give the model a way to send messages from background goroutines.
	m.send = p.Send
	// Re-set with the send function wired; NewProgram already captured the
	// value-copy, so we also stash it in a package-level for streamTurn to use.
	programSend = p.Send
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "zeroclaw: tui error:", err)
		return 1
	}
	return 0
}

// programSend is set before the program runs so background goroutines started
// by tea.Cmd can deliver messages. Only one TUI runs per process.
var programSend func(tea.Msg)
