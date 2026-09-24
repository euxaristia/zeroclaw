package agent

import (
	"testing"
)

func TestValidateSessionID(t *testing.T) {
	for _, tc := range []struct {
		id      string
		wantErr bool
	}{
		{"zero_20260905094815_1788601695006249468_1", false},
		{"abc-123", false},
		{"", true},
		{"../etc/passwd", true},
		{"foo/bar", true},
		{"foo\\bar", true},
		{"..sneaky", true},
	} {
		err := validateSessionID(tc.id)
		if tc.wantErr && err == nil {
			t.Errorf("validateSessionID(%q) = nil, want error", tc.id)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateSessionID(%q) = %v, want nil", tc.id, err)
		}
	}
}

func TestParseSessionEvents(t *testing.T) {
	data := []byte(`{"type":"message","payload":{"content":"hello","role":"user"}}
{"type":"provider_usage","payload":{"promptTokens":100}}
{"type":"tool_call","payload":{"name":"read_file","arguments":"{}"}}
{"type":"tool_result","payload":{"name":"read_file","output":"ok","status":"ok"}}
{"type":"message","payload":{"content":"Hi there!","role":"assistant"}}
{"type":"error","payload":{"message":"auth error: invalid key"}}
`)
	entries := parseSessionEvents(data)
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}

	if entries[0].Role != "user" || entries[0].Content != "hello" {
		t.Errorf("entry 0 = %+v, want user/hello", entries[0])
	}
	if entries[1].Role != "tool_call" || entries[1].Name != "read_file" {
		t.Errorf("entry 1 = %+v, want tool_call/read_file", entries[1])
	}
	if entries[2].Role != "assistant" || entries[2].Content != "Hi there!" {
		t.Errorf("entry 2 = %+v, want assistant/Hi there!", entries[2])
	}
	if entries[3].Role != "error" || entries[3].Content != "auth error: invalid key" {
		t.Errorf("entry 3 = %+v, want error/auth error", entries[3])
	}
}

func TestParseSessionEventsEmpty(t *testing.T) {
	entries := parseSessionEvents(nil)
	if entries != nil {
		t.Errorf("got %v, want nil for empty input", entries)
	}
}

func TestParseSessionEventsSkipsMalformed(t *testing.T) {
	data := []byte(`not json
{"type":"message","payload":{"content":"ok","role":"user"}}
{"type":"message","payload": bad}
`)
	entries := parseSessionEvents(data)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (skip malformed lines)", len(entries))
	}
	if entries[0].Role != "user" {
		t.Errorf("entry 0 role = %q, want user", entries[0].Role)
	}
}
