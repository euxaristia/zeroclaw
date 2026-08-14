package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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

// zeroConfig is the subset of `zero config --json` zeroclaw reads. Zero
// resolves the active provider itself, and the active provider's entry
// carries the model that a turn with no override will use.
type zeroConfig struct {
	ActiveProvider string `json:"activeProvider"`
	Providers      []struct {
		Name   string `json:"name"`
		Model  string `json:"model"`
		Active bool   `json:"active"`
	} `json:"providers"`
}

// Defaults asks zero what it would use for a turn with no override, so the
// operator can see the provider and model before sending anything.
func (ZeroDriver) Defaults(ctx context.Context, container string) (Defaults, error) {
	if container == "" {
		container = env.Container
	}
	cmd := env.DockerCommandContext(ctx, "exec", container, "zero", "config", "--json")
	out, err := cmd.Output()
	if err != nil {
		return Defaults{}, fmt.Errorf("reading zero config: %w", err)
	}
	var cfg zeroConfig
	if err := json.Unmarshal(out, &cfg); err != nil {
		return Defaults{}, fmt.Errorf("parsing zero config: %w", err)
	}
	res := Defaults{Provider: cfg.ActiveProvider}
	for _, p := range cfg.Providers {
		if p.Active || p.Name == cfg.ActiveProvider {
			res.Model = p.Model
			break
		}
	}
	res.ContextWindow = zeroModelContextWindow(ctx, container, res.Model)
	return res, nil
}

// zeroModelContextWindow looks the model up in zero's own model registry,
// the same source its TUI uses for the context figure in its title bar.
// Returns 0 for anything the registry does not list (gateway-routed ids
// commonly are not), which callers treat as "unknown" rather than an error.
func zeroModelContextWindow(ctx context.Context, container, model string) int {
	if model == "" {
		return 0
	}
	cmd := env.DockerCommandContext(ctx, "exec", container, "zero", "models", "--json")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var reg struct {
		Models []struct {
			ID            string `json:"id"`
			APIModel      string `json:"apiModel"`
			ContextWindow int    `json:"contextWindow"`
		} `json:"models"`
	}
	if json.Unmarshal(out, &reg) != nil {
		return 0
	}
	for _, m := range reg.Models {
		if strings.EqualFold(m.ID, model) || strings.EqualFold(m.APIModel, model) {
			return m.ContextWindow
		}
	}
	return 0
}

func buildZeroArgs(opts TurnOptions) []string {
	container := opts.Container
	if container == "" {
		container = env.Container
	}
	args := []string{"exec"}
	if opts.Provider != "" {
		args = append(args, "-e", "ZERO_PROVIDER="+opts.Provider)
	}
	args = append(args,
		"-i", container, "zero", "exec",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"-C", workspace,
	)
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Autonomy != "" {
		args = append(args, "--auto", opts.Autonomy)
	}
	if opts.ReasoningEffort != "" {
		args = append(args, "--reasoning-effort", opts.ReasoningEffort)
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(opts.MaxTurns))
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
	return args
}

func (ZeroDriver) Turn(ctx context.Context, opts TurnOptions, onEvent func(Event)) (res TurnResult, err error) {
	args := buildZeroArgs(opts)
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
	_ = stdin.Close()

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
