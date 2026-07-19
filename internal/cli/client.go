package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"zeroclaw/internal/agent"
	"zeroclaw/internal/daemon"
)

// turnStream sends one turn to zeroclawd and streams driver events back.
// The CLI never executes agent logic in-process; if the daemon is down, the
// only remedy offered is `zeroclaw up`.
func turnStream(conversation, prompt string, onEvent func(agent.Event)) (daemon.Trailer, error) {
	info, ok := daemon.Running()
	if !ok {
		return daemon.Trailer{}, fmt.Errorf("zeroclawd is not running; run `zeroclaw up`")
	}
	body, err := json.Marshal(daemon.TurnRequest{Conversation: conversation, Prompt: prompt})
	if err != nil {
		return daemon.Trailer{}, err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/turn", info.Port), bytes.NewReader(body))
	if err != nil {
		return daemon.Trailer{}, err
	}
	req.Header.Set("Authorization", "Bearer "+info.Token)
	req.Header.Set("Content-Type", "application/json")

	// The turn stream can take several minutes to generate a response for complex tasks.
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return daemon.Trailer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return daemon.Trailer{}, fmt.Errorf("daemon rejected turn: %s", resp.Status)
	}

	var trailer daemon.Trailer
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev agent.Event
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		if ev.Type == "zeroclaw_result" {
			json.Unmarshal(line, &trailer)
			continue
		}
		if onEvent != nil {
			onEvent(ev)
		}
	}
	if err := sc.Err(); err != nil {
		return trailer, err
	}
	if trailer.Type == "" {
		return trailer, fmt.Errorf("connection to zeroclawd ended mid-turn")
	}
	if trailer.Error != "" {
		return trailer, fmt.Errorf("turn failed: %s", trailer.Error)
	}
	return trailer, nil
}
