package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"zeroclaw/internal/env"
)

// CairnDriver runs turns through `cairn-code exec` inside the zeroclaw container,
// speaking stream-JSON schema v2.
type CairnDriver struct{}

var _ Driver = CairnDriver{}

func (CairnDriver) Turn(ctx context.Context, opts TurnOptions, onEvent func(Event)) (TurnResult, error) {
	args := []string{
		"exec", "-i", env.Container, "cairn-code", "exec",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"-C", workspace,
	}
	if opts.Autonomy != "" {
		args = append(args, "--auto", opts.Autonomy)
	}
	if opts.Attended {
		args = append(args, "--no-completion-gate")
	}
	if opts.SessionID != "" {
		args = append(args, "--resume", opts.SessionID)
	}
	if opts.NewSessionID != "" {
		args = append(args, "--init-session-id", opts.NewSessionID)
	}

	cmd := env.DockerCommandContext(ctx, args...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return TurnResult{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return TurnResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return TurnResult{}, fmt.Errorf("starting cairn-code exec in container: %w", err)
	}

	input, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"type":          "message",
		"role":          "user",
		"content":       opts.Prompt,
	})
	if err != nil {
		return TurnResult{}, err
	}
	if _, err := stdin.Write(append(input, '\n')); err != nil {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return TurnResult{}, fmt.Errorf("writing input event: %w", err)
	}
	_ = stdin.Close()

	res := TurnResult{ExitCode: -1}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev Event
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		if onEvent != nil {
			onEvent(ev)
		}
		switch ev.Type {
		case "run_start":
			res.SessionID = ev.SessionID
		case "final":
			res.Final = ev.Text
		case "run_end":
			res.Status = ev.Status
			res.ExitCode = ev.ExitCode
		}
	}
	if err := sc.Err(); err != nil {
		cmd.Wait()
		return res, fmt.Errorf("reading cairn-code events: %w", err)
	}
	if err := cmd.Wait(); err != nil && res.Status == "" {
		return res, fmt.Errorf("cairn-code exec failed before run_end: %w", err)
	}
	if res.Status == "" {
		return res, fmt.Errorf("cairn-code exec ended without a run_end event")
	}
	return res, nil
}
