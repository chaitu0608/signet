package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"server/internal/chain"
	"server/internal/profile"
	"server/internal/pulse"
	"server/internal/relay"
	"server/internal/reviewer"
	"server/internal/stillroom"
	"server/internal/store"
	"server/internal/ws"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, err := store.Open(ctx)
	if err != nil {
		slog.Error("store open failed", "err", err)
		os.Exit(1)
	}
	defer st.Close()
	store.MaybeSeed(ctx, st)

	rl := relay.OpenFromEnv()
	defer func() { _ = rl.Close() }()

	chatHub := ws.NewHub()
	go chatHub.Run()

	stillHub := stillroom.NewHub()
	go stillHub.Run()

	rev := reviewer.New(os.Getenv("OPENAI_API_KEY"))
	pulseHub := pulse.NewHub(ctx, st, rl, rev)
	go pulseHub.Run()

	chainCfg := chain.LoadConfig()
	var chainCli *chain.Client
	if chainCfg.Enabled() {
		chainCli, err = chain.NewClient(ctx, chainCfg)
		if err != nil {
			slog.Warn("chain client init failed", "err", err)
		} else {
			chain.StartWorkers(ctx, chainCfg, st, chainCli)
		}
	}

	if repos := os.Getenv("WATCH_REPOS"); repos != "" {
		paths := strings.Split(repos, ",")
		go pulse.WatchRepos(pulseHub, paths)
		slog.Info("watch repos enabled", "repos", paths)
	}

	staticDir := filepath.Join(".", "static")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/hook", pulse.HandleHook(pulseHub))
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
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

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		slog.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown failed", "err", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
