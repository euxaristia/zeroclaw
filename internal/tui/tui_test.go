package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

func TestPickersAndModelSwitching(t *testing.T) {
	m := newModel(context.Background(), Options{})

	// Direct /model command
	m.input = "/model claude-3-5-sonnet-20241022"
	m2, _ := m.handleSubmit()
	m = m2.(model)
	if m.modelName != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected modelName claude-3-5-sonnet-20241022, got %s", m.modelName)
	}

	// Direct /provider command
	m.input = "/provider openrouter"
	m2, _ = m.handleSubmit()
	m = m2.(model)
	if m.providerName != "openrouter" {
		t.Errorf("expected providerName openrouter, got %s", m.providerName)
	}

	// Open /model picker
	m.input = "/model"
	m2, _ = m.handleSubmit()
	m = m2.(model)
	if m.picker == nil || m.picker.kind != pickerModel {
		t.Fatalf("expected model picker to open")
	}

	// Filter query in picker
	m.picker.appendQuery("gpt-4o")
	if len(m.picker.items) == 0 {
		t.Fatalf("expected filtered items for query gpt-4o")
	}

	// Move and select
	item, ok := m.picker.current()
	if !ok {
		t.Fatalf("expected valid item in picker")
	}

	// Render overlay
	m.width = 120
	m.height = 24
	view := m.View()
	if view.Content == "" {
		t.Errorf("expected non-empty view with picker overlay")
	}
	// Check that top border of modal starts with leading spaces (horizontally centered)
	foundCenteredBorder := false
	for _, l := range strings.Split(view.Content, "\n") {
		if strings.Contains(l, "\u256d\u2500") && strings.HasPrefix(l, " ") {
			foundCenteredBorder = true
			break
		}
	}
	if !foundCenteredBorder {
		t.Errorf("expected modal top border to be horizontally centered with leading spaces")
	}

	// Select item via Enter
	m2, _ = m.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = m2.(model)
	if m.picker != nil {
		t.Errorf("expected picker to close after Enter")
	}
	if m.modelName != item.Value {
		t.Errorf("expected modelName %s, got %s", item.Value, m.modelName)
	}
}

func TestThemePickerAndSwitching(t *testing.T) {
	m := newModel(context.Background(), Options{Conversation: "conv-test"})

	// Direct slash command /theme dracula
	m.input = "/theme dracula"
	m2, _ := m.handleSubmit()
	m = m2.(model)
	if len(m.transcript) == 0 || !strings.Contains(m.transcript[len(m.transcript)-1].text, "dracula") {
		t.Errorf("expected system message about switching theme to dracula, got %#v", m.transcript)
	}

	// Interactive theme picker via /theme
	m.input = "/theme"
	m2, _ = m.handleSubmit()
	m = m2.(model)
	if m.picker == nil || m.picker.kind != pickerTheme {
		t.Fatalf("expected theme picker to open")
	}
	if len(m.picker.items) != len(themeRegistry) {
		t.Errorf("expected %d theme items, got %d", len(themeRegistry), len(m.picker.items))
	}

	// Select second theme item (dracula) via Enter
	m.picker.selected = 1
	item := m.picker.items[1]
	m2, _ = m.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = m2.(model)
	if m.picker != nil {
		t.Errorf("expected theme picker to close after Enter")
	}
	if len(m.transcript) == 0 || !strings.Contains(m.transcript[len(m.transcript)-1].text, item.Value) {
		t.Errorf("expected system message for theme %s", item.Value)
	}
}

func TestSlashCommandPickerTrigger(t *testing.T) {
	m := newModel(context.Background(), Options{Conversation: "conv-test"})

	// Typing '/' into empty prompt opens command picker
	m2, _ := m.handleKey(tea.KeyPressMsg(tea.Key{Text: "/"}))
	m = m2.(model)
	if m.picker == nil || m.picker.kind != pickerCommand {
		t.Fatalf("expected command picker to open when typing /")
	}

	// Typing 'm' filters items to /model
	m2, _ = m.handleKey(tea.KeyPressMsg(tea.Key{Text: "m"}))
	m = m2.(model)
	if m.picker.query != "m" {
		t.Errorf("expected query 'm', got '%s'", m.picker.query)
	}

	// Hitting Enter on /model opens model picker
	m2, _ = m.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = m2.(model)
	if m.picker == nil || m.picker.kind != pickerModel {
		t.Fatalf("expected model picker to open after selecting /model")
	}
}

func TestProviderGatedModelPicker(t *testing.T) {
	m := newModel(context.Background(), Options{Conversation: "conv-test"})
	m.providerName = "gitlawb-opengateway"

	picker := m.newModelPicker()
	if !strings.Contains(picker.title, "gitlawb-opengateway") {
		t.Errorf("expected title to contain gitlawb-opengateway, got %s", picker.title)
	}
	for _, item := range picker.items {
		if item.Provider != "gitlawb-opengateway" && item.Group != "Active Model" {
			t.Errorf("unexpected model item from provider %s in gitlawb-opengateway gated picker", item.Provider)
		}
	}
}

func TestPickerBackStackEsc(t *testing.T) {
	m := newModel(context.Background(), Options{Conversation: "conv-test"})

	// 1. Open /provider
	m.input = "/provider"
	m2, _ := m.handleSubmit()
	m = m2.(model)
	if m.picker == nil || m.picker.kind != pickerProvider {
		t.Fatalf("expected provider picker to open")
	}

	// 2. Select first item (gitlawb-opengateway) -> opens model picker with prev set to provider picker
	m2, _ = m.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = m2.(model)
	if m.picker == nil || m.picker.kind != pickerModel {
		t.Fatalf("expected model picker to open")
	}
	if m.picker.prev == nil || m.picker.prev.kind != pickerProvider {
		t.Fatalf("expected picker.prev to point to provider picker")
	}

	// 3. Press Esc -> should return to provider picker instead of closing
	m2, _ = m.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = m2.(model)
	if m.picker == nil || m.picker.kind != pickerProvider {
		t.Fatalf("expected Esc to return to provider picker, got %+v", m.picker)
	}

	// 4. Press Esc again -> should close picker since prev was nil
	m2, _ = m.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = m2.(model)
	if m.picker != nil {
		t.Errorf("expected Esc on top-level picker to close overlay")
	}
}
