package env

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContainerAndVolumeNaming(t *testing.T) {
	cases := []struct {
		agent         string
		wantContainer string
		wantVolume    string
	}{
		{"", "zeroclaw", "zeroclaw-home"},
		{"default", "zeroclaw", "zeroclaw-home"},
		{"work", "zeroclaw-work", "zeroclaw-work-home"},
		{"staging-1", "zeroclaw-staging-1", "zeroclaw-staging-1-home"},
	}

	for _, c := range cases {
		gotC := ContainerName(c.agent)
		if gotC != c.wantContainer {
			t.Errorf("ContainerName(%q) = %q, want %q", c.agent, gotC, c.wantContainer)
		}
		gotV := VolumeName(c.agent)
		if gotV != c.wantVolume {
			t.Errorf("VolumeName(%q) = %q, want %q", c.agent, gotV, c.wantVolume)
		}
	}
}

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

func TestExtractTarPermissionsMasking(t *testing.T) {
	cases := []struct {
		name     string
		typeflag byte
		mode     int64
		wantMode os.FileMode
	}{
		{
			name:     "dir with SUID/SGID/sticky bits",
			typeflag: tar.TypeDir,
			mode:     0o777 | 04000 | 02000 | 01000,
			wantMode: 0o777,
		},
		{
			name:     "regular file with SUID/SGID/sticky bits",
			typeflag: tar.TypeReg,
			mode:     0o644 | 04000 | 02000 | 01000,
			wantMode: 0o644,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			destDir := t.TempDir()
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)

			entryName := "testentry"
			if c.typeflag == tar.TypeDir {
				entryName += "/"
			}

			hdr := &tar.Header{
				Name:     entryName,
				Typeflag: c.typeflag,
				Mode:     c.mode,
				Size:     int64(len("data")),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatalf("WriteHeader failed: %v", err)
			}
			if c.typeflag == tar.TypeReg {
				if _, err := tw.Write([]byte("data")); err != nil {
					t.Fatalf("Write payload failed: %v", err)
				}
			}
			tw.Close()

			if err := extractTar(&buf, destDir); err != nil {
				t.Fatalf("extractTar failed: %v", err)
			}

			targetPath := filepath.Join(destDir, "testentry")
			info, err := os.Stat(targetPath)
			if err != nil {
				t.Fatalf("os.Stat(%q) failed: %v", targetPath, err)
			}

			if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
				t.Errorf("extracted entry mode %v has SUID/SGID/sticky bits, want masked", info.Mode())
			}
		})
	}
}
