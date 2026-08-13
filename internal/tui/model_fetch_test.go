package tui

import (
	"strings"
	"testing"
)

func TestParseLiveModelsResponseOpenGateway(t *testing.T) {
	jsonPayload := `{
		"data": [
			{
				"id": "nvidia/nemotron-3-ultra-550b-a55b:free",
				"name": "Nemotron 3 Ultra (free)",
				"description": "frontier reasoning MoE · 1M context · free",
				"context_window": 131072,
				"pricing": {"prompt": "0", "completion": "0"}
			},
			{
				"id": "google/gemini-3.1-flash-lite",
				"name": "Gemini 3.1 Flash Lite",
				"context_window": 1048576,
				"pricing": {"prompt": "0.00000025", "completion": "0.0000015"}
			}
		]
	}`

	items, err := parseLiveModelsResponse([]byte(jsonPayload), "gitlawb-opengateway")
	if err != nil {
		t.Fatalf("unexpected error parsing live models: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].Label != "Nemotron 3 Ultra (free)" || !strings.Contains(items[0].Meta, "free") || !strings.Contains(items[0].Meta, "131K ctx") {
		t.Errorf("unexpected item[0]: %+v", items[0])
	}
	if items[1].Label != "Gemini 3.1 Flash Lite" || items[1].Meta != "1.0M ctx" {
		t.Errorf("unexpected item[1]: %+v", items[1])
	}
}

func TestFormatContextWindow(t *testing.T) {
	tests := []struct {
		ctx  int
		want string
	}{
		{131072, "131K ctx"},
		{262144, "262K ctx"},
		{1048576, "1.0M ctx"},
		{2000000, "2M ctx"},
		{0, ""},
	}

	for _, tt := range tests {
		got := formatContextWindow(tt.ctx)
		if got != tt.want {
			t.Errorf("formatContextWindow(%d) = %q, want %q", tt.ctx, got, tt.want)
		}
	}
}
