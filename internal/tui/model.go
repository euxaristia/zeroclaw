package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	ctrlCExitConfirmDuration = 3 * time.Second
	ctrlCExitConfirmText     = "Press Ctrl+C again to exit"
)

type model struct {
	ctx          context.Context
	conversation string
	port         int
	token        string

	// Transcript and streaming state.
	transcript    []transcriptRow
	streamingText []byte
	streamingReas string

	// Turn metadata from run_start events.
	sessionID    string
	providerName string
	modelName    string

	// Run state.
	pending   bool
	spinner   spinner.Model
	runID     int
	turnStart time.Time

	// Composer state.
	input        string
	inputHistory []string
	historyIdx   int
	historyDraft string

	// Terminal dimensions.
	width  int
	height int

	// Scroll state: offset from bottom (0 = at bottom).
	scrollOffset int

	// Exit confirmation.
	exitConfirmActive bool
	exitConfirmSeq    int

	// Exiting.
	exiting bool

	// Interactive picker overlay (/model, /provider).
	picker *commandPicker

	// send delivers messages from background goroutines. Set by Run before
	// the program starts; unused on the model copy (programSend is the real
	// delivery path for background streams).
	send func(tea.Msg)
}

func newModel(ctx context.Context, opts Options) model {
	s := spinner.New(spinner.WithSpinner(spinner.Dot))
	return model{
		ctx:          ctx,
		conversation: opts.Conversation,
		port:         opts.Port,
		token:        opts.Token,
		transcript: []transcriptRow{{
			kind: rowWelcome,
			text: "zeroclaw. Type /quit to exit.",
		}},
		spinner:    s,
		historyIdx: 0,
	}
}

type exitConfirmExpiredMsg struct{ seq int }

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		if !m.pending {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case runStartMsg:
		m.sessionID = msg.sessionID
		m.providerName = msg.provider
		m.modelName = msg.model
		return m, nil

	case textDeltaMsg:
		m.streamingText = append(m.streamingText, msg.delta...)
		m.scrollOffset = 0
		return m, nil

	case reasoningDeltaMsg:
		m.streamingReas += msg.delta
		return m, nil

	case toolCallMsg:
		m.flushStreaming()
		m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
			kind:  rowToolCall,
			tool:  msg.name,
			id:    msg.id,
			runID: m.runID,
		})
		m.scrollOffset = 0
		return m, nil

	case toolResultMsg:
		m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
			kind:    rowToolResult,
			id:      msg.id,
			summary: msg.summary,
			runID:   m.runID,
		})
		return m, nil

	case turnErrorMsg:
		m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
			kind: rowError,
			text: msg.code + ": " + msg.message,
		})
		return m, nil

	case turnDoneMsg:
		m.flushStreaming()
		m.pending = false
		m.turnStart = time.Time{}
		if msg.err != nil {
			m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
				kind: rowError,
				text: msg.err.Error(),
			})
		}
		m.scrollOffset = 0
		return m, nil

	case exitConfirmExpiredMsg:
		if msg.seq == m.exitConfirmSeq {
			m.exitConfirmActive = false
		}
		return m, nil
	}
	return m, nil
}

