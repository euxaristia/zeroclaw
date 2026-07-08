package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"zeroclaw/internal/agent"
)

func newTestServer() *server {
	return &server{
		token: "test-token",
		convs: map[string]*sync.Mutex{},
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

func mustSessionStore(t *testing.T) *agent.SessionStore {
	t.Helper()
	store, err := agent.OpenSessionStore(t.TempDir() + "/c.json")
	if err != nil {
		t.Fatalf("OpenSessionStore: %v", err)
	}
	return store
}
