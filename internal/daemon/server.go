// Package daemon implements zeroclawd: the standalone service that owns the
// agent. It survives terminal close and client disconnects; every CLI command
// except `up` reaches the agent only through this control plane.
package daemon

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"zeroclaw/internal/agent"
	"zeroclaw/internal/channels"
	"zeroclaw/internal/config"
)

type TurnRequest struct {
	Conversation string `json:"conversation"`
	Prompt       string `json:"prompt"`
	// Autonomy defaults to high: inside the container boundary the agent runs
	// unattended and there is no user present to answer permission prompts.
	Autonomy string `json:"autonomy,omitempty"`
}

// Trailer is the final JSONL line of a /turn response, after the raw driver
// events.
type Trailer struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
	Final     string `json:"final"`
	Error     string `json:"error,omitempty"`
}

type server struct {
	driver       agent.Driver
	sessions     *agent.SessionStore
	token        string
	allowedChats map[string]bool

	mu    sync.Mutex
	convs map[string]*sync.Mutex
}

func (s *server) convLock(name string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.convs[name] == nil {
		s.convs[name] = &sync.Mutex{}
	}
	return s.convs[name]
}

// RunServer runs zeroclawd in the foreground of the current process. It is
// normally reached via the hidden `zeroclaw daemon run` subcommand spawned
// detached by `zeroclaw up`.
func RunServer() error {
	if existing, ok := Running(); ok && existing.PID != os.Getpid() {
		return fmt.Errorf("zeroclawd already running (pid %d)", existing.PID)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	sessPath, err := config.Path("conversations.json")
	if err != nil {
		return err
	}
	sessions, err := agent.OpenSessionStore(sessPath)
	if err != nil {
		return err
	}
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, id := range cfg.Telegram.AllowedChats {
		allowed[id] = true
	}
	s := &server{
		driver:       agent.ZeroDriver{},
		sessions:     sessions,
		token:        hex.EncodeToString(tokenBytes),
		allowedChats: allowed,
		convs:        map[string]*sync.Mutex{},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := saveInfo(Info{Port: port, Token: s.token, PID: os.Getpid()}); err != nil {
		return err
	}
	defer removeInfo()

	schedCtx, cancelSched := context.WithCancel(context.Background())
	defer cancelSched()
	s.startScheduler(schedCtx, cfg)

	// Telegram channel (M3): long polling against the Bot API, single-owner
	// allowlist by chat id. Disabled when no token is configured.
	tgCtx, cancelTg := context.WithCancel(context.Background())
	defer cancelTg()
	if tg, ok, err := config.LoadTelegram(); ok {
		go channels.StartTelegram(tgCtx, tg, s)
	} else if err != nil {
		log.Printf("telegram: config error, channel disabled: %v", err)
	} else {
		log.Printf("telegram: no token configured, channel disabled")
	}

	shutdown := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /conversations", s.handleConversations)
	mux.HandleFunc("POST /turn", s.handleTurn)
	mux.HandleFunc("POST /beat", func(w http.ResponseWriter, r *http.Request) {
		go s.runScheduled(schedCtx, "heartbeat", heartbeatPrompt)
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("POST /shutdown", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		close(shutdown)
	})

	httpSrv := &http.Server{
		Handler:           s.auth(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()
	log.Printf("zeroclawd listening on 127.0.0.1:%d (pid %d)", port, os.Getpid())

	select {
	case <-shutdown:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(ctx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// IsAllowedChat reports whether the chat id may drive the agent. The allowlist
// is the single-owner gate: an unknown or empty id is always rejected.
func (s *server) IsAllowedChat(id string) bool {
	return id != "" && s.allowedChats[id]
}

// Turn runs one conversation turn through the driver, persisting the resulting
// session. Shared by the /turn endpoint and the Telegram channel so both paths
// honour the same conversation lock and session bookkeeping.
func (s *server) Turn(ctx context.Context, conversation, prompt, autonomy string) (agent.TurnResult, error) {
	if conversation == "" {
		conversation = "main"
	}
	if autonomy == "" {
		autonomy = "high"
	}
	lock := s.convLock(conversation)
	lock.Lock()
	defer lock.Unlock()

	opts := agent.TurnOptions{
		SessionID: s.sessions.Get(conversation),
		Prompt:    prompt,
		Autonomy:  autonomy,
		Attended:  true, // Telegram turns have an operator on the other end
	}
	res, err := s.driver.Turn(ctx, opts, nil)
	if res.SessionID != "" && opts.SessionID == "" {
		if serr := s.sessions.Set(conversation, res.SessionID); serr != nil && err == nil {
			err = fmt.Errorf("conversation not persisted: %w", serr)
		}
	}
	return res, err
}

// DeleteConversation resets a conversation's session mapping.
func (s *server) DeleteConversation(conversation string) error {
	return s.sessions.Delete(conversation)
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + s.token

		// Hash both tokens to prevent length-based timing leaks.
		// subtle.ConstantTimeCompare returns immediately if lengths mismatch.
		authHash := sha256.Sum256([]byte(auth))
		expectedHash := sha256.Sum256([]byte(expected))

		if subtle.ConstantTimeCompare(authHash[:], expectedHash[:]) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{"pid": os.Getpid(), "conversations": len(s.sessions.All())})
}

func (s *server) handleConversations(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(s.sessions.All())
}

func (s *server) handleTurn(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	var req TurnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		http.Error(w, "bad turn request", http.StatusBadRequest)
		return
	}
	if req.Conversation == "" {
		req.Conversation = "main"
	}
	if req.Autonomy == "" {
		req.Autonomy = "high"
	}

	lock := s.convLock(req.Conversation)
	lock.Lock()
	defer lock.Unlock()

	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/x-ndjson")
	emit := func(v any) {
		enc.Encode(v)
		if flusher != nil {
			flusher.Flush()
		}
	}

	opts := agent.TurnOptions{
		SessionID: s.sessions.Get(req.Conversation),
		Prompt:    req.Prompt,
		Autonomy:  req.Autonomy,
		Attended:  true, // /turn callers (chat, exec) have an operator present
	}
	res, err := s.driver.Turn(r.Context(), opts, func(ev agent.Event) { emit(ev) })
	trailer := Trailer{Type: "zeroclaw_result", SessionID: res.SessionID, Status: res.Status, Final: res.Final}
	if err != nil {
		trailer.Error = err.Error()
	}
	if res.SessionID != "" && opts.SessionID == "" {
		if serr := s.sessions.Set(req.Conversation, res.SessionID); serr != nil && trailer.Error == "" {
			trailer.Error = "conversation not persisted: " + serr.Error()
		}
	}
	emit(trailer)
}