func (m *model) flushStreaming() {
	if len(m.streamingText) > 0 {
		m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
			kind:  rowAssistant,
			text:  string(m.streamingText),
			final: true,
		})
		m.streamingText = m.streamingText[:0]
	}
	if m.streamingReas != "" {
		// Reasoning that wasn't followed by text: append as its own row.
		// In practice this is rare; reasoning deltas are flushed before text.
		m.streamingReas = ""
	}
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C: confirm then quit.
	if msg.Key().Code == 'c' && msg.Key().Mod.Contains(tea.ModCtrl) {
		if m.exitConfirmActive {
			m.exiting = true
			return m, tea.Quit
		}
		m.exitConfirmActive = true
		m.exitConfirmSeq++
		seq := m.exitConfirmSeq
		return m, tea.Tick(ctrlCExitConfirmDuration, func(time.Time) tea.Msg {
			return exitConfirmExpiredMsg{seq: seq}
		})
	}
	m.exitConfirmActive = false

	// If a picker overlay is open, delegate input to the picker.
	if m.picker != nil {
		switch {
		case msg.Key().Code == tea.KeyEsc:
			m.picker = nil
			return m, nil
		case msg.Key().Code == tea.KeyUp:
			m.picker.move(-1)
			return m, nil
		case msg.Key().Code == tea.KeyDown:
			m.picker.move(1)
			return m, nil
		case msg.Key().Code == tea.KeyEnter:
			item, ok := m.picker.current()
			kind := m.picker.kind
			m.picker = nil
			if ok {
				if kind == pickerModel {
					m.modelName = item.Value
					if item.Provider != "" {
						m.providerName = item.Provider
					}
					m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
						kind: rowSystem,
						text: "Switched model to " + item.Value,
					})
				} else if kind == pickerProvider {
					m.providerName = item.Value
					m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
						kind: rowSystem,
						text: "Switched provider to " + item.Value,
					})
					m.picker = m.newModelPicker()
				} else if kind == pickerTheme {
					if entry, ok := lookupTheme(item.Value); ok {
						zcTheme = buildTheme(entry.Palette)
						m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
							kind: rowSystem,
							text: "Switched theme to " + entry.Name,
						})
					}
				}
			}
			return m, nil
		case msg.Key().Code == tea.KeyBackspace:
			m.picker.deleteQueryRune()
			return m, nil
		default:
			if msg.Key().Text != "" && !msg.Key().Mod.Contains(tea.ModCtrl) && !msg.Key().Mod.Contains(tea.ModAlt) {
				m.picker.appendQuery(msg.Key().Text)
			}
			return m, nil
		}
	}

	// While a turn is running, eat input (except Ctrl+C above and scroll).
	if m.pending {
		return m.handleScroll(msg)
	}

	switch {
	case msg.Key().Code == tea.KeyEnter:
		return m.handleSubmit()
	case msg.Key().Code == tea.KeyUp:
		return m.handleHistoryUp()
	case msg.Key().Code == tea.KeyDown:
		return m.handleHistoryDown()
	case msg.Key().Code == tea.KeyBackspace:
		if len(m.input) > 0 {
			runes := []rune(m.input)
			m.input = string(runes[:len(runes)-1])
		}
		return m, nil
	case msg.Key().Code == 'u' && msg.Key().Mod.Contains(tea.ModCtrl):
		m.input = ""
		return m, nil
	case msg.Key().Code == 'w' && msg.Key().Mod.Contains(tea.ModCtrl):
		m.input = deleteWordBefore(m.input)
		return m, nil
	default:
		return m.handleScroll(msg)
	}
}

func (m model) handleScroll(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Key().Code == tea.KeyPgUp:
		m.scrollOffset += m.height / 2
		return m, nil
	case msg.Key().Code == tea.KeyPgDown:
		m.scrollOffset -= m.height / 2
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		return m, nil
	default:
		// Regular character input when not pending.
		if !m.pending && msg.Key().Text != "" {
			m.input += msg.Key().Text
		}
		return m, nil
	}
}

