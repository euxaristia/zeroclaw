package daemon

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"zeroclaw/internal/config"
)

// Launch ensures a zeroclawd is running for the given agent, spawning
// `zeroclaw [-a <agent>] daemon run` detached from the current terminal when needed.
// Logs go to ~/.zeroclaw/daemon.log (or ~/.zeroclaw/agents/<name>/daemon.log).
func Launch(agent ...string) error {
	agentName := "default"
	if len(agent) > 0 && agent[0] != "" {
		agentName = agent[0]
	}
	if _, ok := Running(agentName); ok {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath, err := config.Path("daemon.log", agentName)
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()

	var args []string
	if agentName != "default" {
		args = append(args, "-a", agentName)
	}
	args = append(args, "daemon", "run")

	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawning zeroclawd: %w", err)
	}
	// Intentionally no Wait: the daemon outlives this process.
	for i := 0; i < 50; i++ {
		if _, ok := Running(agentName); ok {
			fmt.Printf("zeroclawd running for agent %s (pid %d, log %s)\n", agentName, cmd.Process.Pid, logPath)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("zeroclawd did not come up; see %s", logPath)
}

// Beat asks a running daemon to fire a heartbeat turn immediately.
func Beat(agent ...string) error {
	agentName := "default"
	if len(agent) > 0 && agent[0] != "" {
		agentName = agent[0]
	}
	info, ok := Running(agentName)
	if !ok {
		return fmt.Errorf("zeroclawd is not running for agent %s; run `zeroclaw up`", agentName)
	}
	req, err := http.NewRequest(http.MethodPost, info.url("/beat"), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+info.Token)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	logPath, _ := config.Path("daemon.log", agentName)
	fmt.Printf("heartbeat fired for agent %s; watch %s\n", agentName, logPath)
	return nil
}

// Stop asks a running daemon to shut down. Missing daemon is not an error.
func Stop(agent ...string) error {
	agentName := "default"
	if len(agent) > 0 && agent[0] != "" {
		agentName = agent[0]
	}
	info, ok := Running(agentName)
	if !ok {
		return nil
	}
	req, err := http.NewRequest(http.MethodPost, info.url("/shutdown"), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+info.Token)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	fmt.Printf("zeroclawd stopped for agent %s\n", agentName)
	return nil
}
