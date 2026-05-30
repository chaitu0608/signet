package handler

import (
	"net/http"
	"net/url"
	"strings"

	"server/internal/server"
)

// Handler is the Vercel serverless entrypoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	normalizeVercelPath(r)
	server.Handler().ServeHTTP(w, r)
}

// normalizeVercelPath maps Vercel rewrite paths back to app routes.
//
// vercel.json rewrites "/(.*)" -> "/api/$1", so the function receives
// "/api/dev" instead of "/dev". Real API routes arrive as "/api/api/dev/...".
func normalizeVercelPath(r *http.Request) {
	p := r.URL.Path

	switch {
	case strings.HasPrefix(p, "/api/api/"):
		// /api/api/events -> /api/events
		r.URL.Path = strings.TrimPrefix(p, "/api")
	case strings.HasPrefix(p, "/api/"):
		rest := strings.TrimPrefix(p, "/api")
		if strings.HasPrefix(rest, "/api/") {
			return
		}
		if rest == "" {
			r.URL.Path = "/"
		} else {
			r.URL.Path = rest
		}
	case p == "/api" || p == "/api/index":
		if orig := r.Header.Get("X-Vercel-Original-Url"); orig != "" {
			if u, err := url.Parse(orig); err == nil && u.Path != "" {
				r.URL.Path = u.Path
			}
		}
	}
}
