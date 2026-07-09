package cli

import "os"

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

func accent(s string) string  { return paint("1;38;2;202;255;63", s) } // brand lime, bold
func badge(s string) string   { return paint("1;38;2;0;0;0;48;2;202;255;63", s) }
func boldInk(s string) string { return paint("1", s) } // tool names; terminal's own ink
func muted(s string) string   { return paint("38;2;154;154;162", s) }
func faint(s string) string   { return paint("38;2;124;124;130", s) }
func green(s string) string   { return paint("38;2;93;209;164", s) }
func red(s string) string     { return paint("38;2;255;122;122", s) }
