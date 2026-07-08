package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Schedule struct {
	Name string `json:"name"`
	// Every is a Go duration string ("30m", "1h"). Interval schedules keep the
	// prototype stdlib-only; cron syntax can come later.
	Every  string `json:"every"`
	Prompt string `json:"prompt"`
	// Conversation defaults to "sched-<name>".
	Conversation string `json:"conversation,omitempty"`
}

type Config struct {
	// HeartbeatEvery is a Go duration string; empty or "off" disables the
	// heartbeat.
	HeartbeatEvery string     `json:"heartbeatEvery"`
	Schedules      []Schedule `json:"schedules"`
}

var defaultConfig = Config{HeartbeatEvery: "30m"}

// Load reads ~/.zeroclaw/config.json, writing the default file first if it
// does not exist yet so the user has something to edit.
func Load() (Config, error) {
	p, err := Path("config.json")
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		out, merr := json.MarshalIndent(defaultConfig, "", "  ")
		if merr != nil {
			return Config{}, merr
		}
		if werr := os.WriteFile(p, out, 0o600); werr != nil {
			return Config{}, werr
		}
		return defaultConfig, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", p, err)
	}
	return cfg, nil
}

// Interval parses a schedule duration, returning ok=false for disabled or
// invalid values.
func Interval(every string) (time.Duration, bool) {
	if every == "" || every == "off" {
		return 0, false
	}
	d, err := time.ParseDuration(every)
	if err != nil || d < time.Minute {
		return 0, false
	}
	return d, true
}