func (m model) handleSubmit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input)
	m.input = ""
	if text == "" {
		return m, nil
	}

	// Slash commands.
	switch {
	case text == "/quit" || text == "/exit":
		return m, tea.Quit
	case text == "/clear":
		m.transcript = []transcriptRow{{kind: rowWelcome, text: "zeroclaw. Type /quit to exit."}}
		m.scrollOffset = 0
		return m, nil
	case text == "/model":
		m.picker = m.newModelPicker()
		return m, nil
	case strings.HasPrefix(text, "/model "):
		arg := strings.TrimSpace(text[7:])
		if arg != "" {
			m.modelName = arg
			m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
				kind: rowSystem,
				text: "Switched model to " + arg,
			})
		}
		return m, nil
	case text == "/provider":
		m.picker = m.newProviderPicker()
		return m, nil
	case strings.HasPrefix(text, "/provider "):
		arg := strings.TrimSpace(text[10:])
		if arg != "" {
			m.providerName = arg
			m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
				kind: rowSystem,
				text: "Switched provider to " + arg,
			})
		}
		return m, nil
	case text == "/theme":
		m.picker = m.newThemePicker()
		return m, nil
	case strings.HasPrefix(text, "/theme "):
		arg := strings.TrimSpace(text[7:])
		if arg != "" {
			if entry, ok := lookupTheme(arg); ok {
				zcTheme = buildTheme(entry.Palette)
				m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
					kind: rowSystem,
					text: "Switched theme to " + entry.Name,
				})
			} else {
				m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
					kind: rowError,
					text: "Unknown theme: " + arg,
				})
			}
		}
		return m, nil
	}

	// Record input history.
	m.inputHistory = append(m.inputHistory, text)
	m.historyIdx = len(m.inputHistory)
	m.historyDraft = ""

	// Append user row.
	m.transcript = appendTranscriptRow(m.transcript, transcriptRow{
		kind: rowUser,
		text: text,
	})

	// Start the turn.
	m.pending = true
	m.runID++
	m.turnStart = time.Now()
	m.scrollOffset = 0

	conv := m.conversation
	port := m.port
	token := m.token
	providerName := m.providerName
	modelName := m.modelName
	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			send := programSend
			if send == nil {
				return turnDoneMsg{err: fmt.Errorf("tui not initialized")}
			}
			streamTurn(conv, text, providerName, modelName, port, token, send)
			return nil
		},
	)
}

func (m model) handleHistoryUp() (tea.Model, tea.Cmd) {
	if len(m.inputHistory) == 0 {
		return m, nil
	}
	if m.historyIdx == len(m.inputHistory) {
		m.historyDraft = m.input
	}
	if m.historyIdx > 0 {
		m.historyIdx--
		m.input = m.inputHistory[m.historyIdx]
	}
	return m, nil
}

func (m model) handleHistoryDown() (tea.Model, tea.Cmd) {
	if m.historyIdx >= len(m.inputHistory) {
		return m, nil
	}
	m.historyIdx++
	if m.historyIdx == len(m.inputHistory) {
		m.input = m.historyDraft
	} else {
		m.input = m.inputHistory[m.historyIdx]
	}
	return m, nil
}

// View renders the full TUI frame.
func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("")
	}

	title := m.titleBar()
	status := m.statusLine()
	divider := m.composerDivider()
	inputLine := m.composerView()

	// Fixed chrome: title(2) + divider(1) + input(1) + status(1) = 5 lines.
	chrome := 5
	bodyHeight := m.height - chrome
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	body := m.transcriptBody(bodyHeight)

	if m.picker != nil {
		overlay := m.pickerOverlay(m.width)
		oLines := strings.Split(overlay, "\n")
		padTop := (bodyHeight - len(oLines)) / 2
		if padTop < 0 {
			padTop = 0
		}
		var centered []string
		for i := 0; i < padTop; i++ {
			centered = append(centered, "")
		}
		centered = append(centered, oLines...)
		for len(centered) < bodyHeight {
			centered = append(centered, "")
		}
		body = strings.Join(centered, "\n")
	}

	content := title + "\n" + body + "\n" + divider + "\n" + inputLine + "\n" + status
	v := tea.NewView(content)
	v.AltScreen = true
	v.BackgroundColor = zcTheme.bgPanel
	return v
}

