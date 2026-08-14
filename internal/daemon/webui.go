package daemon

import (
	"embed"
	"io/fs"
	"net/http"
)

// webdist holds the built web UI (web/ built with Bun, see web/package.json).
// It is committed rather than generated at `go build` time so the module
// stays buildable from a bare clone without a Bun toolchain; regenerate it
// with `cd web && bun install && bun run build` after changing web/ and
// commit the result alongside the source change.
//
//go:embed all:webdist
var webdist embed.FS

// webUIHandler serves the built web UI. Unlike the /status, /conversations,
// /turn, /beat, and /shutdown routes, it is deliberately not wrapped in
// s.auth: a browser's initial navigation to zeroclaw web's URL can't attach
// an Authorization header, so the page shell itself must be reachable
// without the bearer token. The shell carries no agent data; the token
// travels in the launch URL's query string instead and every API call the
// page makes afterward still goes through s.auth like any other client.
func webUIHandler() (http.Handler, error) {
	sub, err := fs.Sub(webdist, "webdist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		// 'self' rather than the API routes' 'none': the shell needs to load
		// its own script and stylesheet and call back into this same origin.
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		fileServer.ServeHTTP(w, r)
	}), nil
}
