package tui

import (
	"context"
	"testing"
)

func TestNewModel(t *testing.T) {
	opts := Options{
		Conversation: "test-conv",
		Port:         1234,
		Token:        "secret",
	}
	m := newModel(context.Background(), opts)
	if m.conversation != "test-conv" {
		t.Errorf("expected conversation test-conv, got %s", m.conversation)
	}
	if m.port != 1234 {
		t.Errorf("expected port 1234, got %d", m.port)
	}
	if len(m.transcript) != 1 || m.transcript[0].kind != rowWelcome {
		t.Errorf("expected welcome row in initial transcript")
	}
}

func TestModelEvents(t *testing.T) {
	m := newModel(context.Background(), Options{Conversation: "main"})

	// runStart
	m2, _ := m.Update(runStartMsg{
		sessionID: "sess-123",
		provider:  "openrouter",
		model:     "gpt-4o",
	})
	m = m2.(model)
	if m.sessionID != "sess-123" || m.providerName != "openrouter" || m.modelName != "gpt-4o" {
		t.Fatalf("unexpected metadata after runStartMsg: %+v", m)
	}

	// textDelta
	m2, _ = m.Update(textDeltaMsg{delta: "Hello"})
	m = m2.(model)
	if string(m.streamingText) != "Hello" {
		t.Fatalf("expected streamingText Hello, got %s", string(m.streamingText))
	}

	// toolCall
	m2, _ = m.Update(toolCallMsg{id: "call-1", name: "read_file"})
	m = m2.(model)
	// toolCall flushes streaming text into transcript, so rows are: welcome, assistant text, tool call
	if len(m.transcript) != 3 {
		t.Fatalf("expected 3 transcript rows, got %d", len(m.transcript))
	}
	if m.transcript[1].kind != rowAssistant || m.transcript[1].text != "Hello" {
		t.Fatalf("expected assistant text row 'Hello', got %+v", m.transcript[1])
	}
	if m.transcript[2].kind != rowToolCall || m.transcript[2].tool != "read_file" {
		t.Fatalf("expected toolCall row 'read_file', got %+v", m.transcript[2])
	}

	// turnDone
	m2, _ = m.Update(turnDoneMsg{sessionID: "sess-123", status: "OK"})
	m = m2.(model)
	if m.pending {
		t.Fatalf("expected pending false after turnDone")
	}
}

func TestRenderRow(t *testing.T) {
	welcome := renderRow(transcriptRow{kind: rowWelcome, text: "Welcome"}, 80)
	if welcome == "" {
		t.Error("expected non-empty welcome rendering")
	}

	user := renderRow(transcriptRow{kind: rowUser, text: "hello"}, 80)
	if user == "" {
		t.Error("expected non-empty user rendering")
	}

	tool := renderRow(transcriptRow{kind: rowToolCall, tool: "bash"}, 80)
	if tool == "" {
		t.Error("expected non-empty tool call rendering")
	}

	errRow := renderRow(transcriptRow{kind: rowError, text: "fail"}, 80)
	if errRow == "" {
		t.Error("expected non-empty error rendering")
	}
}

func TestHistoryNavigation(t *testing.T) {
	m := newModel(context.Background(), Options{})
	m.inputHistory = []string{"cmd1", "cmd2"}
	m.historyIdx = 2
	m.input = "current"

	m2, _ := m.handleHistoryUp()
	m = m2.(model)
	if m.input != "cmd2" {
		t.Errorf("expected cmd2 after Up, got %s", m.input)
	}

	m2, _ = m.handleHistoryUp()
	m = m2.(model)
	if m.input != "cmd1" {
		t.Errorf("expected cmd1 after second Up, got %s", m.input)
	}

	m2, _ = m.handleHistoryDown()
	m = m2.(model)
	if m.input != "cmd2" {
		t.Errorf("expected cmd2 after Down, got %s", m.input)
	}

	m2, _ = m.handleHistoryDown()
	m = m2.(model)
	if m.input != "current" {
		t.Errorf("expected draft 'current' after final Down, got %s", m.input)
	}
}
