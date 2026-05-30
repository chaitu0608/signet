package payapi

import (
	"sync"
	"time"
)

// Receipt records a paid oracle query for analytics and abuse control.
type Receipt struct {
	Payer       string    `json:"payer"`
	SettlementTx string   `json:"settlement_tx,omitempty"`
	Route       string    `json:"route"`
	Amount      string    `json:"amount"`
	TS          time.Time `json:"ts"`
}

// Meter stores paid-query receipts.
type Meter struct {
	mu       sync.RWMutex
	receipts []Receipt
	max      int
}

// NewMeter creates a receipt store with a rolling cap.
func NewMeter(max int) *Meter {
	if max <= 0 {
		max = 10000
	}
	return &Meter{max: max}
}

// Record appends a paid query receipt.
func (m *Meter) Record(r Receipt) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.TS.IsZero() {
		r.TS = time.Now().UTC()
	}
	m.receipts = append(m.receipts, r)
	if len(m.receipts) > m.max {
		m.receipts = m.receipts[len(m.receipts)-m.max:]
	}
}

// Recent returns the latest receipts up to limit.
func (m *Meter) Recent(limit int) []Receipt {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := len(m.receipts)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]Receipt, limit)
	copy(out, m.receipts[n-limit:])
	return out
}

// Stats returns aggregate metering stats.
func (m *Meter) Stats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	byRoute := make(map[string]int)
	for _, r := range m.receipts {
		byRoute[r.Route]++
	}
	return map[string]any{
		"total_queries": len(m.receipts),
		"by_route":      byRoute,
	}
}
