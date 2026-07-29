package env

import (
	"path/filepath"
	"strings"
	"testing"
)

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

func TestSafeJoin(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "dest")
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"plain file", "a.txt", false},
		{"nested", "sub/a.txt", false},
		{"dot prefixed (tar's own convention)", "./a.txt", false},
		{"traversal", "../escape.txt", true},
		{"nested traversal", "sub/../../escape.txt", true},
		{"posix absolute", "/etc/passwd", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := safeJoin(destDir, c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("safeJoin(%q, %q) = %q, want error", destDir, c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("safeJoin(%q, %q) unexpected error: %v", destDir, c.in, err)
			}
			rel, relErr := filepath.Rel(destDir, got)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Fatalf("safeJoin(%q, %q) = %q, escapes destDir", destDir, c.in, got)
			}
		})
	}
}
