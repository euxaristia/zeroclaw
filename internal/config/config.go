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
	// Backend selects the execution backend ("zero" [default], "cairn", or "cairn-code").
	Backend string `json:"backend,omitempty"`
	// HeartbeatEvery is a Go duration string; empty or "off" disables the
	// heartbeat.
	HeartbeatEvery string     `json:"heartbeatEvery"`
	Schedules      []Schedule `json:"schedules"`
	// Telegram is the M3 chat channel. Empty token disables the channel.
	Telegram Telegram `json:"telegram"`
}

type Telegram struct {
	// Token is the Bot API token from @BotFather. Empty disables the channel.
	Token string `json:"token"`
	// AllowedChats is the single-owner allowlist. Each entry is a chat id
	// (numeric string) permitted to drive the agent. Empty means no one is
	// allowed, so a misconfigured token cannot receive commands.
	AllowedChats []string `json:"allowedChats"`
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

// LoadTelegram reads the Telegram channel config, returning ok=false when the
// channel is disabled (no token) so callers skip startup cleanly.
func LoadTelegram() (Telegram, bool, error) {
	cfg, err := Load()
	if err != nil {
		return Telegram{}, false, err
	}
	if cfg.Telegram.Token == "" {
		return Telegram{}, false, nil
	}
	return cfg.Telegram, true, nil
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
