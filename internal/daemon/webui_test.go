package daemon

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWebUIServesWithoutAuth guards the one deliberate hole in s.auth: a
// browser's first navigation to the web UI can't carry an Authorization
// header, so the shell must be reachable without the token while every API
// route stays gated.
func TestWebUIServesWithoutAuth(t *testing.T) {
	webUI, err := webUIHandler()
	if err != nil {
		t.Fatalf("webUIHandler: %v", err)
	}
	rec := httptest.NewRecorder()
	webUI.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "zeroclaw") {
		t.Errorf("index body missing expected content: %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'self'" {
		t.Errorf("CSP = %q, want default-src 'self'", got)
	}
}

// TestWebUIRouteSplit reproduces RunServer's mux wiring at a smaller scale:
// the web UI mount is unauthenticated, but an API route on the same mux
// still rejects requests with no token.
func TestWebUIRouteSplit(t *testing.T) {
	s := newTestServer()
	s.sessions = mustSessionStore(t)

	webUI, err := webUIHandler()
	if err != nil {
		t.Fatalf("webUIHandler: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /status", s.auth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("/", webUI)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("unauthenticated GET / = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET /status = %d, want 401", rec.Code)
	}
}

// TestWebUIDevDirOverride guards the ZEROCLAW_WEB_DIR escape hatch: when
// set, it must serve from that directory on disk instead of the embedded
// build, so a rebuild is visible on refresh without restarting zeroclawd.
func TestWebUIDevDirOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("dev mode marker"), 0o644); err != nil {
		t.Fatalf("writing dev index.html: %v", err)
	}
	t.Setenv("ZEROCLAW_WEB_DIR", dir)

	webUI, err := webUIHandler()
	if err != nil {
		t.Fatalf("webUIHandler: %v", err)
	}
	rec := httptest.NewRecorder()
	webUI.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "dev mode marker" {
		t.Errorf("body = %q, want the dev dir's file, not the embedded build", rec.Body.String())
	}
}
