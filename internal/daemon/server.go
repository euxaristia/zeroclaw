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
	"zeroclaw/internal/catalog"
	"zeroclaw/internal/channels"
	"zeroclaw/internal/config"
	"zeroclaw/internal/env"
)

type TurnRequest struct {
	Conversation string `json:"conversation"`
	Prompt       string `json:"prompt"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
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
	agentName    string
	container    string
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
func RunServer(agentNameOpt ...string) error {
	agentName := "default"
	if len(agentNameOpt) > 0 && agentNameOpt[0] != "" {
		agentName = agentNameOpt[0]
	}
	if existing, ok := Running(agentName); ok && existing.PID != os.Getpid() {
		return fmt.Errorf("zeroclawd already running for agent %s (pid %d)", agentName, existing.PID)
	}
	cfg, err := config.Load(agentName)
	if err != nil {
		return err
	}
	sessPath, err := config.Path("conversations.json", agentName)
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
	driver, err := agent.NewDriver(cfg.Backend)
	if err != nil {
		return err
	}
	s := &server{
		agentName:    agentName,
		container:    env.ContainerName(agentName),
		driver:       driver,
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
	if err := saveInfo(Info{Port: port, Token: s.token, PID: os.Getpid()}, agentName); err != nil {
		return err
	}
	defer removeInfo(agentName)

	schedCtx, cancelSched := context.WithCancel(context.Background())
	defer cancelSched()
	s.startScheduler(schedCtx, cfg)

	// Telegram channel (M3): long polling against the Bot API, single-owner
	// allowlist by chat id. Disabled when no token is configured.
	tgCtx, cancelTg := context.WithCancel(context.Background())
	defer cancelTg()
	if tg, ok, err := config.LoadTelegram(agentName); ok {
		go channels.StartTelegram(tgCtx, tg, s)
	} else if err != nil {
		log.Printf("telegram (%s): config error, channel disabled: %v", agentName, err)
	} else {
		log.Printf("telegram (%s): no token configured, channel disabled", agentName)
	}

	shutdown := make(chan struct{})
	mux := http.NewServeMux()
	mux.Handle("GET /status", s.auth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("GET /conversations", s.auth(http.HandlerFunc(s.handleConversations)))
	mux.Handle("GET /providers", s.auth(http.HandlerFunc(s.handleProviders)))
	mux.Handle("GET /models", s.auth(http.HandlerFunc(s.handleModels)))
	mux.Handle("POST /turn", s.auth(http.HandlerFunc(s.handleTurn)))
	mux.Handle("POST /beat", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		go s.runScheduled(schedCtx, "heartbeat", heartbeatPrompt)
		w.WriteHeader(http.StatusAccepted)
	})))
	mux.Handle("POST /shutdown", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		close(shutdown)
	})))
	// The web UI shell is intentionally outside s.auth; see webUIHandler.
	webUI, err := webUIHandler()
	if err != nil {
		return err
	}
	mux.Handle("/", webUI)

	httpSrv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()
	log.Printf("zeroclawd (%s) listening on 127.0.0.1:%d (pid %d)", agentName, port, os.Getpid())

	select {
	case <-shutdown:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
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
		Container: s.container,
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
		// Add standard security headers to all responses
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")

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
	body := map[string]any{
		"agent":         s.agentName,
		"container":     s.container,
		"pid":           os.Getpid(),
		"conversations": len(s.sessions.All()),
	}
	// Zeroclaw holds no default provider/model; the backend resolves them.
	// Report them so a client can show what a turn would use before one has
	// run. Best effort: a container that is down must not fail /status.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if def, err := s.driver.Defaults(ctx, s.container); err == nil {
		body["provider"] = def.Provider
		body["model"] = def.Model
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (s *server) handleConversations(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(s.sessions.All())
}

func (s *server) handleProviders(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(catalog.Providers())
}

// handleModels mirrors internal/tui's model picker: try the provider's live
// catalog when it has one, and fall back to the curated static list on any
// failure (unsupported provider, network error, bad response) rather than
// erroring the request.
func (s *server) handleModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if catalog.LiveFetchSupported(provider) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if items, err := catalog.FetchLive(ctx, provider); err == nil {
			_ = json.NewEncoder(w).Encode(items)
			return
		}
	}
	_ = json.NewEncoder(w).Encode(catalog.StaticModels(provider))
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
		_ = enc.Encode(v)
		if flusher != nil {
			flusher.Flush()
		}
	}

	opts := agent.TurnOptions{
		Container: s.container,
		SessionID: s.sessions.Get(req.Conversation),
		Prompt:    req.Prompt,
		Provider:  req.Provider,
		Model:     req.Model,
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
