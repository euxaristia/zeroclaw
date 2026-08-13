package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type pickerKind int

const (
	pickerModel pickerKind = iota
	pickerProvider
	pickerTheme
	pickerCommand
)

type pickerItem struct {
	Group    string
	Label    string
	Value    string
	Meta     string
	Provider string
}

type commandPicker struct {
	kind     pickerKind
	title    string
	items    []pickerItem
	allItems []pickerItem
	query    string
	selected int
	prev     *commandPicker
}

func (p *commandPicker) move(delta int) {
	n := len(p.items)
	if n == 0 {
		return
	}
	p.selected = ((p.selected+delta)%n + n) % n
}

func (p *commandPicker) current() (pickerItem, bool) {
	if p.selected < 0 || p.selected >= len(p.items) {
		return pickerItem{}, false
	}
	return p.items[p.selected], true
}

func (p *commandPicker) appendQuery(s string) {
	for _, r := range s {
		if r < 32 {
			continue
		}
		p.query += string(r)
	}
	p.applyQuery()
}

func (p *commandPicker) deleteQueryRune() {
	if p.query == "" {
		return
	}
	runes := []rune(p.query)
	p.query = string(runes[:len(runes)-1])
	p.applyQuery()
}

func (p *commandPicker) applyQuery() {
	source := p.allItems
	if len(source) == 0 {
		source = p.items
	}
	q := strings.ToLower(strings.TrimSpace(p.query))
	if q == "" {
		p.items = append([]pickerItem{}, source...)
		p.selected = 0
		return
	}

	var filtered []pickerItem
	for _, item := range source {
		hay := strings.ToLower(item.Label + " " + item.Group + " " + item.Value + " " + item.Meta + " " + item.Provider)
		if strings.Contains(hay, q) {
			filtered = append(filtered, item)
		}
	}
	p.items = filtered
	p.selected = 0
}

