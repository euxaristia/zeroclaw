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

func TestBuildZeroArgsProviderEnv(t *testing.T) {
	args := buildZeroArgs(TurnOptions{Container: "c", Provider: "gitlawb-opengateway"})
	found := false
	for i, arg := range args {
		if arg == "-e" && i+1 < len(args) && args[i+1] == "ZERO_PROVIDER=gitlawb-opengateway" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing ZERO_PROVIDER in %v", args)
	}
}

func TestAnnotateStreamEventPrefersRequestedProfile(t *testing.T) {
	ev := Event{Type: "run_start", Provider: "openai", Model: "nvidia/nemotron-3-ultra-550b-a55b:free"}
	annotateStreamEvent(&ev, TurnOptions{Provider: "gitlawb-opengateway"})
	if ev.Provider != "gitlawb-opengateway" {
		t.Fatalf("provider = %q, want gitlawb-opengateway", ev.Provider)
	}
}

func TestAnnotateStreamEventLeavesKindWhenNoOverride(t *testing.T) {
	ev := Event{Type: "run_start", Provider: "openai"}
	annotateStreamEvent(&ev, TurnOptions{})
	if ev.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", ev.Provider)
	}
}

func TestAnnotateStreamEventIgnoresNonStart(t *testing.T) {
	ev := Event{Type: "text", Provider: "openai"}
	annotateStreamEvent(&ev, TurnOptions{Provider: "gitlawb-opengateway"})
	if ev.Provider != "openai" {
		t.Fatalf("non-start event mutated: %q", ev.Provider)
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
