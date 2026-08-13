package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVisualizerOutput(t *testing.T) {
	var buf bytes.Buffer
	err := RunVisualizer(&buf, false)
	if err != nil {
		t.Fatalf("RunVisualizer returned unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ZEROCLAW SYSTEM DASHBOARD") {
		t.Errorf("output missing title, got:\n%s", out)
	}
	if !strings.Contains(out, "DAEMON STATUS") {
		t.Errorf("output missing daemon status section, got:\n%s", out)
	}
	if !strings.Contains(out, "CONTAINER RESOURCES") {
		t.Errorf("output missing container resources section, got:\n%s", out)
	}
	if !strings.Contains(out, "EXECUTION BACKENDS") {
		t.Errorf("output missing execution backends section, got:\n%s", out)
	}
}
