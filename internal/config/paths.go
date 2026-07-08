// Package config owns zeroclaw's host-side state directory (~/.zeroclaw).
// Agent-side state lives in the container volume, never here.
package config

import (
	"os"
	"path/filepath"
)

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".zeroclaw")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func Path(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
