package stillroom

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ServeStill upgrades the connection and registers with the Stillroom hub.
func ServeStill(hub *Hub, w http.ResponseWriter, r *http.Request) {
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

	// Send initial room state to the new sitter.
	client.send <- encodePresence(hub.sittingCount(), hub.stillnessSeconds)
	client.send <- encodeMandala(FormatSeed(hub.mandalaSeed), MandalaLayers(hub.stillnessSeconds))

	go client.writePump()
	go client.readPump()
}
