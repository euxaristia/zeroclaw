package agent

import (
	"strings"
	"testing"
)

// The per-run overrides only reach the backend if buildZeroArgs emits the
// flags, so assert on the argv rather than trusting the field assignment.
func TestBuildZeroArgsThreadsEffortAndTurns(t *testing.T) {
	argv := strings.Join(buildZeroArgs(TurnOptions{
		Container:       "c",
		ReasoningEffort: "high",
		MaxTurns:        7,
	}), " ")

	if !strings.Contains(argv, "--reasoning-effort high") {
		t.Errorf("missing --reasoning-effort in %q", argv)
	}
	if !strings.Contains(argv, "--max-turns 7") {
		t.Errorf("missing --max-turns in %q", argv)
	}
}

// Unset overrides must leave the backend's own defaults alone rather than
// passing an empty or zero value through.
func TestBuildZeroArgsOmitsUnsetOptions(t *testing.T) {
	for _, arg := range buildZeroArgs(TurnOptions{Container: "c"}) {
		if arg == "--reasoning-effort" || arg == "--max-turns" {
			t.Errorf("unset override should not be passed, found %q", arg)
		}
	}
}
