package cli

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// These tests cover the argument-validation branches of Run that never touch
// Docker, the daemon, or the network. The operational commands (up, down,
// status, doctor, exec, chat, give, take, reset-env, daemon) require a live
// environment and are exercised manually / in integration tests.

func TestRunNoArgs(t *testing.T) {
	if err := Run(nil); err == nil {
		t.Error("Run(nil) returned nil error, want usage error")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	err := Run([]string{"frobnicate"})
	if err == nil {
		t.Fatal("Run([frobnicate]) returned nil error, want error")
	}
	if want := "unknown command"; !contains(err.Error(), want) {
		t.Errorf("error = %q, want to contain %q", err.Error(), want)
	}
}

func TestRunExecEmpty(t *testing.T) {
	if err := Run([]string{"exec"}); err == nil {
		t.Error("Run([exec]) returned nil error, want usage error")
	}
	if err := Run([]string{"exec", "   "}); err == nil {
		t.Error("Run([exec, '   ']) returned nil error, want usage error")
	}
}

func TestRunRaceEmpty(t *testing.T) {
	if err := Run([]string{"race"}); err == nil {
		t.Error("Run([race]) returned nil error, want usage error")
	}
	if err := Run([]string{"race", "   "}); err == nil {
		t.Error("Run([race, '   ']) returned nil error, want usage error")
	}
}

func TestRunGiveArgCount(t *testing.T) {
	if err := Run([]string{"give"}); err == nil {
		t.Error("Run([give]) returned nil error, want usage error")
	}
	if err := Run([]string{"give", "a", "b"}); err == nil {
		t.Error("Run([give, a, b]) returned nil error, want usage error")
	}
}

func TestRunTakeArgCount(t *testing.T) {
	if err := Run([]string{"take"}); err == nil {
		t.Error("Run([take]) returned nil error, want usage error")
	}
	if err := Run([]string{"take", "a", "b", "c"}); err == nil {
		t.Error("Run([take, a, b, c]) returned nil error, want usage error")
	}
}

func TestRunResetEnvRequiresForce(t *testing.T) {
	if err := Run([]string{"reset-env"}); err == nil {
		t.Error("Run([reset-env]) returned nil error, want safety error")
	}
	if err := Run([]string{"reset-env", "maybe"}); err == nil {
		t.Error("Run([reset-env, maybe]) returned nil error, want safety error")
	}
}

func TestRunDaemonArgCount(t *testing.T) {
	if err := Run([]string{"daemon"}); err == nil {
		t.Error("Run([daemon]) returned nil error, want usage error")
	}
	if err := Run([]string{"daemon", "wobble"}); err == nil {
		t.Error("Run([daemon, wobble]) returned nil error, want usage error")
	}
}

func TestRunHelp(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantSubstr string
	}{
		{name: "help command", args: []string{"help"}, wantSubstr: "usage: zeroclaw <command>"},
		{name: "short help flag", args: []string{"-h"}, wantSubstr: "usage: zeroclaw <command>"},
		{name: "long help flag", args: []string{"--help"}, wantSubstr: "usage: zeroclaw <command>"},
		{name: "contextual help auth", args: []string{"help", "auth"}, wantSubstr: "usage: zeroclaw auth [sync|login]"},
		{name: "contextual help daemon", args: []string{"help", "daemon"}, wantSubstr: "usage: zeroclaw daemon start|run|stop"},
		{name: "contextual help give", args: []string{"help", "give"}, wantSubstr: "usage: zeroclaw give <file>"},
		{name: "contextual help take", args: []string{"help", "take"}, wantSubstr: "usage: zeroclaw take <path> [dest]"},
		{name: "contextual help exec", args: []string{"help", "exec"}, wantSubstr: `usage: zeroclaw exec "<prompt>"`},
		{name: "contextual help race", args: []string{"help", "race"}, wantSubstr: `usage: zeroclaw race "<prompt>"`},
		{name: "contextual help reset-env", args: []string{"help", "reset-env"}, wantSubstr: "usage: zeroclaw reset-env --force"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := Run(tc.args); err != nil {
					t.Errorf("Run(%v) returned error %v, want nil", tc.args, err)
				}
			})
			if !contains(out, tc.wantSubstr) {
				t.Errorf("Run(%v) printed %q, want substring %q", tc.args, out, tc.wantSubstr)
			}
		})
	}
}

func TestRunVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "version command", args: []string{"version"}},
		{name: "short version flag", args: []string{"-v"}},
		{name: "long version flag", args: []string{"--version"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := Run(tc.args); err != nil {
					t.Errorf("Run(%v) returned error %v, want nil", tc.args, err)
				}
			})
			if !contains(out, "zeroclaw") {
				t.Errorf("Run(%v) printed %q, want substring zeroclaw", tc.args, out)
			}
		})
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	os.Stdout = w

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = old
	return <-outC
}

func TestRunAuthUsage(t *testing.T) {
	if err := Run([]string{"auth", "invalid"}); err == nil {
		t.Error("Run([auth, invalid]) returned nil error, want usage error")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