func (m model) newModelPicker() *commandPicker {
	var allItems []pickerItem

	// GitLawb OpenGateway
	allItems = append(allItems,
		pickerItem{Group: "GitLawb OpenGateway", Label: "NVIDIA Nemotron 3 Ultra 550B (Free)", Value: "nvidia/nemotron-3-ultra-550b-a55b:free", Meta: "128K ctx · free", Provider: "gitlawb-opengateway"},
		pickerItem{Group: "GitLawb OpenGateway", Label: "Mimo v2.5 Pro", Value: "mimo-v2.5-pro", Meta: "128K ctx · smart-route", Provider: "gitlawb-opengateway"},
		pickerItem{Group: "GitLawb OpenGateway", Label: "Qwen 3 Coder 480B", Value: "qwen3-coder:480b", Meta: "128K ctx · code", Provider: "gitlawb-opengateway"},
	)

	// OpenRouter
	allItems = append(allItems,
		pickerItem{Group: "OpenRouter", Label: "DeepSeek V4 Pro", Value: "deepseek/deepseek-v4-pro", Meta: "1M ctx · tools", Provider: "openrouter"},
		pickerItem{Group: "OpenRouter", Label: "DeepSeek V4 Flash", Value: "deepseek/deepseek-v4-flash-0731", Meta: "1M ctx · fast · tools", Provider: "openrouter"},
		pickerItem{Group: "OpenRouter", Label: "Claude Sonnet 4.5", Value: "anthropic/claude-sonnet-4.5", Meta: "200K ctx · tools · vision", Provider: "openrouter"},
		pickerItem{Group: "OpenRouter", Label: "Claude Haiku 4.5", Value: "anthropic/claude-haiku-4.5", Meta: "200K ctx · fast · tools", Provider: "openrouter"},
		pickerItem{Group: "OpenRouter", Label: "Gemini 2.5 Flash", Value: "google/gemini-2.5-flash", Meta: "1M ctx · fast · vision", Provider: "openrouter"},
		pickerItem{Group: "OpenRouter", Label: "Gemini 2.5 Pro", Value: "google/gemini-2.5-pro", Meta: "1M ctx · reasoning · vision", Provider: "openrouter"},
		pickerItem{Group: "OpenRouter", Label: "GPT-4.1", Value: "openai/gpt-4.1", Meta: "1M ctx · tools · vision", Provider: "openrouter"},
		pickerItem{Group: "OpenRouter", Label: "GPT-4.1 Mini", Value: "openai/gpt-4.1-mini", Meta: "1M ctx · fast · tools", Provider: "openrouter"},
		pickerItem{Group: "OpenRouter", Label: "o3-mini", Value: "openai/o3-mini", Meta: "200K ctx · reasoning", Provider: "openrouter"},
		pickerItem{Group: "OpenRouter", Label: "Llama 3.3 70B Instruct", Value: "meta-llama/llama-3.3-70b-instruct", Meta: "128K ctx · tools", Provider: "openrouter"},
	)

	// Anthropic Direct
	allItems = append(allItems,
		pickerItem{Group: "Anthropic", Label: "Claude Sonnet 4.5", Value: "claude-sonnet-4.5", Meta: "200K ctx · tools · vision", Provider: "anthropic"},
		pickerItem{Group: "Anthropic", Label: "Claude Haiku 4.5", Value: "claude-haiku-4.5", Meta: "200K ctx · fast · tools", Provider: "anthropic"},
		pickerItem{Group: "Anthropic", Label: "Claude Opus 4.1", Value: "claude-opus-4.1", Meta: "200K ctx · reasoning", Provider: "anthropic"},
	)

	// OpenAI Direct
	allItems = append(allItems,
		pickerItem{Group: "OpenAI", Label: "GPT-4.1", Value: "gpt-4.1", Meta: "1M ctx · tools · vision", Provider: "openai"},
		pickerItem{Group: "OpenAI", Label: "GPT-4.1 Mini", Value: "gpt-4.1-mini", Meta: "1M ctx · fast · tools", Provider: "openai"},
		pickerItem{Group: "OpenAI", Label: "GPT-4.1 Nano", Value: "gpt-4.1-nano", Meta: "1M ctx · fast · lightweight", Provider: "openai"},
		pickerItem{Group: "OpenAI", Label: "o3-mini", Value: "o3-mini", Meta: "200K ctx · reasoning", Provider: "openai"},
		pickerItem{Group: "OpenAI", Label: "o1", Value: "o1", Meta: "200K ctx · reasoning", Provider: "openai"},
		pickerItem{Group: "OpenAI", Label: "GPT-4o", Value: "gpt-4o", Meta: "128K ctx · tools · vision", Provider: "openai"},
		pickerItem{Group: "OpenAI", Label: "GPT-4o Mini", Value: "gpt-4o-mini", Meta: "128K ctx · fast · tools", Provider: "openai"},
	)

	// DeepSeek Direct
	allItems = append(allItems,
		pickerItem{Group: "DeepSeek", Label: "DeepSeek V4 Pro", Value: "deepseek-chat", Meta: "1M ctx · tools", Provider: "deepseek"},
		pickerItem{Group: "DeepSeek", Label: "DeepSeek V4 Flash", Value: "deepseek-reasoner", Meta: "1M ctx · fast · reasoning", Provider: "deepseek"},
	)

	// Google Direct
	allItems = append(allItems,
		pickerItem{Group: "Google AI", Label: "Gemini 2.5 Flash", Value: "gemini-2.5-flash", Meta: "1M ctx · fast · vision", Provider: "google"},
		pickerItem{Group: "Google AI", Label: "Gemini 2.5 Pro", Value: "gemini-2.5-pro", Meta: "1M ctx · reasoning · vision", Provider: "google"},
		pickerItem{Group: "Google AI", Label: "Gemini 2.5 Flash-Lite", Value: "gemini-2.5-flash-lite", Meta: "1M ctx · fast · lightweight", Provider: "google"},
	)

	// Ollama Local
	allItems = append(allItems,
		pickerItem{Group: "Ollama (Local)", Label: "Llama 3.3 70B", Value: "llama3.3", Meta: "local", Provider: "ollama"},
		pickerItem{Group: "Ollama (Local)", Label: "Qwen 2.5 Coder 32B", Value: "qwen2.5-coder", Meta: "local · code", Provider: "ollama"},
		pickerItem{Group: "Ollama (Local)", Label: "DeepSeek R1 Distill 32B", Value: "deepseek-r1", Meta: "local · reasoning", Provider: "ollama"},
	)

	// Provider gating: if an active provider is set, check fetchedModels first, or filter allItems
	activeProvider := strings.TrimSpace(m.providerName)
	if activeProvider == "" {
		activeProvider = "gitlawb-opengateway"
	}

	var items []pickerItem
	if fetched, ok := m.fetchedModels[activeProvider]; ok && len(fetched) > 0 {
		items = append([]pickerItem{}, fetched...)
	} else if activeProvider != "" {
		for _, item := range allItems {
			if strings.EqualFold(item.Provider, activeProvider) {
				items = append(items, item)
			}
		}
	}
	if len(items) == 0 {
		items = allItems
	}

	// If current active model isn't in items, prepend it
	if m.modelName != "" {
		found := false
		for _, item := range items {
			if item.Value == m.modelName {
				found = true
				break
			}
		}
		if !found {
			items = append([]pickerItem{{
				Group:    "Active Model",
				Label:    m.modelName,
				Value:    m.modelName,
				Meta:     "currently active",
				Provider: m.providerName,
			}}, items...)
		}
	}

	// Find initial selected index matching active model
	selected := 0
	if m.modelName != "" {
		for idx, item := range items {
			if item.Value == m.modelName {
				selected = idx
				break
			}
		}
	}

	title := "Choose a model"
	if activeProvider != "" {
		title = fmt.Sprintf("Choose a model (%s)", activeProvider)
	}

	return &commandPicker{
		kind:     pickerModel,
		title:    title,
		items:    items,
		allItems: append([]pickerItem{}, items...),
		selected: selected,
	}
}

