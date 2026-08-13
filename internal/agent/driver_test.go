package agent

import (
	"testing"
)

func TestNewDriver(t *testing.T) {
	tests := []struct {
		backend   string
		wantZero  bool
		wantCairn bool
		wantErr   bool
	}{
		{"", true, false, false},
		{"zero", true, false, false},
		{"cairn", false, true, false},
		{"cairn-code", false, true, false},
		{"invalid", false, false, true},
	}

	for _, tc := range tests {
		d, err := NewDriver(tc.backend)
		if tc.wantErr {
			if err == nil {
				t.Errorf("NewDriver(%q) returned nil error, want error", tc.backend)
			}
			continue
		}
		if err != nil {
			t.Errorf("NewDriver(%q) returned unexpected error: %v", tc.backend, err)
			continue
		}
		if tc.wantZero {
			if _, ok := d.(ZeroDriver); !ok {
				t.Errorf("NewDriver(%q) = %T, want ZeroDriver", tc.backend, d)
			}
		}
		if tc.wantCairn {
			if _, ok := d.(CairnDriver); !ok {
				t.Errorf("NewDriver(%q) = %T, want CairnDriver", tc.backend, d)
			}
		}
	}
}

func TestDoctor(t *testing.T) {
	results := Doctor("nonexistent-container")
	if len(results) != 2 {
		t.Fatalf("Doctor() returned %d results, want 2", len(results))
	}
	if results[0].OK {
		t.Errorf("ZeroDriver.Doctor() returned OK=true for non-existent container")
	}
	if results[1].OK {
		t.Errorf("CairnDriver.Doctor() returned OK=true for non-existent container")
	}
	if results[0].Hint == "" {
		t.Errorf("ZeroDriver.Doctor() returned empty hint on failure")
	}
	if results[1].Hint == "" {
		t.Errorf("CairnDriver.Doctor() returned empty hint on failure")
	}
}

func TestDriverModelForwarding(t *testing.T) {
	tests := []struct {
		name      string
		opts      TurnOptions
		buildArgs func(TurnOptions) []string
		wantModel string
		wantHas   bool
	}{
		{
			name:      "zero driver model set",
			opts:      TurnOptions{Model: "gpt-4o"},
			buildArgs: buildZeroArgs,
			wantModel: "gpt-4o",
			wantHas:   true,
		},
		{
			name:      "zero driver model unset",
			opts:      TurnOptions{Model: ""},
			buildArgs: buildZeroArgs,
			wantHas:   false,
		},
		{
			name:      "cairn driver model set",
			opts:      TurnOptions{Model: "claude-3-5-sonnet"},
			buildArgs: buildCairnArgs,
			wantModel: "claude-3-5-sonnet",
			wantHas:   true,
		},
		{
			name:      "cairn driver model unset",
			opts:      TurnOptions{Model: ""},
			buildArgs: buildCairnArgs,
			wantHas:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.buildArgs(tc.opts)
			hasModel := false
			var modelVal string
			for i, arg := range args {
				if arg == "--model" && i+1 < len(args) {
					hasModel = true
					modelVal = args[i+1]
					break
				}
			}

			if hasModel != tc.wantHas {
				t.Errorf("hasModel = %v, want %v", hasModel, tc.wantHas)
			}
			if tc.wantHas && modelVal != tc.wantModel {
				t.Errorf("modelVal = %q, want %q", modelVal, tc.wantModel)
			}
		})
	}
}
