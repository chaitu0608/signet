package pulse

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/websocket"

)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func hookSecret() string {
	return os.Getenv("HOOK_SECRET")
}

func hookToken() string {
	return os.Getenv("HOOK_TOKEN")
}

// HandleHook ingests GitHub or native git hook payloads.
func HandleHook(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var events []Event
		var err error

		if r.Header.Get("X-GitHub-Event") != "" {
			events, err = ReadGitHubHook(r, hookSecret())
		} else {
			if err := VerifyNativeAuth(r, hookToken()); err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var e Event
			e, err = ParseNativeHook(r)
			if err == nil {
				events = []Event{e}
			}
		}

		if err != nil {
			slog.Warn("hook ingest failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for i := range events {
			if events[i].LeafHash == "" {
				h, herr := EventLeafHash(events[i])
				if herr == nil {
					events[i].LeafHash = h
				}
			}
			hub.Ingest(events[i])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    true,
			"count": len(events),
		})
	}
}

// ServeLive streams events over WebSocket.
func ServeLive(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}
	hub.register <- client

	go client.writePump()
	go client.readPump()
}

// HandleEventsAPI returns recent events as JSON.
func HandleEventsAPI(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		limit := 50
		if q := r.URL.Query().Get("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= maxEvents {
				limit = n
			}
		}

		events := hub.RecentEvents()
		if len(events) > limit {
			events = events[len(events)-limit:]
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events": events,
			"count":  len(events),
		})
	}
}
