package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
