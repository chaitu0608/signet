package serverlite

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"

	"server/internal/profile"
	"server/internal/store"
)

//go:embed all:static
var embeddedStatic embed.FS

var (
	once    sync.Once
	handler http.Handler
)

// Handler is a Vercel-safe HTTP handler (no go-ethereum / chain deps).
func Handler() http.Handler {
	once.Do(func() {
		handler = newMux()
	})
	return handler
}

func newMux() http.Handler {
	ctx := context.Background()
	st := store.NewMemory()
	if os.Getenv("SIGNET_DEV_SEED") == "1" || os.Getenv("VERCEL") == "1" {
		store.MaybeSeed(ctx, st)
	}

	staticFS := mustStaticFS()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/api/chain", chainConfigHandler)
	mux.HandleFunc("/api/events", eventsHandler(st))
	mux.HandleFunc("/api/dev/leaderboard", profile.HandleLeaderboard(st))
	mux.HandleFunc("/api/dev/recent", profile.HandleRecent(st))
	mux.HandleFunc("/api/dev/", profile.HandleDev(st))
	mux.HandleFunc("/embed/", profile.HandleEmbed(st))
	mux.HandleFunc("/dev/", serveDevHTML())
	mux.HandleFunc("/web3", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dev/leaderboard", http.StatusFound)
	})
	mux.HandleFunc("/zendo", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, r, staticFS, "zendo.html")
	})
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	return mux
}

func mustStaticFS() fs.FS {
	sub, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		panic("embed static: " + err.Error())
	}
	return sub
}

func serveDevHTML() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(mustStaticFS(), "dev.html")
		if err != nil {
			http.Error(w, "dev.html missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	}
}

func serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(name, ".svg") {
		w.Header().Set("Content-Type", "image/svg+xml")
	} else if strings.HasSuffix(name, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	_, _ = w.Write(data)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "mode": "vercel-lite"})
}

func chainConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"chain_id":    84532,
		"enabled":     false,
		"eas_enabled": false,
		"explorer":    "https://sepolia.basescan.org",
		"eas_explorer": "https://base-sepolia.easscan.org",
		"note":        "Full chain pipeline requires a long-running host (Railway/Fly), not Vercel serverless",
	})
}

func eventsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, _ := st.RecentEvents(r.Context(), 50)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events": events,
			"count":  len(events),
		})
	}
}

// SyncStatic is a no-op; static files are committed under internal/serverlite/static for embed.
