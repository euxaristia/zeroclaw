package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"zeroclaw/internal/agent"
)

// stubDriver stands in for the zero backend. RunServer always sets a real
// driver, so tests supply one rather than the production path tolerating a
// nil one.
type stubDriver struct {
	defaults    agent.Defaults
	defaultsErr error
}

func (stubDriver) Turn(context.Context, agent.TurnOptions, func(agent.Event)) (agent.TurnResult, error) {
	return agent.TurnResult{}, nil
}

func (d stubDriver) Defaults(context.Context, string) (agent.Defaults, error) {
	return d.defaults, d.defaultsErr
}

// capturingDriver records the options it was handed so a test can assert
// what the daemon forwarded.
type capturingDriver struct {
	stubDriver
	opts agent.TurnOptions
}

func (d *capturingDriver) Turn(_ context.Context, opts agent.TurnOptions, _ func(agent.Event)) (agent.TurnResult, error) {
	d.opts = opts
	return agent.TurnResult{Status: "success"}, nil
}

func newTestServer() *server {
	return &server{
		token:  "test-token",
		convs:  map[string]*sync.Mutex{},
		driver: stubDriver{defaults: agent.Defaults{Provider: "stub-provider", Model: "stub-model"}},
	}
}

func TestConvLockCreatesAndReuses(t *testing.T) {
	s := newTestServer()
	a := s.convLock("main")
	if a == nil {
		t.Fatal("convLock returned nil")
	}
	b := s.convLock("main")
	if a != b {
		t.Error("convLock returned a different mutex for the same conversation")
	}
	c := s.convLock("other")
	if c == a {
		t.Error("convLock returned the same mutex for a different conversation")
	}
}

func TestAuthRejectsBadToken(t *testing.T) {
	s := newTestServer()
	var hit bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true })
	handler := s.auth(next)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if hit {
		t.Error("auth allowed a request with the wrong token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("auth status = %d, want 401", rec.Code)
	}
}

func TestAuthAcceptsGoodToken(t *testing.T) {
	s := newTestServer()
	var hit bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true })
	handler := s.auth(next)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !hit {
		t.Error("auth rejected a request with the correct token")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("auth status = %d, want 200", rec.Code)
	}
}

func TestHandleStatus(t *testing.T) {
	s := newTestServer()
	s.sessions = mustSessionStore(t)
	rec := httptest.NewRecorder()
	s.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("status body not json: %v", err)
	}
	if _, ok := body["pid"]; !ok {
		t.Errorf("status body missing pid: %v", body)
	}
	if n, ok := body["conversations"].(float64); !ok || int(n) != 0 {
		t.Errorf("status conversations = %v, want 0", body["conversations"])
	}
	if body["provider"] != "stub-provider" || body["model"] != "stub-model" {
		t.Errorf("status provider/model = %v/%v, want stub-provider/stub-model", body["provider"], body["model"])
	}
}

// Zero's model registry does not enumerate gateway-routed model ids, so a
// zero ContextWindow from the driver must fall back to the curated catalog
// rather than reporting nothing.
func TestHandleStatusFallsBackToCatalogContextWindow(t *testing.T) {
	s := newTestServer()
	s.sessions = mustSessionStore(t)
	s.driver = stubDriver{defaults: agent.Defaults{
		Provider: "gitlawb-opengateway",
		Model:    "nvidia/nemotron-3-ultra-550b-a55b:free",
		// Registry miss.
		ContextWindow: 0,
	}}

	rec := httptest.NewRecorder()
	s.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("status body not json: %v", err)
	}
	if got, ok := body["contextWindow"].(float64); !ok || int(got) != 128000 {
		t.Errorf("contextWindow = %v, want 128000 from the catalog fallback", body["contextWindow"])
	}
	if body["contextWindowLabel"] != "128K ctx" {
		t.Errorf("contextWindowLabel = %v, want 128K ctx", body["contextWindowLabel"])
	}
}

// An unknown model reports no context window at all rather than a zero,
// mirroring zero's own "only render it when > 0".
func TestHandleStatusOmitsUnknownContextWindow(t *testing.T) {
	s := newTestServer()
	s.sessions = mustSessionStore(t)
	s.driver = stubDriver{defaults: agent.Defaults{Provider: "custom", Model: "not-a-real-model"}}

	rec := httptest.NewRecorder()
	s.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("status body not json: %v", err)
	}
	if _, ok := body["contextWindow"]; ok {
		t.Errorf("contextWindow should be absent for an unknown model, got %v", body["contextWindow"])
	}
}

