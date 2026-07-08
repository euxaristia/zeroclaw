package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"zeroclaw/internal/agent"
)

// fakeBackend records the turns it receives and answers with a canned reply.
type fakeBackend struct {
	mu        sync.Mutex
	turns     []string
	allowed   map[string]bool
	reply     string
	turnErr   error
	deleted   []string
}

func (f *fakeBackend) IsAllowedChat(chatID string) bool {
	return f.allowed[chatID]
}

func (f *fakeBackend) Turn(ctx context.Context, conversation, prompt, autonomy string) (agent.TurnResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.turns = append(f.turns, prompt)
	if f.turnErr != nil {
		return agent.TurnResult{}, f.turnErr
	}
	return agent.TurnResult{Final: f.reply, Status: "ok"}, nil
}

func (f *fakeBackend) DeleteConversation(conversation string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, conversation)
	return nil
}

func (f *fakeBackend) gotTurns() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.turns...)
}

// botServer is a minimal fake Telegram Bot API. It returns the queued updates
// once, then 200/empty for subsequent polls. sendMessage records replies.
func botServer(t *testing.T, updates []update) (*httptest.Server, *[]string) {
	t.Helper()
	sent := &[]string{}
	var mu sync.Mutex
	pending := updates
	served := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/getUpdates"):
			mu.Lock()
			defer mu.Unlock()
			var result []update
			if !served {
				result = pending
				served = true
			}
			json.NewEncoder(w).Encode(getUpdatesResponse{OK: true, Result: result})
		case strings.HasPrefix(r.URL.Path, "/sendMessage"):
			var req sendMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			*sent = append(*sent, req.Text)
			mu.Unlock()
			json.NewEncoder(w).Encode(sendMessageResponse{OK: true})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, sent
}

func newTestChannel(baseURL string, fb Backend) *Channel {
	return &Channel{
		allowed: map[string]bool{"123": true},
		backend: fb,
		baseURL: baseURL,
	}
}

func mkUpdate(id int64, text string) update {
	var u update
	u.UpdateID = int(id)
	u.Message.Chat.ID = id
	u.Message.Text = text
	return u
}

func TestChannelDeliversTurn(t *testing.T) {
	fb := &fakeBackend{allowed: map[string]bool{"123": true}, reply: "hello from agent"}
	srv, sent := botServer(t, []update{mkUpdate(123, "ping me")})

	ch := newTestChannel(srv.URL, fb)
	ctx, cancel := context.WithCancel(context.Background())
	go ch.Run(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if len(fb.gotTurns()) > 0 {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("backend never received a turn")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	if got := fb.gotTurns(); len(got) != 1 || got[0] != "ping me" {
		t.Fatalf("unexpected turns: %v", got)
	}
	if len(*sent) != 1 || (*sent)[0] != "hello from agent" {
		t.Fatalf("unexpected replies sent: %v", *sent)
	}
}

func TestChannelRejectsUnknownChat(t *testing.T) {
	fb := &fakeBackend{allowed: map[string]bool{"999": true}}
	srv, sent := botServer(t, []update{mkUpdate(123, "intruder")})

	ch := newTestChannel(srv.URL, fb)
	ctx, cancel := context.WithCancel(context.Background())
	go ch.Run(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if len(*sent) > 0 {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("rejection message never sent")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	if len(fb.gotTurns()) != 0 {
		t.Fatalf("backend should not have been called: %v", fb.gotTurns())
	}
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "not authorized") {
		t.Fatalf("expected unauthorized reply, got: %v", *sent)
	}
}

func TestChannelResetCommand(t *testing.T) {
	fb := &fakeBackend{allowed: map[string]bool{"123": true}}
	srv, _ := botServer(t, []update{mkUpdate(123, "/new")})

	ch := newTestChannel(srv.URL, fb)
	ctx, cancel := context.WithCancel(context.Background())
	go ch.Run(ctx)

	deadline := time.After(2 * time.Second)
	for {
		fb.mu.Lock()
		done := len(fb.deleted) > 0
		fb.mu.Unlock()
		if done {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("conversation was not reset")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	if len(fb.gotTurns()) != 0 {
		t.Fatalf("reset should not run a turn: %v", fb.gotTurns())
	}
}

func TestChannelReportsTurnError(t *testing.T) {
	fb := &fakeBackend{allowed: map[string]bool{"123": true}, turnErr: fmt.Errorf("boom")}
	srv, sent := botServer(t, []update{mkUpdate(123, "break it")})

	ch := newTestChannel(srv.URL, fb)
	ctx, cancel := context.WithCancel(context.Background())
	go ch.Run(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if len(*sent) > 0 {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("error reply never sent")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	if len(*sent) != 1 || !strings.Contains((*sent)[0], "turn failed") {
		t.Fatalf("expected turn-failed reply, got: %v", *sent)
	}
}

func TestChunkMessage(t *testing.T) {
	// A single long run of ASCII stays under the limit in one chunk.
	short := strings.Repeat("a", 10)
	if got := chunkMessage(short); len(got) != 1 {
		t.Fatalf("short message should be one chunk, got %d", len(got))
	}

	// Exactly maxMessageBytes runes is one chunk; one more splits.
	exact := strings.Repeat("b", maxMessageBytes)
	if got := chunkMessage(exact); len(got) != 1 {
		t.Fatalf("exact-size message should be one chunk, got %d", len(got))
	}
	oneMore := strings.Repeat("b", maxMessageBytes+1)
	if got := chunkMessage(oneMore); len(got) != 2 {
		t.Fatalf("message over limit should split to 2, got %d", len(got))
	}

	// Multi-byte rune (emoji) must never split mid-character.
	emoji := strings.Repeat("😀", maxMessageBytes/2+5)
	for _, c := range chunkMessage(emoji) {
		if len([]rune(c)) > maxMessageBytes {
			t.Fatalf("chunk split a multibyte rune: len %d", len([]rune(c)))
		}
	}
}
