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