// A container that is down must degrade /status, not break it: the fields
// are display sugar for the web UI's model indicator.
func TestHandleStatusOmitsDefaultsOnError(t *testing.T) {
	s := newTestServer()
	s.sessions = mustSessionStore(t)
	s.driver = stubDriver{defaultsErr: errors.New("container down")}

	rec := httptest.NewRecorder()
	s.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("status body not json: %v", err)
	}
	if _, ok := body["provider"]; ok {
		t.Errorf("provider should be absent when the backend cannot be reached, got %v", body["provider"])
	}
	if body["agent"] == nil {
		t.Error("the rest of status should still be reported")
	}
}

func TestHandleDeleteConversation(t *testing.T) {
	s := newTestServer()
	s.sessions = mustSessionStore(t)
	if err := s.sessions.Set("main", "sess-1"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/conversations/main", nil)
	req.SetPathValue("name", "main")
	s.handleDeleteConversation(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	if got := s.sessions.Get("main"); got != "" {
		t.Errorf("session still mapped after reset: %q", got)
	}
}

func TestHandleDeleteConversationRequiresName(t *testing.T) {
	s := newTestServer()
	s.sessions = mustSessionStore(t)

	rec := httptest.NewRecorder()
	s.handleDeleteConversation(rec, httptest.NewRequest(http.MethodDelete, "/conversations/", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("delete without a name = %d, want 400", rec.Code)
	}
}

// The turn options a browser can set are validated at the boundary so a bad
// value fails here with a clear message instead of deep inside the container.
func TestHandleTurnRejectsBadOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"unknown effort", `{"prompt":"hi","reasoningEffort":"ultra"}`},
		{"negative turns", `{"prompt":"hi","maxTurns":-1}`},
		{"absurd turns", `{"prompt":"hi","maxTurns":100000}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer()
			s.sessions = mustSessionStore(t)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/turn", strings.NewReader(tc.body))
			s.handleTurn(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", rec.Code, tc.body)
			}
		})
	}
}

// A valid effort must survive validation and reach the driver.
func TestHandleTurnForwardsOptions(t *testing.T) {
	s := newTestServer()
	s.sessions = mustSessionStore(t)
	captured := &capturingDriver{}
	s.driver = captured

	rec := httptest.NewRecorder()
	body := `{"prompt":"hi","reasoningEffort":"high","maxTurns":7}`
	s.handleTurn(rec, httptest.NewRequest(http.MethodPost, "/turn", strings.NewReader(body)))

	if captured.opts.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want high", captured.opts.ReasoningEffort)
	}
	if captured.opts.MaxTurns != 7 {
		t.Errorf("MaxTurns = %d, want 7", captured.opts.MaxTurns)
	}
}

func TestHandleConversations(t *testing.T) {
	s := newTestServer()
	s.sessions = mustSessionStore(t)
	if err := s.sessions.Set("main", "sess-1"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.handleConversations(rec, httptest.NewRequest(http.MethodGet, "/conversations", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not json: %v", err)
	}
	if body["main"] != "sess-1" {
		t.Errorf("conversations = %v, want {main: sess-1}", body)
	}
}

func TestHandleProviders(t *testing.T) {
	s := newTestServer()
	rec := httptest.NewRecorder()
	s.handleProviders(rec, httptest.NewRequest(http.MethodGet, "/providers", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("body not json: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected at least one provider")
	}
}

func TestHandleModelsFallsBackToStatic(t *testing.T) {
	s := newTestServer()
	rec := httptest.NewRecorder()
	s.handleModels(rec, httptest.NewRequest(http.MethodGet, "/models?provider=openai", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("body not json: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected at least one openai model from the static catalog")
	}
	for _, item := range items {
		if item["provider"] != "openai" {
			t.Errorf("got model for provider %v, want openai", item["provider"])
		}
	}
}

func mustSessionStore(t *testing.T) *agent.SessionStore {
	t.Helper()
	store, err := agent.OpenSessionStore(t.TempDir() + "/c.json")
	if err != nil {
		t.Fatalf("OpenSessionStore: %v", err)
	}
	return store
}
