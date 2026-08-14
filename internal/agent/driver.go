// Package agent defines the harness-driver boundary. Zeroclaw talks to an
// execution backend only through the Driver interface; everything specific to
// zero (flags, stream-JSON, session semantics) stays inside zerodriver.go.
package agent

import (
	"context"
	"fmt"
	"io"

	"zeroclaw/internal/env"
)

// Event is the driver-neutral projection of a backend progress event. Field
// names follow zero's stream-JSON schema v2 because it is the first backend,
// but nothing outside this package may assume zero is on the other side.
type Event struct {
	SchemaVersion int    `json:"schemaVersion"`
	Type          string `json:"type"`
	RunID         string `json:"runId"`
	SessionID     string `json:"sessionId"`
	Delta         string `json:"delta"`
	Text          string `json:"text"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	ExitCode      int    `json:"exitCode"`
	Code          string `json:"code"`
	Message       string `json:"message"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	// Token counts from a "usage" event. The backend reports these per run;
	// zeroclaw passes them through so a client can show context pressure
	// rather than waiting to be surprised by automatic compaction.
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
	Display       struct {
		Kind    string `json:"kind"`
		Summary string `json:"summary"`
	} `json:"display"`
}

type TurnOptions struct {
	// Container overrides the target container name (defaults to env.Container).
	Container string
	// SessionID resumes an existing backend session when set.
	SessionID string
	// NewSessionID asks the backend to create the session under this id, so
	// zeroclaw owns the conversation-to-session mapping.
	NewSessionID string
	Prompt       string
	// Provider requests a specific provider profile override for the turn.
	Provider string
	// Model requests a specific LLM model override for the execution turn.
	Model string
	// ReasoningEffort is low, medium, or high, for models that support it.
	// Empty leaves the backend's own default alone.
	ReasoningEffort string
	// MaxTurns caps the agent loop's tool turns for this run. Zero leaves
	// the backend's default in place.
	MaxTurns int
	// Autonomy is low, medium, or high. Inside the container boundary the
	// daemon will run high; the M0 CLI path defaults to medium.
	Autonomy string
	// Attended marks a turn with an operator present to reply (chat, exec,
	// telegram). The driver relaxes the backend's headless completion gate for
	// these: "I'm blocked on X, say the word and I'll continue" is a complete
	// conversational answer, not an unfinished task. Scheduled turns
	// (heartbeats) leave this false so unattended runs still surface
	// INCOMPLETE honestly.
	Attended bool
}

type TurnResult struct {
	SessionID string
	Final     string
	Status    string
	ExitCode  int
}

// Defaults is what the backend will use for a turn that requests no
// provider or model override. Zeroclaw itself holds no default: an empty
// TurnOptions.Provider/Model leaves the choice to the backend's own
// configuration, so the only way to display it before the first turn is to
// ask the backend.
type Defaults struct {
	Provider string
	Model    string
	// ContextWindow is the active model's context size in tokens, 0 when
	// the backend does not know it (its registry need not enumerate every
	// gateway-routed model id).
	ContextWindow int
}

type Driver interface {
	Turn(ctx context.Context, opts TurnOptions, onEvent func(Event)) (TurnResult, error)
	// Defaults reports the backend's active provider and model. Callers
	// treat an error as "unknown" rather than fatal; it is display sugar.
	Defaults(ctx context.Context, container string) (Defaults, error)
}

// NewDriver constructs the execution driver for the requested backend.
// Supported backends: "zero" (default when empty).
func NewDriver(backend string) (Driver, error) {
	switch backend {
	case "", "zero":
		return ZeroDriver{}, nil
	default:
		return nil, fmt.Errorf("unknown execution backend: %q (supported: zero)", backend)
	}
}

type HealthResult struct {
	Name string
	OK   bool
	Hint string
}

func Doctor(container string) []HealthResult {
	return []HealthResult{
		ZeroDriver{}.Doctor(container),
	}
}

func init() {
	env.RegisterBackendDoctor(func(w io.Writer, container string) {
		for _, res := range Doctor(container) {
			mark := "ok  "
			if !res.OK {
				mark = "FAIL"
			}
			fmt.Fprintf(w, "%s %s", mark, res.Name)
			if !res.OK && res.Hint != "" {
				fmt.Fprintf(w, " (%s)", res.Hint)
			}
			fmt.Fprintln(w)
		}
	})
}
