package env

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunAudit(t *testing.T) {
	report := RunAudit()
	if report.Score < 0 || report.Score > 100 {
		t.Errorf("expected score between 0 and 100, got %d", report.Score)
	}
	if report.Grade == "" {
		t.Error("expected non-empty grade")
	}
	if len(report.Items) == 0 {
		t.Error("expected audit items")
	}
}

func TestAuditOutput(t *testing.T) {
	var buf bytes.Buffer
	err := Audit(&buf)
	if err != nil {
		t.Fatalf("Audit returned unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ZEROCLAW SECURITY AUDIT SCORECARD") {
		t.Errorf("output missing title, got:\n%s", out)
	}
	if !strings.Contains(out, "Score") {
		t.Errorf("output missing Score, got:\n%s", out)
	}
}
