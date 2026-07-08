// Package agent defines the harness-driver boundary. Zeroclaw talks to an
// execution backend only through the Driver interface; everything specific to
// zero (flags, stream-JSON, session semantics) stays inside zerodriver.go.
package agent

import "context"

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
