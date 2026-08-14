package daemon

import (
	"os"
	"testing"
)

func TestInfoURL(t *testing.T) {
	i := Info{Port: 1234, Token: "secret"}
	if got := i.url("/status"); got != "http://127.0.0.1:1234/status" {
		t.Errorf("url(/status) = %q, want http://127.0.0.1:1234/status", got)
	}
	if got := i.url("/beat"); got != "http://127.0.0.1:1234/beat" {
		t.Errorf("url(/beat) = %q, want http://127.0.0.1:1234/beat", got)
	}
}

func TestSaveLoadInfo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	if got, err := os.UserHomeDir(); err != nil || got != tmp {
		t.Skipf("os.UserHomeDir did not resolve to temp home (%q); skipping", got)
	}

	in := Info{Port: 4321, Token: "abc", PID: 99}
	if err := saveInfo(in); err != nil {
		t.Fatalf("saveInfo: %v", err)
	}
	out, err := LoadInfo()
	if err != nil {
		t.Fatalf("LoadInfo: %v", err)
	}
	if out != in {
		t.Errorf("round-trip = %+v, want %+v", out, in)
	}
}

func TestNamedAgentSaveLoadInfo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	if got, err := os.UserHomeDir(); err != nil || got != tmp {
		t.Skipf("os.UserHomeDir did not resolve to temp home (%q); skipping", got)
	}

	in := Info{Port: 8765, Token: "xyz", PID: 123}
	if err := saveInfo(in, "work"); err != nil {
		t.Fatalf("saveInfo(work): %v", err)
	}
	out, err := LoadInfo("work")
	if err != nil {
		t.Fatalf("LoadInfo(work): %v", err)
	}
	if out != in {
		t.Errorf("named agent round-trip = %+v, want %+v", out, in)
	}
	removeInfo("work")
	if _, err := LoadInfo("work"); err == nil {
		t.Error("LoadInfo(work) after removeInfo returned nil error, want error")
	}
}

func TestLoadInfoMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	if got, err := os.UserHomeDir(); err != nil || got != tmp {
		t.Skipf("os.UserHomeDir did not resolve to temp home (%q); skipping", got)
	}
	if _, err := LoadInfo(); err == nil {
		t.Error("LoadInfo on missing handle returned nil error, want error")
	}
}

func TestRunningWithNoDaemon(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	if got, err := os.UserHomeDir(); err != nil || got != tmp {
		t.Skipf("os.UserHomeDir did not resolve to temp home (%q); skipping", got)
	}
	if _, ok := Running(); ok {
		t.Error("Running() = true with no daemon handle, want false")
	}
}
