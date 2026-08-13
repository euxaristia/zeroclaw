package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRaceOutput(t *testing.T) {
	var buf bytes.Buffer
	// Pass invalid/mock backends to verify race runner error formatting and table rendering
	err := RunRace(&buf, "test prompt", []string{"unknown1", "unknown2"})
	if err != nil {
		t.Fatalf("RunRace returned unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "zeroclaw race") {
		t.Errorf("output missing badge title, got:\n%s", out)
	}
	if !strings.Contains(out, "BACKEND") || !strings.Contains(out, "DURATION") {
		t.Errorf("output missing table header, got:\n%s", out)
	}
}