func (m model) newProviderPicker() *commandPicker {
	items := []pickerItem{
		{Group: "Providers", Label: "GitLawb OpenGateway", Value: "gitlawb-opengateway", Meta: "smart-routing gateway"},
		{Group: "Providers", Label: "OpenRouter", Value: "openrouter", Meta: "multi-provider gateway"},
		{Group: "Providers", Label: "Anthropic", Value: "anthropic", Meta: "Claude models"},
		{Group: "Providers", Label: "OpenAI", Value: "openai", Meta: "GPT & o-series models"},
		{Group: "Providers", Label: "DeepSeek", Value: "deepseek", Meta: "V4 Pro & Flash models"},
		{Group: "Providers", Label: "Google AI", Value: "google", Meta: "Gemini models"},
		{Group: "Providers", Label: "Ollama (Local)", Value: "ollama", Meta: "Local models"},
		{Group: "Providers", Label: "AIMLAPI", Value: "aimlapi", Meta: "AI/ML API gateway"},
		{Group: "Providers", Label: "Custom (OpenAI-compatible)", Value: "custom-openai-compatible", Meta: "Custom endpoint"},
	}

	selected := 0
	if m.providerName != "" {
		for idx, item := range items {
			if strings.EqualFold(item.Value, m.providerName) {
				selected = idx
				break
			}
		}
	}

	return &commandPicker{
		kind:     pickerProvider,
		title:    "Choose a provider",
		items:    items,
		allItems: append([]pickerItem{}, items...),
		selected: selected,
	}
}

func (m model) newThemePicker() *commandPicker {
	var items []pickerItem

	for _, entry := range themeRegistry {
		group := "Dark Themes"
		if !entry.IsDark {
			group = "Light Themes"
		}
		items = append(items, pickerItem{
			Group: group,
			Label: entry.Label,
			Value: entry.Name,
			Meta:  entry.Name,
		})
	}

	return &commandPicker{
		kind:     pickerTheme,
		title:    "Choose a theme",
		items:    items,
		allItems: append([]pickerItem{}, items...),
		selected: 0,
	}
}

func (m model) newCommandPicker() *commandPicker {
	items := []pickerItem{
		{Group: "Commands", Label: "/model", Value: "/model", Meta: "Choose an LLM model"},
		{Group: "Commands", Label: "/provider", Value: "/provider", Meta: "Choose an LLM provider"},
		{Group: "Commands", Label: "/theme", Value: "/theme", Meta: "Choose a UI color theme"},
		{Group: "Commands", Label: "/clear", Value: "/clear", Meta: "Clear chat transcript"},
		{Group: "Commands", Label: "/quit", Value: "/quit", Meta: "Exit zeroclaw chat"},
	}

	return &commandPicker{
		kind:     pickerCommand,
		title:    "Choose a command",
		items:    items,
		allItems: append([]pickerItem{}, items...),
		selected: 0,
	}
}

