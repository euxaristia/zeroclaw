// Package agent defines the harness-driver boundary. Zeroclaw talks to an
// execution backend only through the Driver interface; everything specific to
// zero (flags, stream-JSON, session semantics) stays inside zerodriver.go.
package agent

import (
	"context"
	"fmt"
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
	Display       struct {
		Kind    string `json:"kind"`
		Summary string `json:"summary"`
	} `json:"display"`
}

type TurnOptions struct {
	// SessionID resumes an existing backend session when set.
	SessionID string
	// NewSessionID asks the backend to create the session under this id, so
	// zeroclaw owns the conversation-to-session mapping.
	NewSessionID string
	Prompt       string
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

type Driver interface {
	Turn(ctx context.Context, opts TurnOptions, onEvent func(Event)) (TurnResult, error)
}

// NewDriver constructs the execution driver for the requested backend.
// Supported backends: "zero" (default when empty), "cairn", or "cairn-code".
func NewDriver(backend string) (Driver, error) {
	switch backend {
	case "", "zero":
		return ZeroDriver{}, nil
	case "cairn", "cairn-code":
		return CairnDriver{}, nil
	default:
		return nil, fmt.Errorf("unknown execution backend: %q (supported: zero, cairn-code)", backend)
	}
}
