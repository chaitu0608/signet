package pulse

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"server/internal/relay"
	"server/internal/reviewer"
	"server/internal/store"
)

const (
	maxEvents       = 200
	presenceEvery   = 2 * time.Second
	pusherWindow    = 60 * time.Second
	branchWindow    = 60 * time.Second
)

type branchActivity struct {
	Repo   string    `json:"repo"`
	Branch string    `json:"branch"`
	Count  int       `json:"count"`
	Last   time.Time `json:"last"`
}

// Hub fans out git events to live WebSocket clients.
type Hub struct {
	mu sync.RWMutex

	st       store.Store
	rl       relay.Relay
	reviewer reviewer.Reviewer
	ctx      context.Context

	clients map[*Client]bool
	events  []Event
	war     *WarDetector

	pusherSeen map[string]time.Time
	branchHits map[string]*branchActivity

	ingest     chan Event
	external   chan []byte
	register   chan *Client
	unregister chan *Client
}

// NewHub creates a Pulse hub.
func NewHub(ctx context.Context, st store.Store, rl relay.Relay, rev reviewer.Reviewer) *Hub {
	return &Hub{
		st:         st,
		rl:         rl,
		reviewer:   rev,
		ctx:        ctx,
		clients:    make(map[*Client]bool),
		events:     make([]Event, 0, maxEvents),
		war:        NewWarDetector(),
		pusherSeen: make(map[string]time.Time),
		branchHits: make(map[string]*branchActivity),
		ingest:     make(chan Event),
		external:   make(chan []byte, 64),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run processes hub events.
func (h *Hub) Run() {
	presenceTicker := time.NewTicker(presenceEvery)
	defer presenceTicker.Stop()

	go func() {
		if err := h.rl.Subscribe(h.ctx, h.external); err != nil && h.ctx.Err() == nil {
			slog.Warn("relay subscribe ended", "err", err)
		}
	}()

	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.sendReplay(client)

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}

		case payload := <-h.external:
			var event Event
			if err := json.Unmarshal(payload, &event); err != nil {
				continue
			}
			h.storeEvent(event)
			h.broadcastEvent(event)

		case event := <-h.ingest:
			h.processIngest(event)

		case <-presenceTicker.C:
			h.broadcastPresence()
		}
	}
}

func (h *Hub) processIngest(event Event) {
	if event.LeafHash == "" && event.Type != TypeBranchWar {
		if leaf, err := EventLeafHash(event); err == nil {
			event.LeafHash = leaf
		}
	}
	if h.st != nil {
		if err := h.st.Insert(h.ctx, ToDomain(event)); err != nil {
			slog.Warn("store insert failed", "err", err)
		}
	}
	if b, err := json.Marshal(event); err == nil {
		_ = h.rl.Publish(h.ctx, b)
	}

	h.storeEvent(event)
	h.broadcastEvent(event)
	if h.reviewer != nil && event.IsPushLike() {
		go h.runReview(event)
	}
	if event.IsPushLike() {
		if war := h.war.Check(event); war != nil {
			Finalize(war)
			if h.st != nil {
				_ = h.st.Insert(h.ctx, ToDomain(*war))
			}
			if b, err := json.Marshal(*war); err == nil {
				_ = h.rl.Publish(h.ctx, b)
			}
			h.storeEvent(*war)
			h.broadcastEvent(*war)
		}
	}
}

// Ingest adds an event to the hub.
func (h *Hub) Ingest(event Event) {
	Finalize(&event)
	h.ingest <- event
}

// Store returns the backing store.
func (h *Hub) Store() store.Store {
	return h.st
}

// RecentEvents returns recent events from store or memory cache.
func (h *Hub) RecentEvents() []Event {
	if h.st != nil {
		domainEvents, err := h.st.RecentEvents(h.ctx, maxEvents)
		if err == nil && len(domainEvents) > 0 {
			out := make([]Event, len(domainEvents))
			for i, e := range domainEvents {
				out[i] = FromDomain(e)
			}
			return out
		}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Event, len(h.events))
	copy(out, h.events)
	return out
}

