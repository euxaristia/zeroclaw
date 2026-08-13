package cli

import (
	"zeroclaw/internal/env"
)

// handleAuth dispatches the zeroclaw auth subcommand.
func handleAuth(args []string) error {
	subArgs := []string{}
	if len(args) > 1 {
		subArgs = args[1:]
	}
	return env.InteractiveAuth(subArgs)
}
