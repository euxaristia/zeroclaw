package agent

import (
	"testing"
)

func TestNewDriver(t *testing.T) {
	tests := []struct {
		backend  string
		wantZero bool
		wantErr  bool
	}{
		{"", true, false},
		{"zero", true, false},
		{"invalid", false, true},
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
	}
}

func TestDoctor(t *testing.T) {
	results := Doctor("nonexistent-container")
	if len(results) != 1 {
		t.Fatalf("Doctor() returned %d results, want 1", len(results))
	}
	if results[0].OK {
		t.Errorf("ZeroDriver.Doctor() returned OK=true for non-existent container")
	}
	if results[0].Hint == "" {
		t.Errorf("ZeroDriver.Doctor() returned empty hint on failure")
	}
}

func TestDriverModelForwarding(t *testing.T) {
	tests := []struct {
		name      string
		opts      TurnOptions
		wantModel string
		wantHas   bool
	}{
		{
			name:      "model set",
			opts:      TurnOptions{Model: "gpt-4o"},
			wantModel: "gpt-4o",
			wantHas:   true,
		},
		{
			name:    "model unset",
			opts:    TurnOptions{Model: ""},
			wantHas: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := buildZeroArgs(tc.opts)
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
