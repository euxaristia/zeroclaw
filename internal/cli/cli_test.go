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

// TestRunNoExtraArgs covers noExtraArgs's rejection path for every
// subcommand that takes none. It never reaches daemon/docker code: the
// check runs before any of that, so an unrecognized trailing argument
// (e.g. the reported `zeroclaw web --dev`, which isn't a real flag) errors
// immediately instead of being silently ignored.
func TestRunNoExtraArgs(t *testing.T) {
	for _, cmd := range []string{"list", "up", "down", "status", "doctor", "audit", "web", "chat", "beat", "reset-container"} {
		err := Run([]string{cmd, "--dev"})
		if err == nil {
			t.Errorf("Run([%s, --dev]) returned nil error, want rejection of the unknown argument", cmd)
			continue
		}
		if want := "takes no arguments"; !contains(err.Error(), want) {
			t.Errorf("Run([%s, --dev]) error = %q, want to contain %q", cmd, err.Error(), want)
		}
	}
}

func TestParseAgentAndArgs(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		args      []string
		wantAgent string
		wantArgs  []string
		wantErr   bool
	}{
		{
			name:      "default when empty",
			args:      []string{"status"},
			wantAgent: "default",
			wantArgs:  []string{"status"},
		},
		{
			name:      "short flag -a",
			args:      []string{"-a", "work", "up"},
			wantAgent: "work",
			wantArgs:  []string{"up"},
		},
		{
			name:      "long flag --agent",
			args:      []string{"--agent", "lab", "chat"},
			wantAgent: "lab",
			wantArgs:  []string{"chat"},
		},
		{
			name:      "equal flag --agent=foo",
			args:      []string{"--agent=foo", "status"},
			wantAgent: "foo",
			wantArgs:  []string{"status"},
		},
		{
			name:      "equal flag -a=bar",
			args:      []string{"-a=bar", "status"},
			wantAgent: "bar",
			wantArgs:  []string{"status"},
		},
		{
			name:      "env var fallback",
			env:       "staging",
			args:      []string{"doctor"},
			wantAgent: "staging",
			wantArgs:  []string{"doctor"},
		},
		{
			name:    "missing -a value",
			args:    []string{"-a"},
			wantErr: true,
		},
		{
			name:    "missing --agent value",
			args:    []string{"--agent"},
			wantErr: true,
		},
		{
			name:    "empty --agent= value",
			args:    []string{"--agent=", "status"},
			wantErr: true,
		},
		{
			name:    "empty -a= value",
			args:    []string{"-a=", "status"},
			wantErr: true,
		},
		{
			name:    "traversal in -a",
			args:    []string{"-a", "../etc", "status"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("ZEROCLAW_AGENT", tc.env)
			} else {
				t.Setenv("ZEROCLAW_AGENT", "")
			}
			agent, rest, err := parseAgentAndArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseAgentAndArgs(%v) returned nil error, want error", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAgentAndArgs(%v) unexpected error: %v", tc.args, err)
			}
			if agent != tc.wantAgent {
				t.Errorf("agent = %q, want %q", agent, tc.wantAgent)
			}
			if len(rest) != len(tc.wantArgs) {
				t.Fatalf("rest len = %d, want %d", len(rest), len(tc.wantArgs))
			}
			for i := range rest {
				if rest[i] != tc.wantArgs[i] {
					t.Errorf("rest[%d] = %q, want %q", i, rest[i], tc.wantArgs[i])
				}
			}
		})
	}
}

func TestRunInvalidAgentFlag(t *testing.T) {
	if err := Run([]string{"-a"}); err == nil {
		t.Error("Run([-a]) returned nil error, want error")
	}
	if err := Run([]string{"--agent="}); err == nil {
		t.Error("Run([--agent=]) returned nil error, want error")
	}
	if err := Run([]string{"-a", "../escape", "status"}); err == nil {
		t.Error("Run([-a, ../escape, status]) returned nil error, want error")
	}
}

func TestRunHelp(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantSubstr string
	}{
		{name: "help command", args: []string{"help"}, wantSubstr: "usage: zeroclaw [-a <agent>] <command>"},
		{name: "short help flag", args: []string{"-h"}, wantSubstr: "usage: zeroclaw [-a <agent>] <command>"},
		{name: "long help flag", args: []string{"--help"}, wantSubstr: "usage: zeroclaw [-a <agent>] <command>"},
		{name: "contextual help list", args: []string{"help", "list"}, wantSubstr: "usage: zeroclaw list"},
		{name: "contextual help auth", args: []string{"help", "auth"}, wantSubstr: "usage: zeroclaw [-a <agent>] auth [sync|login]"},
		{name: "contextual help daemon", args: []string{"help", "daemon"}, wantSubstr: "usage: zeroclaw [-a <agent>] daemon start|run|stop"},
		{name: "contextual help give", args: []string{"help", "give"}, wantSubstr: "usage: zeroclaw [-a <agent>] give <file>"},
		{name: "contextual help take", args: []string{"help", "take"}, wantSubstr: "usage: zeroclaw [-a <agent>] take <path> [dest]"},
		{name: "contextual help exec", args: []string{"help", "exec"}, wantSubstr: `usage: zeroclaw [-a <agent>] exec "<prompt>"`},
		{name: "contextual help web", args: []string{"help", "web"}, wantSubstr: "usage: zeroclaw [-a <agent>] web|chat"},
		{name: "contextual help chat", args: []string{"help", "chat"}, wantSubstr: "usage: zeroclaw [-a <agent>] web|chat"},
		{name: "contextual help race", args: []string{"help", "race"}, wantSubstr: `usage: zeroclaw race "<prompt>"`},
		{name: "contextual help reset-container", args: []string{"help", "reset-container"}, wantSubstr: "usage: zeroclaw [-a <agent>] reset-container"},
		{name: "contextual help reset-env", args: []string{"help", "reset-env"}, wantSubstr: "usage: zeroclaw [-a <agent>] reset-env --force"},
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
