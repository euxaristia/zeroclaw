package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type palette struct {
	panel    string
	promptBg string
	line     string
	line2    string
	ink      string
	muted    string
	faint    string
	faintest string
	accent   string
	green    string
	red      string
	amber    string
	blue     string
	onAccent string
	cardRun  string
	cardErr  string
}

type tuiTheme struct {
	ink             lipgloss.Style
	muted           lipgloss.Style
	faint           lipgloss.Style
	faintest        lipgloss.Style
	accent          lipgloss.Style
	green           lipgloss.Style
	red             lipgloss.Style
	amber           lipgloss.Style
	blue            lipgloss.Style
	line            lipgloss.Style
	lineStrong      lipgloss.Style
	badge           lipgloss.Style
	selectedRow     lipgloss.Style
	userPrompt      lipgloss.Style
	toolName        lipgloss.Style
	toolTarget      lipgloss.Style
	toolArg         lipgloss.Style
	cardRun         lipgloss.Style
	cardErr         lipgloss.Style
	bashPrompt      lipgloss.Style
	panel           lipgloss.Style
	userPromptPanel lipgloss.Style

	accentColor color.Color
	inkColor    color.Color
	bgPanel     color.Color
}

func buildTheme(p palette) tuiTheme {
	return tuiTheme{
		bgPanel:  lipgloss.Color(p.panel),
		ink:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.ink)),
		muted:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.muted)),
		faint:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.faint)),
		faintest: lipgloss.NewStyle().Foreground(lipgloss.Color(p.faintest)),
		accent:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.accent)),
		green:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.green)),
		red:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.red)),
		amber:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.amber)),
		blue:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.blue)),

		line:       lipgloss.NewStyle().Foreground(lipgloss.Color(p.line)),
		lineStrong: lipgloss.NewStyle().Foreground(lipgloss.Color(p.line2)),

		badge: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.onAccent)).
			Background(lipgloss.Color(p.accent)).
			Bold(true).
			Padding(0, 1),

		selectedRow: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.onAccent)).
			Background(lipgloss.Color(p.accent)).
			Bold(true),

		userPrompt: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.ink)).
			Bold(true),

		toolName: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.accent)).
			Bold(true),

		toolTarget: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.muted)),

		toolArg: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.faint)),

		cardRun: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.ink)).
			Background(lipgloss.Color(p.cardRun)),

		cardErr: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.ink)).
			Background(lipgloss.Color(p.cardErr)),

		bashPrompt: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.accent)).
			Bold(true),

		panel: lipgloss.NewStyle().
			Background(lipgloss.Color(p.panel)),

		userPromptPanel: lipgloss.NewStyle().
			Background(lipgloss.Color(p.promptBg)),

		accentColor: lipgloss.Color(p.accent),
		inkColor:    lipgloss.Color(p.ink),
	}
}

var zcTheme = buildTheme(darkPalette)
