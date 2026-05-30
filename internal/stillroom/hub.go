package stillroom

import (
	"crypto/sha256"
	"fmt"
	"time"
)

const (
	phaseThreshold    = 0.08
	alignedTicksNeed  = 3
	bellCooldown      = 5 * time.Minute
	mandalaBroadcast  = 30 * time.Second
	presenceBroadcast = 5 * time.Second
)

// Hub maintains the collective silence room.
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	clientMsg  chan clientMessage

	stillnessSeconds uint64
	mandalaSeed      []byte
	lastBell         time.Time
	lastMandalaSent  time.Time
	lastPresenceSent time.Time

	alignTicks      int
	lastSittingCount int
}

type clientMessage struct {
	client *Client
	event  ClientEvent
}

// NewHub creates a Stillroom hub.
func NewHub() *Hub {
	seed := computeSeed(0, 0)
	return &Hub{
		clients:     make(map[*Client]bool),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		clientMsg:   make(chan clientMessage),
		mandalaSeed: seed,
		lastBell:    time.Now().Add(-bellCooldown),
	}
}

func computeSeed(stillnessSeconds uint64, sittingCount int) []byte {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", stillnessSeconds, sittingCount)))
	return h[:16]
}

// Run processes hub events and the 1 Hz room ticker.
func (h *Hub) Run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.broadcastPresenceIfNeeded(true)

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.broadcastPresenceIfNeeded(true)
			}

		case msg := <-h.clientMsg:
			h.handleClientMessage(msg.client, msg.event)

		case <-ticker.C:
			h.tick()
		}
	}
}

func (h *Hub) sittingCount() int {
	n := 0
	for c := range h.clients {
		if c.sitting {
			n++
		}
	}
	return n
}

func (h *Hub) tick() {
	sitting := h.sittingCount()

	if sitting >= 1 {
		h.stillnessSeconds++
		h.mandalaSeed = computeSeed(h.stillnessSeconds, sitting)
	}

	if sitting >= 2 {
		h.checkBreathAlignment()
	} else {
		h.alignTicks = 0
	}

	now := time.Now()
	if now.Sub(h.lastPresenceSent) >= presenceBroadcast {
		h.broadcastPresence()
		h.lastPresenceSent = now
	}

	if sitting >= 1 && now.Sub(h.lastMandalaSent) >= mandalaBroadcast {
		h.broadcastMandala()
		h.lastMandalaSent = now
	}
}

func (h *Hub) checkBreathAlignment() {
	var phases []float64
	now := time.Now()

	for c := range h.clients {
		if !c.sitting {
			continue
		}
		if now.Sub(c.lastBreathAt) > 5*time.Second {
			h.alignTicks = 0
			return
		}
		phases = append(phases, c.lastPhase)
	}

	if len(phases) < 2 {
		h.alignTicks = 0
		return
	}

	maxDist := 0.0
	for i := 0; i < len(phases); i++ {
		for j := i + 1; j < len(phases); j++ {
			if d := PhaseDistance(phases[i], phases[j]); d > maxDist {
				maxDist = d
			}
		}
	}

	if maxDist <= phaseThreshold {
		h.alignTicks++
	} else {
		h.alignTicks = 0
	}

	if h.alignTicks >= alignedTicksNeed && time.Since(h.lastBell) >= bellCooldown {
		h.broadcast(encodeBell())
		h.lastBell = time.Now()
		h.alignTicks = 0
	}
}

func (h *Hub) handleClientMessage(client *Client, event ClientEvent) {
	switch event.Type {
	case EventSit:
		client.sitting = true
		h.broadcastPresenceIfNeeded(true)

	case EventRise:
		client.sitting = false
		h.broadcastPresenceIfNeeded(true)

	case EventBreath:
		if !client.sitting {
			h.breakSilence(client, "breath before sit")
			return
		}
		client.lastPhase = event.Phase
		client.lastBreathAt = time.Now()
	}
}

func (h *Hub) breakSilence(offender *Client, reason string) {
	h.stillnessSeconds = 0
	h.mandalaSeed = computeSeed(0, 0)
	h.alignTicks = 0

	offender.send <- encodeSilenceBroken("Silence was broken.")
	h.broadcastMandala()

	go func(c *Client) {
		time.Sleep(100 * time.Millisecond)
		c.conn.Close()
	}(offender)
}

func (h *Hub) broadcastPresenceIfNeeded(force bool) {
	sitting := h.sittingCount()
	if force || sitting != h.lastSittingCount {
		h.lastSittingCount = sitting
		h.broadcastPresence()
	}
}

func (h *Hub) broadcastPresence() {
	msg := encodePresence(h.sittingCount(), h.stillnessSeconds)
	h.broadcast(msg)
	h.lastPresenceSent = time.Now()
}

func (h *Hub) broadcastMandala() {
	layers := MandalaLayers(h.stillnessSeconds)
	msg := encodeMandala(FormatSeed(h.mandalaSeed), layers)
	h.broadcast(msg)
	h.lastMandalaSent = time.Now()
}

func (h *Hub) broadcast(message []byte) {
	for client := range h.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}
