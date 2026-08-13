package tui

type rowKind int

const (
	rowWelcome rowKind = iota
	rowUser
	rowAssistant
	rowReasoning
	rowToolCall
	rowToolResult
	rowSystem
	rowError
)

type transcriptRow struct {
	kind    rowKind
	text    string
	tool    string // tool name for call/result rows
	summary string // result summary for tool_result
	runID   int
	id      string // tool call id
	final   bool   // marks final assistant answer
}

func renderRow(row transcriptRow, width int) string {
	switch row.kind {
	case rowWelcome:
		return zcTheme.faint.Render(row.text)
	case rowUser:
		return zcTheme.userPrompt.Render(">") + " " + row.text
	case rowAssistant:
		return row.text
	case rowReasoning:
		return "  " + zcTheme.muted.Render(row.text)
	case rowToolCall:
		return zcTheme.accent.Render("\u23fa") + " " + zcTheme.toolName.Render(row.tool)
	case rowToolResult:
		if row.summary != "" {
			return "  " + zcTheme.faint.Render(row.summary)
		}
		return ""
	case rowSystem:
		return zcTheme.faint.Render(row.text)
	case rowError:
		return zcTheme.red.Render("\u2717 " + row.text)
	default:
		return row.text
	}
}

func appendTranscriptRow(rows []transcriptRow, row transcriptRow) []transcriptRow {
	return append(rows, row)
}
