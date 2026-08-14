package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"zeroclaw/internal/catalog"
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
	allItems := fromCatalogItems(catalog.StaticModels(""))

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
	items := fromCatalogItems(catalog.Providers())

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
		{Group: "Commands", Label: "/help", Value: "/help", Meta: "Show available commands"},
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
