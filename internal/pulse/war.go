package pulse

import (
	"sync"
	"time"
)

const warWindow = 10 * time.Second

type branchPusher struct {
	Pusher string
	At     time.Time
}

// WarDetector detects concurrent pushers on the same branch.
type WarDetector struct {
	mu      sync.Mutex
	last    map[string]branchPusher
	wars    int
	warSeen map[string]time.Time
}

// NewWarDetector creates a branch war detector.
func NewWarDetector() *WarDetector {
	return &WarDetector{
		last:    make(map[string]branchPusher),
		warSeen: make(map[string]time.Time),
	}
}

// Check returns a branch_war event if two pushers collide on the same branch.
func (w *WarDetector) Check(event Event) *Event {
	if !event.IsPushLike() || event.Pusher == "" || event.Branch == "" {
		return nil
	}

	key := event.BranchKey()
	now := time.Now()

	w.mu.Lock()
	defer w.mu.Unlock()

	prev, ok := w.last[key]
	w.last[key] = branchPusher{Pusher: event.Pusher, At: now}

	if !ok || prev.Pusher == event.Pusher {
		return nil
	}
	if now.Sub(prev.At) > warWindow {
		return nil
	}

	// Debounce duplicate war alerts for same branch within window.
	if t, seen := w.warSeen[key]; seen && now.Sub(t) < warWindow {
		return nil
	}
	w.warSeen[key] = now
	w.wars++

	war := NewEvent()
	war.Type = TypeBranchWar
	war.TS = now
	war.Repo = event.Repo
	war.Ref = event.Ref
	war.Branch = event.Branch
	war.Pusher = event.Pusher
	war.Source = event.Source
	war.Attackers = []string{prev.Pusher, event.Pusher}
	return &war
}

// ActiveWars returns wars detected in the last 60 seconds.
func (w *WarDetector) ActiveWars() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	count := 0
	for k, t := range w.warSeen {
		if now.Sub(t) > 60*time.Second {
			delete(w.warSeen, k)
			continue
		}
		count++
	}
	return count
}
