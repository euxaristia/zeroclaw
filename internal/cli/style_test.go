package cli

import "testing"

func TestFormatMarkdown(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bold markdown",
			in:   "I am **Zeroclaw** agent",
			want: "I am \x1b[1mZeroclaw\x1b[0m agent",
		},
		{
			name: "italic markdown",
			in:   "One sentence: *I am a coding agent*",
			want: "One sentence: \x1b[3mI am a coding agent\x1b[0m",
		},
		{
			name: "bold italic markdown",
			in:   "***Important notice***",
			want: "\x1b[1;3mImportant notice\x1b[0m",
		},
		{
			name: "inline code markdown",
			in:   "path is `~/memory/`",
			want: "path is \x1b[38;2;202;255;63m~/memory/\x1b[0m",
		},
		{
			name: "plain text unchanged",
			in:   "Hello world",
			want: "Hello world",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatMarkdown(tc.in)
			if got != tc.want {
				t.Errorf("FormatMarkdown(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripMarkdown(t *testing.T) {
	in := "I am **Zeroclaw** in `~/memory/` *mode*"
	want := "I am Zeroclaw in ~/memory/ mode"
	got := stripMarkdown(in)
	if got != want {
		t.Errorf("stripMarkdown(%q) = %q, want %q", in, got, want)
	}
}
