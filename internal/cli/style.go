package cli

import (
	"os"
	"strings"
)

// ANSI painting for chat and exec output. The hex values are zero's dark-theme
// palette (internal/tui/theme_palettes.go in the zero repo) projected onto
// truecolor escapes, so a zeroclaw transcript reads like a zero one without
// pulling in a TUI stack. Terminals downsample truecolor themselves; NO_COLOR
// disables everything.
var noColor = os.Getenv("NO_COLOR") != ""

func paint(code, s string) string {
	if noColor || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func accent(s string) string        { return paint("1;38;2;202;255;63", s) } // brand lime, bold
func badge(s string) string         { return paint("1;38;2;0;0;0;48;2;202;255;63", s) }
func boldInk(s string) string       { return paint("1", s) } // tool names; terminal's own ink
func italicInk(s string) string     { return paint("3", s) }
func boldItalicInk(s string) string { return paint("1;3", s) }
func codeInk(s string) string       { return paint("38;2;202;255;63", s) }
func muted(s string) string         { return paint("38;2;154;154;162", s) }
func faint(s string) string         { return paint("38;2;124;124;130", s) }
func green(s string) string         { return paint("38;2;93;209;164", s) }
func red(s string) string           { return paint("38;2;255;122;122", s) }

func stripMarkdown(s string) string {
	s = strings.ReplaceAll(s, "***", "")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

// FormatMarkdown converts inline markdown formatting (**bold**, *italic*, ***bold italic***, `code`)
// into ANSI truecolor escape sequences using zero's palette styling.
func FormatMarkdown(s string) string {
	if s == "" {
		return s
	}
	if noColor {
		return stripMarkdown(s)
	}

	var sb strings.Builder
	sb.Grow(len(s))
	i := 0
	n := len(s)

	for i < n {
		if s[i] == '`' {
			end := strings.IndexByte(s[i+1:], '`')
			if end != -1 {
				codeContent := s[i+1 : i+1+end]
				sb.WriteString(codeInk(codeContent))
				i += end + 2
				continue
			}
		}

		if strings.HasPrefix(s[i:], "***") {
			end := strings.Index(s[i+3:], "***")
			if end != -1 {
				content := s[i+3 : i+3+end]
				sb.WriteString(boldItalicInk(content))
				i += end + 6
				continue
			}
		}

		if strings.HasPrefix(s[i:], "**") {
			end := strings.Index(s[i+2:], "**")
			if end != -1 {
				content := s[i+2 : i+2+end]
				sb.WriteString(boldInk(content))
				i += end + 4
				continue
			}
		}
		if strings.HasPrefix(s[i:], "__") {
			end := strings.Index(s[i+2:], "__")
			if end != -1 {
				content := s[i+2 : i+2+end]
				sb.WriteString(boldInk(content))
				i += end + 4
				continue
			}
		}

		if s[i] == '*' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\n' || s[i-1] == '(' || s[i-1] == '>') {
			end := strings.IndexByte(s[i+1:], '*')
			if end > 0 && !strings.HasPrefix(s[i+1:], "*") {
				content := s[i+1 : i+1+end]
				sb.WriteString(italicInk(content))
				i += end + 2
				continue
			}
		}
		if s[i] == '_' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\n' || s[i-1] == '(') {
			end := strings.IndexByte(s[i+1:], '_')
			if end > 0 && !strings.HasPrefix(s[i+1:], "_") {
				content := s[i+1 : i+1+end]
				sb.WriteString(italicInk(content))
				i += end + 2
				continue
			}
		}

		sb.WriteByte(s[i])
		i++
	}

	return sb.String()
}
