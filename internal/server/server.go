package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"server/internal/chain"
	"server/internal/profile"
	"server/internal/pulse"
	"server/internal/relay"
	"server/internal/reviewer"
	"server/internal/stillroom"
	"server/internal/store"
	"server/internal/ws"
)

var (
	once    sync.Once
	handler http.Handler
)

// Handler returns the shared HTTP handler (safe for Vercel serverless reuse).
func Handler() http.Handler {
	once.Do(func() {
		handler = newMux()
	})
	return handler
}

func newMux() http.Handler {
	ctx := context.Background()

	st, err := store.Open(ctx)
	if err != nil {
		slog.Error("store open failed", "err", err)
		st = store.NewMemory()
	}
	if os.Getenv("SIGNET_DEV_SEED") == "1" || os.Getenv("VERCEL") == "1" {
		store.MaybeSeed(ctx, st)
	}

	rl := relay.OpenFromEnv()
	chatHub := ws.NewHub()
	go chatHub.Run()

	stillHub := stillroom.NewHub()
	go stillHub.Run()

	rev := reviewer.New(os.Getenv("OPENAI_API_KEY"))
	pulseHub := pulse.NewHub(ctx, st, rl, rev)
	go pulseHub.Run()

	chainCfg := chain.LoadConfig()
	var chainCli *chain.Client
	if chainCfg.Enabled() && os.Getenv("VERCEL") == "" {
		chainCli, err = chain.NewClient(ctx, chainCfg)
		if err != nil {
			slog.Warn("chain client init failed", "err", err)
		} else {
			chain.StartWorkers(ctx, chainCfg, st, chainCli)
		}
	}

	if repos := os.Getenv("WATCH_REPOS"); repos != "" && os.Getenv("VERCEL") == "" {
		paths := strings.Split(repos, ",")
		go pulse.WatchRepos(pulseHub, paths)
	}

	staticDir := staticDir()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", HealthHandler)
	mux.HandleFunc("/hook", pulse.HandleHook(pulseHub))
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("VERCEL") != "" {
			http.Error(w, "websockets unavailable on serverless; use /api/dev/recent", http.StatusNotImplemented)
			return
		}
		pulse.ServeLive(pulseHub, w, r)
	})
	mux.HandleFunc("/api/events", pulse.HandleEventsAPI(pulseHub))
	mux.HandleFunc("/api/proof/", chain.HandleProof(st, chainCli))
	mux.HandleFunc("/api/bounty/claim", chain.HandleBountyClaim(st, chainCfg))
	mux.HandleFunc("/api/bounties", chain.HandleBounties(st))
	mux.HandleFunc("/api/sbt/", chain.HandleSBTLookup(st, chainCli))
	mux.HandleFunc("/api/chain", chain.HandleChainConfig(chainCfg))
	mux.HandleFunc("/api/dev/leaderboard", profile.HandleLeaderboard(st))
	mux.HandleFunc("/api/dev/recent", profile.HandleRecent(st))
	mux.HandleFunc("/api/dev/", profile.HandleDev(st))
	mux.HandleFunc("/embed/", profile.HandleEmbed(st))
	mux.HandleFunc("/dev/", profile.ServePage(staticDir))
	mux.HandleFunc("/still", func(w http.ResponseWriter, r *http.Request) {
		stillroom.ServeStill(stillHub, w, r)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWS(chatHub, w, r)
	})
	mux.HandleFunc("/zendo", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(staticDir, "zendo.html"))
	})
	mux.HandleFunc("/web3", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dev/leaderboard", http.StatusFound)
	})
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))

	return mux
}

func staticDir() string {
	candidates := []string{
		filepath.Join(".", "static"),
		filepath.Join("..", "static"),
		"/var/task/static",
		"/var/task/server/static",
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return filepath.Join(".", "static")
}

// HealthHandler serves GET /health.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