func (m model) titleBar() string {
	left := zcTheme.badge.Render(" zeroclaw ")
	convSegment := zcTheme.faint.Render("  " + m.conversation)

	right := ""
	if m.modelName != "" {
		provider := m.providerName
		if provider != "" {
			right = zcTheme.ink.Render(provider + "/" + m.modelName)
		} else {
			right = zcTheme.ink.Render(m.modelName)
		}
	}

	header := left + convSegment
	if right != "" {
		gap := m.width - lipgloss.Width(header) - lipgloss.Width(right)
		if gap > 0 {
			header += strings.Repeat(" ", gap) + right
		}
	}

	rule := zcTheme.line.Render(strings.Repeat("\u2500", m.width))
	return header + "\n" + rule
}

func (m model) composerDivider() string {
	if m.width < 4 {
		return zcTheme.lineStrong.Render(strings.Repeat("\u2500", m.width))
	}
	model := ""
	if m.modelName != "" {
		model = m.modelName
	}
	meta := zcTheme.muted.Render(model)
	metaW := lipgloss.Width(meta)
	if m.width < metaW+6 || model == "" {
		return zcTheme.lineStrong.Render("\u2570" + strings.Repeat("\u2500", m.width-2) + "\u256f")
	}
	ruleW := m.width - metaW - 4
	return zcTheme.lineStrong.Render("\u2570"+strings.Repeat("\u2500", ruleW)+" ") + meta + zcTheme.lineStrong.Render(" \u256f")
}

func (m model) composerView() string {
	prompt := zcTheme.userPrompt.Render("\u276f ")
	if m.pending {
		prompt = m.spinner.View() + " "
	}
	return prompt + m.input
}

func (m model) statusLine() string {
	left := "  " + zcTheme.accent.Render("\u25cf") + " "
	if m.exitConfirmActive {
		left += zcTheme.amber.Render(ctrlCExitConfirmText)
	} else if m.pending && !m.turnStart.IsZero() {
		elapsed := time.Since(m.turnStart).Truncate(time.Second)
		left += zcTheme.muted.Render("working " + elapsed.String())
	} else {
		left += zcTheme.green.Render("ready")
	}

	right := ""
	if m.sessionID != "" {
		right = zcTheme.faint.Render("session " + truncateID(m.sessionID))
	}

	if right != "" {
		gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
		if gap > 0 {
			return left + strings.Repeat(" ", gap) + right
		}
	}
	return left
}

func (m model) transcriptBody(height int) string {
	var lines []string

	// Render settled transcript rows.
	for _, row := range m.transcript {
		rendered := renderRow(row, m.width)
		if rendered == "" {
			continue
		}
		rowLines := strings.Split(rendered, "\n")
		lines = append(lines, rowLines...)
	}

	// Render in-progress reasoning.
	if m.streamingReas != "" {
		rLines := strings.Split(zcTheme.muted.Render(m.streamingReas), "\n")
		lines = append(lines, rLines...)
	}

	// Render in-progress assistant text.
	if len(m.streamingText) > 0 {
		tLines := strings.Split(string(m.streamingText), "\n")
		lines = append(lines, tLines...)
	}

	// Render working indicator when pending with no streaming text yet.
	if m.pending && len(m.streamingText) == 0 && m.streamingReas == "" {
		lines = append(lines, zcTheme.muted.Render("  thinking..."))
	}

	// Apply scroll offset and pad/trim to height.
	total := len(lines)
	end := total - m.scrollOffset
	if end < 0 {
		end = 0
	}
	if end > total {
		end = total
	}
	start := end - height
	if start < 0 {
		start = 0
	}
	// Clamp scrollOffset so we don't scroll past the top.
	if m.scrollOffset > total-height && total > height {
		m.scrollOffset = total - height
	}

	visible := lines[start:end]

	// Pad to fill height.
	for len(visible) < height {
		visible = append([]string{""}, visible...)
	}

	return strings.Join(visible, "\n")
}

func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func deleteWordBefore(s string) string {
	runes := []rune(s)
	i := len(runes)
	for i > 0 && runes[i-1] == ' ' {
		i--
	}
	for i > 0 && runes[i-1] != ' ' {
		i--
	}
	return string(runes[:i])
}
