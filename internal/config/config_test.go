package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInterval(t *testing.T) {
	tests := []struct {
		in      string
		wantOK  bool
		wantMin time.Duration
	}{
		{"", false, 0},
		{"off", false, 0},
		{"30m", true, 30 * time.Minute},
		{"1h", true, time.Hour},
		{"59s", false, 0},                // below the 1-minute floor
		{"1m", true, time.Minute},        // exactly at the floor
		{"not-a-duration", false, 0},     // unpar
	}
	for _, tc := range tests {
		got, ok := Interval(tc.in)
		if ok != tc.wantOK {
			t.Errorf("Interval(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			continue
		}
		if ok && got < tc.wantMin {
			t.Errorf("Interval(%q) = %v, want >= %v", tc.in, got, tc.wantMin)
		}
	}
}

// withTempHome points os.UserHomeDir at a fresh temp dir for the duration of
// the test so config file IO never touches the real ~/.zeroclaw. If the
// runtime does not resolve the home dir to our temp dir (e.g. an unusual
// setup), the test is skipped rather than risk clobbering the user's config.
func withTempHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	// os.UserHomeDir reads HOME on Unix and USERPROFILE on Windows.
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	if got, err := os.UserHomeDir(); err != nil || got != tmp {
		t.Skipf("os.UserHomeDir did not resolve to temp home (%q); skipping file IO test", got)
	}
	return tmp
}

func TestDirAndPath(t *testing.T) {
	tmp := withTempHome(t)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	want := filepath.Join(tmp, ".zeroclaw")
	if dir != want {
		t.Errorf("Dir = %q, want %q", dir, want)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("Dir did not create %q: %v", dir, err)
	}

	p, err := Path("config.json")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if p != filepath.Join(want, "config.json") {
		t.Errorf("Path = %q, want %q", p, filepath.Join(want, "config.json"))
	}
}

func TestLoadDefault(t *testing.T) {
	tmp := withTempHome(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HeartbeatEvery != "30m" {
		t.Errorf("default HeartbeatEvery = %q, want 30m", cfg.HeartbeatEvery)
	}

	p := filepath.Join(tmp, ".zeroclaw", "config.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	var round Config
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("written config not valid json: %v", err)
	}
	if round.HeartbeatEvery != "30m" {
		t.Errorf("persisted config heartbeat = %q, want 30m", round.HeartbeatEvery)
	}
}

func TestLoadReadsExisting(t *testing.T) {
	tmp := withTempHome(t)

	p := filepath.Join(tmp, ".zeroclaw", "config.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	existing := `{"heartbeatEvery":"15m","schedules":[{"name":"nightly","every":"24h","prompt":"review"}]}`
	if err := os.WriteFile(p, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HeartbeatEvery != "15m" {
		t.Errorf("HeartbeatEvery = %q, want 15m", cfg.HeartbeatEvery)
	}
	if len(cfg.Schedules) != 1 || cfg.Schedules[0].Name != "nightly" {
		t.Errorf("schedules not loaded: %+v", cfg.Schedules)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	tmp := withTempHome(t)

	p := filepath.Join(tmp, ".zeroclaw", "config.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Error("Load returned nil error for invalid json, want error")
	}
}
