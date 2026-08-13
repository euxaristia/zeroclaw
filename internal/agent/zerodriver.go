package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"zeroclaw/internal/env"
)

// The agent's workspace root is its whole home: memory, skills, and projects
// are all inside its world, and the container is the boundary around it.
const workspace = env.Home

// ZeroDriver runs turns through `zero exec` inside the zeroclaw container,
// speaking stream-JSON schema v2 (see zero's docs/STREAM_JSON_PROTOCOL.md).
type ZeroDriver struct{}

var _ Driver = ZeroDriver{}

func (ZeroDriver) Doctor(container string) HealthResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := env.DockerCommandContext(ctx, "exec", container, "zero", "--version")
	out, err := cmd.CombinedOutput()
	ver := strings.TrimSpace(string(out))
	if err == nil {
		return HealthResult{
			Name: "zero inside container (" + ver + ")",
			OK:   true,
		}
	}
	return HealthResult{
		Name: "zero inside container",
		OK:   false,
		Hint: "zero binary missing or unavailable inside container",
	}
}

func (ZeroDriver) Turn(ctx context.Context, opts TurnOptions, onEvent func(Event)) (res TurnResult, err error) {
	args := []string{
		"exec", "-i", env.Container, "zero", "exec",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"-C", workspace,
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
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
	stdin, pipeErr := cmd.StdinPipe()
	if pipeErr != nil {
		return TurnResult{}, pipeErr
	}
	stdout, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		_ = stdin.Close()
		return TurnResult{}, pipeErr
	}
	if startErr := cmd.Start(); startErr != nil {
		_ = stdin.Close()
		return TurnResult{}, fmt.Errorf("starting zero exec in container: %w", startErr)
	}

	waited := false
	defer func() {
		if !waited || err != nil {
			_ = stdin.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
		}
	}()

	input, marshalErr := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"type":          "message",
		"role":          "user",
		"content":       opts.Prompt,
	})
	if marshalErr != nil {
		err = marshalErr
		return
	}
	if _, writeErr := stdin.Write(append(input, '\n')); writeErr != nil {
		err = fmt.Errorf("writing input event: %w", writeErr)
		return
	}
	stdin.Close()

	res = TurnResult{ExitCode: -1}
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
	if scanErr := sc.Err(); scanErr != nil {
		err = fmt.Errorf("reading zero events: %w", scanErr)
		return
	}
	if waitErr := cmd.Wait(); waitErr != nil && res.Status == "" {
		waited = true
		err = fmt.Errorf("zero exec failed before run_end: %w", waitErr)
		return
	}
	waited = true
	if res.Status == "" {
		err = fmt.Errorf("zero exec ended without a run_end event")
		return
	}
	return res, nil
}
