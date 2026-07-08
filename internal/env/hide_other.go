//go:build !windows

package env

import "os/exec"

func hideConsole(*exec.Cmd) {}
