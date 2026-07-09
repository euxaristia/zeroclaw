// Package channels holds the gateway's inbound chat adapters. The CLI chat is
// itself a channel (a thin RPC client of zeroclawd); Telegram is the second,
// added in M3. Every channel is a peer that turns inbound messages into
// conversation turns through the daemon server and writes replies back.
package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"zeroclaw/internal/agent"
	"zeroclaw/internal/config"
)

// Telegram Bot API surface we use. Long polling only; no webhooks, no inbound
// port (AGENTS.md, Stack and non-goals).
const telegramAPIBase = "https://api.telegram.org/bot"

// defaultPollTimeout bounds each getUpdates call; Telegram holds the
// connection open until an update arrives or this elapses.
const defaultPollTimeout = 30 * time.Second

// maxMessageUnits is Telegram's hard caption/message limit: 4096 UTF-16 code
// units (https://core.telegram.org/bots/api#sendmessage). We chunk the agent's
// reply at the rune level but budget by UTF-16 units, because astral-plane
// runes (emoji, CJK Ext-B+) count as two units each.
const maxMessageUnits = 4096

// Channel drives one Telegram bot for its lifetime: it long-polls getUpdates,
// dispatches allowed chats to the daemon, and sends replies. Callers pass a
// Backend so the long-poll loop is testable without a real bot token.
type Channel struct {
	token   string
	allowed map[string]bool
	backend Backend
	baseURL string
	client  *http.Client
}

// Backend is the small surface the channel needs from the daemon. It is an
// interface so tests can drive the loop with a fake.
type Backend interface {
	IsAllowedChat(chatID string) bool
	Turn(ctx context.Context, conversation, prompt, autonomy string) (agent.TurnResult, error)
	DeleteConversation(conversation string) error
}

// NewChannel builds a Telegram channel bound to the daemon server.
func NewChannel(tg config.Telegram, srv Backend) *Channel {
	allowed := map[string]bool{}
	for _, id := range tg.AllowedChats {
		allowed[id] = true
	}
	return &Channel{
		token:   tg.Token,
		allowed: allowed,
		backend: srv,
		baseURL: telegramAPIBase + tg.Token,
		client:  &http.Client{},
	}
}

// StartTelegram long-polls Telegram until ctx is cancelled. It logs lifecycle
// events and never returns an error; a transient API failure just logs and
// retries, because an unattended agent must not die on a transient blip.
func StartTelegram(ctx context.Context, tg config.Telegram, srv Backend) {
	ch := NewChannel(tg, srv)
	ch.Run(ctx)
}

// Run is the long-poll loop. It is exported so callers can inject a custom
// Backend for tests while sharing the production wiring via NewChannel/Start.
func (c *Channel) Run(ctx context.Context) {
	log.Printf("telegram: channel started (allowed chats: %d)", len(c.allowed))
	offset := 0
	for {
		select {
		case <-ctx.Done():
			log.Printf("telegram: channel stopped")
			return
		default:
		}

		updates, err := c.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("telegram: getUpdates failed: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		for _, up := range updates {
			if up.UpdateID >= offset {
				offset = up.UpdateID + 1
			}
			c.handleUpdate(ctx, up)
		}
	}
}

type update struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

type getUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []update `json:"result"`
}

func (c *Channel) getUpdates(ctx context.Context, offset int) ([]update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=%d", c.baseURL, offset, int(defaultPollTimeout.Seconds()))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getUpdates status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed getUpdatesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decoding getUpdates: %w", err)
	}
	if !parsed.OK {
		return nil, fmt.Errorf("getUpdates not ok: %s", strings.TrimSpace(string(body)))
	}
	return parsed.Result, nil
}

func (c *Channel) handleUpdate(ctx context.Context, up update) {
	chatID := fmt.Sprintf("%d", up.Message.Chat.ID)
	text := strings.TrimSpace(up.Message.Text)
	if text == "" {
		return
	}
	if !c.backend.IsAllowedChat(chatID) {
		log.Printf("telegram: rejecting message from non-allowed chat %s", chatID)
		c.send(ctx, chatID, "This chat is not authorized to talk to zeroclaw.")
		return
	}

	switch {
	case text == "/new":
		c.backend.DeleteConversation("telegram-" + chatID)
		c.send(ctx, chatID, "Conversation reset. Starting fresh.")
		return
	case text == "/ping":
		c.send(ctx, chatID, "pong")
		return
	}

	res, err := c.backend.Turn(ctx, "telegram-"+chatID, text, "high")
	if err != nil {
		log.Printf("telegram: turn failed for chat %s: %v", chatID, err)
		c.send(ctx, chatID, "Sorry, that turn failed: "+err.Error())
		return
	}
	c.send(ctx, chatID, safeFinal(res.Final))
}

type sendMessageRequest struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
	// disable_web_page_preview keeps long tool output from rendering oddly.
	DisableWebPagePreview bool `json:"disable_web_page_preview"`
}

type sendMessageResponse struct {
	OK bool `json:"ok"`
}

// send posts one message, chunking it at the rune boundary when it exceeds
// Telegram's size limit. The last chunk carries any error context when the
// turn itself failed was already reported by the caller.
func (c *Channel) send(ctx context.Context, chatID, text string) {
	for _, chunk := range chunkMessage(text) {
		if err := c.sendOne(ctx, chatID, chunk); err != nil {
			log.Printf("telegram: send failed to chat %s: %v", chatID, err)
			return
		}
	}
}

func (c *Channel) sendOne(ctx context.Context, chatID, text string) error {
	payload, err := json.Marshal(sendMessageRequest{ChatID: chatID, Text: text, DisableWebPagePreview: true})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sendMessage",
		bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sendMessage status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed sendMessageResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("decoding sendMessage: %w", err)
	}
	if !parsed.OK {
		return fmt.Errorf("sendMessage not ok: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

// chunkMessage splits text into Telegram-sized pieces. It preserves newlines
// (splitting after each "\n") and never splits a rune, but budgets by UTF-16
// code units (Telegram's actual limit, maxMessageUnits) rather than rune count,
// so astral-plane runes (2 units each) cannot push a chunk over the wire limit.
func chunkMessage(text string) []string {
	if text == "" {
		return nil
	}
	var out []string
	for _, part := range strings.SplitAfter(text, "\n") {
		runes := []rune(part)
		if len(runes) == 0 {
			continue // Skip the trailing "" that SplitAfter yields on "\n".
		}
		start, used := 0, 0
		for i, r := range runes {
			sz := 1
			if r > 0xFFFF {
				sz = 2
			}
			if used+sz > maxMessageUnits {
				out = append(out, string(runes[start:i]))
				start, used = i, sz
			} else {
				used += sz
			}
		}
		out = append(out, string(runes[start:]))
	}
	return out
}

// safeFinal returns a readable reply when the agent produced none.
func safeFinal(final string) string {
	if strings.TrimSpace(final) == "" {
		return "(no reply text)"
	}
	return final
}
