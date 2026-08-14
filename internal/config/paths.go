// Package config owns zeroclaw's host-side state directory (~/.zeroclaw).
// Agent-side state lives in the container volume, never here.
package config

import (
	"os"
	"path/filepath"
)

// Dir returns the base configuration directory for the given agent profile.
// Default or empty agent maps to ~/.zeroclaw, while named agents map to ~/.zeroclaw/agents/<name>.
func Dir(agent ...string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	name := "default"
	if len(agent) > 0 && agent[0] != "" {
		name = agent[0]
	}
	var dir string
	if name == "default" {
		dir = filepath.Join(home, ".zeroclaw")
	} else {
		dir = filepath.Join(home, ".zeroclaw", "agents", name)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Path returns the path to a named configuration file inside the agent's config directory.
func Path(name string, agent ...string) (string, error) {
	dir, err := Dir(agent...)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// ConfiguredAgents scans ~/.zeroclaw/agents for configured named agents.
func ConfiguredAgents() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	agentsDir := filepath.Join(home, ".zeroclaw", "agents")
	entries, err := os.ReadDir(agentsDir)
	if os.IsNotExist(err) {
		return []string{"default"}, nil
	}
	if err != nil {
		return nil, err
	}
	agents := []string{"default"}
	for _, entry := range entries {
		if entry.IsDir() {
			agents = append(agents, entry.Name())
		}
	}
	return agents, nil
}
