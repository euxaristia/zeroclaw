package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	tea "charm.land/bubbletea/v2"

	"zeroclaw/internal/agent"
	"zeroclaw/internal/daemon"
)

// Msg types relayed from daemon NDJSON events into the Bubble Tea update loop.

type runStartMsg struct {
	sessionID string
	provider  string
	model     string
}

type textDeltaMsg struct {
	delta string
}

type reasoningDeltaMsg struct {
	delta string
}

type toolCallMsg struct {
	id   string
	name string
}

type toolResultMsg struct {
	id      string
	summary string
}

type turnErrorMsg struct {
	code    string
	message string
}

type turnDoneMsg struct {
	sessionID string
	status    string
	err       error
}

// eventLine extends agent.Event with the id field that tool_call and
// tool_result events carry but the base struct omits.
type eventLine struct {
	agent.Event
	ID string `json:"id"`
}

// streamClient bounds only connect and header wait time, not the full body
// read: turns stream a live agent run and can legitimately last minutes.
var streamClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

// streamTurn POSTs a turn to zeroclawd and relays each NDJSON event as a
// tea.Msg via send. It blocks until the stream ends and always finishes with
// a turnDoneMsg.
func streamTurn(conversation, prompt string, port int, token string, send func(tea.Msg)) {
	body, err := json.Marshal(daemon.TurnRequest{
		Conversation: conversation,
		Prompt:       prompt,
	})
	if err != nil {
		send(turnDoneMsg{err: err})
		return
	}
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/turn", port),
		bytes.NewReader(body))
	if err != nil {
		send(turnDoneMsg{err: err})
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := streamClient.Do(req)
	if err != nil {
		send(turnDoneMsg{err: err})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		send(turnDoneMsg{err: fmt.Errorf("daemon rejected turn: %s", resp.Status)})
		return
	}

	var gotTrailer bool
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev eventLine
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		switch ev.Type {
		case "zeroclaw_result":
			var t daemon.Trailer
			json.Unmarshal(line, &t)
			gotTrailer = true
			var turnErr error
			if t.Error != "" {
				turnErr = fmt.Errorf("%s", t.Error)
			}
			send(turnDoneMsg{sessionID: t.SessionID, status: t.Status, err: turnErr})
			return
		case "run_start":
			send(runStartMsg{sessionID: ev.SessionID, provider: ev.Provider, model: ev.Model})
		case "reasoning":
			send(reasoningDeltaMsg{delta: ev.Delta})
		case "text":
			send(textDeltaMsg{delta: ev.Delta})
		case "tool_call":
			send(toolCallMsg{id: ev.ID, name: ev.Name})
		case "tool_result":
			send(toolResultMsg{id: ev.ID, summary: ev.Display.Summary})
		case "error":
			send(turnErrorMsg{code: ev.Code, message: ev.Message})
		}
	}
	if err := sc.Err(); err != nil {
		send(turnDoneMsg{err: err})
		return
	}
	if !gotTrailer {
		send(turnDoneMsg{err: fmt.Errorf("connection to zeroclawd ended mid-turn")})
	}
}
