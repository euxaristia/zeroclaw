package daemon

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"zeroclaw/internal/config"
)

// Launch ensures a zeroclawd is running, spawning `zeroclaw daemon run`
// detached from the current terminal when needed. Logs go to
// ~/.zeroclaw/daemon.log.
func Launch() error {
	if _, ok := Running(); ok {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath, err := config.Path("daemon.log")
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "daemon", "run")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawning zeroclawd: %w", err)
	}
	// Intentionally no Wait: the daemon outlives this process.
	for i := 0; i < 50; i++ {
		if _, ok := Running(); ok {
			fmt.Printf("zeroclawd running (pid %d, log %s)\n", cmd.Process.Pid, logPath)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("zeroclawd did not come up; see %s", logPath)
}

// Stop asks a running daemon to shut down. Missing daemon is not an error.
func Stop() error {
	info, ok := Running()
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
	resp.Body.Close()
	fmt.Println("zeroclawd stopped")
	return nil
}
