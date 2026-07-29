package env

import "testing"

func TestSanitizeContainerPath(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"relative nested", "workspace/notes.md", Home + "/workspace/notes.md", false},
		{"relative traversal", "../../../../etc/passwd", "", true},
		{"absolute outside home", "/etc/shadow", "", true},
		{"absolute home sibling", "/home/zeroclaw-other/file", "", true},
		{"absolute normalized escape", "/home/zeroclaw/../etc/passwd", "", true},
		{"home itself", Home, Home, false},
		{"dot", ".", Home + "/.", false},
		{"relative trailing dot", "workspace/.", Home + "/workspace/.", false},
		{"absolute trailing dot", Home + "/workspace/.", Home + "/workspace/.", false},
		{"absolute nested", Home + "/workspace/notes.md", Home + "/workspace/notes.md", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := sanitizeContainerPath(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("sanitizeContainerPath(%q) = %q, want error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("sanitizeContainerPath(%q) unexpected error: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("sanitizeContainerPath(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