func (h *Hub) storeEvent(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.events = append(h.events, event)
	if len(h.events) > maxEvents {
		h.events = h.events[len(h.events)-maxEvents:]
	}

	if event.Pusher != "" && event.Type != TypeBranchWar {
		h.pusherSeen[event.Pusher] = time.Now()
	}

	if event.Branch != "" && event.Repo != "" && event.Type != TypeBranchWar {
		key := event.BranchKey()
		ba, ok := h.branchHits[key]
		if !ok {
			ba = &branchActivity{Repo: event.Repo, Branch: event.Branch}
			h.branchHits[key] = ba
		}
		ba.Count++
		ba.Last = time.Now()
	}

	now := time.Now()
	for k, t := range h.pusherSeen {
		if now.Sub(t) > pusherWindow {
			delete(h.pusherSeen, k)
		}
	}
	for k, ba := range h.branchHits {
		if now.Sub(ba.Last) > branchWindow {
			delete(h.branchHits, k)
		}
	}
}

func (h *Hub) sendReplay(client *Client) {
	events := h.RecentEvents()
	for _, e := range events {
		msg, _ := json.Marshal(map[string]any{
			"type":  "event",
			"event": e,
		})
		select {
		case client.send <- msg:
		default:
			return
		}
	}
}

func (h *Hub) runReview(event Event) {
	ctx, cancel := context.WithTimeout(h.ctx, 60*time.Second)
	defer cancel()
	rev, err := h.reviewer.Review(ctx, ToDomain(event))
	if err != nil {
		slog.Warn("review failed", "event", event.ID, "err", err)
		return
	}
	event.QualityScore = rev.Score
	event.QualitySummary = rev.Summary
	event.SecurityFlags = rev.Security
	event.Category = rev.Category
	if h.st != nil {
		de := ToDomain(event)
		if existing, _ := h.st.GetByID(ctx, event.ID); existing != nil {
			de = *existing
			de.QualityScore = rev.Score
			de.QualitySummary = rev.Summary
			de.SecurityFlags = rev.Security
			de.Category = rev.Category
		}
		if err := h.st.UpdateEvent(ctx, de); err != nil {
			slog.Warn("update event review failed", "err", err)
		}
	}
	h.mu.Lock()
	for i := range h.events {
		if h.events[i].ID == event.ID {
			h.events[i].QualityScore = rev.Score
			h.events[i].QualitySummary = rev.Summary
			h.events[i].SecurityFlags = rev.Security
			h.events[i].Category = rev.Category
			event = h.events[i]
			break
		}
	}
	h.mu.Unlock()
	h.broadcastEventUpdated(event)
}

func (h *Hub) broadcastEventUpdated(event Event) {
	msg, _ := json.Marshal(map[string]any{
		"type":  "event_updated",
		"event": event,
	})
	h.broadcast(msg)
}

func (h *Hub) broadcastEvent(event Event) {
	msg, _ := json.Marshal(map[string]any{
		"type":  "event",
		"event": event,
	})
	h.broadcast(msg)
}

func (h *Hub) broadcastPresence() {
	h.mu.RLock()
	pushers := make([]string, 0, len(h.pusherSeen))
	for p := range h.pusherSeen {
		pushers = append(pushers, p)
	}
	branches := make([]branchActivity, 0, len(h.branchHits))
	for _, ba := range h.branchHits {
		branches = append(branches, *ba)
	}
	viewers := len(h.clients)
	h.mu.RUnlock()

	msg, _ := json.Marshal(map[string]any{
		"type":        "presence",
		"viewers":     viewers,
		"pushers":     pushers,
		"branches":    branches,
		"wars_active": h.war.ActiveWars(),
	})
	h.broadcast(msg)
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
