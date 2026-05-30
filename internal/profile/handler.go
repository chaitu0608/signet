package profile

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"server/internal/store"
)

// HandleLeaderboard returns top developers by attested rep.
func HandleLeaderboard(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := st.Leaderboard(r.Context(), 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"leaderboard": entries})
	}
}

// HandleRecent returns recent attestations across all developers.
func HandleRecent(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if q := r.URL.Query().Get("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 100 {
				limit = n
			}
		}
		events, err := st.RecentAttested(r.Context(), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"recent": events})
	}
}

// HandleDev routes /api/dev/{addr} and sub-resources.
func HandleDev(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/dev/")
		path = strings.Trim(path, "/")
		if path == "" || path == "leaderboard" || path == "recent" {
			http.NotFound(w, r)
			return
		}

		parts := strings.Split(path, "/")
		addr := parts[0]
		if !strings.HasPrefix(strings.ToLower(addr), "0x") {
			http.Error(w, "invalid address", http.StatusBadRequest)
			return
		}

		if len(parts) >= 2 {
			switch parts[1] {
			case "badge.svg":
				serveBadge(w, r, st, addr)
				return
			case "og.svg":
				serveOG(w, r, st, addr)
				return
			case "timeseries":
				serveTimeseries(w, r, st, addr)
				return
			}
		}

		prof, err := st.DevProfile(r.Context(), addr, 100)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if prof == nil {
			prof = &store.DevProfile{Address: addr}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(prof)
	}
}

func serveBadge(w http.ResponseWriter, r *http.Request, st store.Store, addr string) {
	prof, err := st.DevProfile(r.Context(), addr, 1000)
	if err != nil || prof == nil {
		prof = &store.DevProfile{Address: addr}
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = fmt.Fprint(w, renderBadgeSVG(prof, addr))
}

func serveOG(w http.ResponseWriter, r *http.Request, st store.Store, addr string) {
	prof, err := st.DevProfile(r.Context(), addr, 1000)
	if err != nil || prof == nil {
		prof = &store.DevProfile{Address: addr}
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = fmt.Fprint(w, renderOGSVG(prof, addr))
}

func serveTimeseries(w http.ResponseWriter, r *http.Request, st store.Store, addr string) {
	days := 30
	if q := r.URL.Query().Get("days"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	buckets, err := st.Timeseries(r.Context(), addr, days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"timeseries": buckets})
}

// ServePage serves dev.html for /dev/* paths.
func ServePage(staticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(staticDir, "dev.html"))
	}
}
