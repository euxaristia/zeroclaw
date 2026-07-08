package cli

import "testing"

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

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