func (m model) pickerOverlay(width int) string {
	if m.picker == nil {
		return ""
	}

	overlayWidth := 76
	if width < overlayWidth {
		overlayWidth = width
	}
	if overlayWidth < 20 {
		overlayWidth = 20
	}
	innerWidth := overlayWidth - 4

	maxVisible := 10
	totalItems := len(m.picker.items)
	if maxVisible > totalItems {
		maxVisible = totalItems
	}

	start := 0
	if totalItems > maxVisible {
		start = m.picker.selected - maxVisible/2
		if start < 0 {
			start = 0
		}
		if start+maxVisible > totalItems {
			start = totalItems - maxVisible
		}
	}

	visible := m.picker.items[start : start+maxVisible]

	var lines []string

	// Title / filter input
	filterText := m.picker.query
	if filterText == "" {
		filterText = zcTheme.faint.Render("type to filter...")
	} else {
		filterText = zcTheme.ink.Render(filterText)
	}
	lines = append(lines, "  "+zcTheme.accent.Render("\u276f ")+filterText)
	lines = append(lines, zcTheme.line.Render(strings.Repeat("\u2500", innerWidth)))

	lastGroup := ""
	for i, item := range visible {
		absIdx := start + i
		if item.Group != "" && item.Group != lastGroup {
			lines = append(lines, zcTheme.accent.Render(item.Group))
			lastGroup = item.Group
		}

		isSelected := absIdx == m.picker.selected
		marker := "  "
		if isSelected {
			marker = "\u276f "
		}

		left := marker + item.Label
		right := item.Meta
		if item.Provider != "" && right == "" {
			right = item.Provider
		}

		gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 1 {
			gap = 1
		}

		lineStr := left + strings.Repeat(" ", gap) + right

		if isSelected {
			lines = append(lines, zcTheme.selectedRow.Render(fitWidth(lineStr, innerWidth)))
		} else {
			leftPart := zcTheme.ink.Render(left)
			rightPart := zcTheme.faint.Render(right)
			rowContent := leftPart + strings.Repeat(" ", gap) + rightPart
			lines = append(lines, fitWidth(rowContent, innerWidth))
		}
	}

	if len(visible) == 0 {
		lines = append(lines, zcTheme.faint.Render("  no matching items"))
	}

	lines = append(lines, zcTheme.line.Render(strings.Repeat("\u2500", innerWidth)))
	lines = append(lines, zcTheme.faint.Render(fmt.Sprintf(" \u2191/\u2193 move  Enter select  Esc close (%d/%d)", m.picker.selected+1, totalItems)))

	title := strings.TrimSpace(m.picker.title)
	block := styledBlockFillTitle(overlayWidth, title, lines, zcTheme.lineStrong, lipgloss.NewStyle())
	return centerRenderedBlock(block, width)
}

func styledBlockFillTitle(width int, title string, lines []string, borderStyle lipgloss.Style, fill lipgloss.Style) string {
	if width < 4 {
		width = 4
	}
	ruleWidth := width - 2
	titleText := " " + title + " "
	titleWidth := lipgloss.Width(titleText)
	if titleWidth >= ruleWidth {
		titleText = ""
		titleWidth = 0
	}

	leftRule := "\u2500\u2500"
	rightRule := strings.Repeat("\u2500", maxInt(0, ruleWidth-lipgloss.Width(leftRule)-titleWidth))
	top := borderStyle.Render("\u256d"+leftRule) + zcTheme.ink.Bold(true).Render(titleText) + borderStyle.Render(rightRule+"\u256e")
	bottom := borderStyle.Render("\u2570" + strings.Repeat("\u2500", width-2) + "\u256f")

	body := make([]string, 0, len(lines)+2)
	body = append(body, top)
	for _, line := range lines {
		available := width - 4
		fitted := fitWidth(line, available)
		pad := fill.Render(strings.Repeat(" ", maxInt(0, available-lipgloss.Width(fitted))))
		body = append(body, borderStyle.Render("\u2502 ")+fitted+pad+borderStyle.Render(" \u2502"))
	}
	body = append(body, bottom)
	return strings.Join(body, "\n")
}

func centerRenderedBlock(block string, width int) string {
	if block == "" || width <= 0 {
		return block
	}
	lines := strings.Split(block, "\n")
	maxWidth := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > maxWidth {
			maxWidth = w
		}
	}
	pad := (width - maxWidth) / 2
	if pad <= 0 {
		return block
	}
	indent := strings.Repeat(" ", pad)
	for i, l := range lines {
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}

func fitWidth(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		runes := []rune(s)
		if len(runes) > width {
			return string(runes[:width])
		}
	}
	return s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
