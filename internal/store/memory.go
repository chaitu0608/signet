package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"server/internal/domain"
)

// Memory is an in-process Store for dev without Postgres.
type Memory struct {
	mu         sync.RWMutex
	events     []domain.Event
	bounties   []Bounty
	sbtQueue   []SBTQueueItem
	batchRoots map[uint64]string
	proofs     map[string][]string
}

// NewMemory creates an in-memory store.
func NewMemory() *Memory {
	return &Memory{
		batchRoots: make(map[uint64]string),
		proofs:     make(map[string][]string),
	}
}

func (m *Memory) Close() {}

func (m *Memory) Insert(ctx context.Context, e domain.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.AnchorStatus == "" {
		e.AnchorStatus = domain.AnchorPending
	}
	m.events = append(m.events, e)
	if len(m.events) > 500 {
		m.events = m.events[len(m.events)-500:]
	}
	return nil
}

func (m *Memory) RecentEvents(ctx context.Context, limit int) ([]domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := len(m.events)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]domain.Event, limit)
	copy(out, m.events[n-limit:])
	return out, nil
}

func (m *Memory) PendingForAnchor(ctx context.Context, limit int) ([]domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.Event
	for _, e := range m.events {
		if e.AnchorStatus == domain.AnchorPending && e.ShouldAnchor() {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *Memory) MarkAnchored(ctx context.Context, batchID uint64, txHash string, leafHashes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := make(map[string]struct{}, len(leafHashes))
	for _, h := range leafHashes {
		set[h] = struct{}{}
	}
	for i := range m.events {
		if _, ok := set[m.events[i].LeafHash]; ok {
			m.events[i].AnchorStatus = domain.AnchorAnchored
			m.events[i].AnchorTx = txHash
			m.events[i].BatchID = batchID
		}
	}
	return nil
}

func (m *Memory) GetByID(ctx context.Context, id string) (*domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.events {
		if m.events[i].ID == id {
			e := m.events[i]
			return &e, nil
		}
	}
	return nil, nil
}

func (m *Memory) SaveBatchRoot(ctx context.Context, batchID uint64, root string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchRoots[batchID] = root
	return nil
}

func (m *Memory) GetBatchRoot(ctx context.Context, batchID uint64) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.batchRoots[batchID], nil
}

func (m *Memory) GetProof(ctx context.Context, id string) (*Proof, error) {
	e, err := m.GetByID(ctx, id)
	if err != nil || e == nil || e.BatchID == 0 {
		return nil, err
	}
	root, _ := m.GetBatchRoot(ctx, e.BatchID)
	m.mu.RLock()
	proof := m.proofs[e.LeafHash]
	m.mu.RUnlock()
	return &Proof{
		EventID:  e.ID,
		BatchID:  e.BatchID,
		LeafHash: e.LeafHash,
		Root:     root,
		Proof:    proof,
		AnchorTx: e.AnchorTx,
	}, nil
}

func (m *Memory) SetProofsForBatch(proofs map[string][]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range proofs {
		m.proofs[k] = v
	}
}

func (m *Memory) AnchorBatchInMemory(batchID uint64, txHash string, leaves []string, root string, proofs map[string][]string) error {
	_ = m.SaveBatchRoot(context.Background(), batchID, root)
	_ = m.MarkAnchored(context.Background(), batchID, txHash, leaves)
	m.SetProofsForBatch(proofs)
	return nil
}

func (m *Memory) InsertBounty(ctx context.Context, b Bounty) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bounties = append(m.bounties, b)
	return nil
}

func (m *Memory) ListBounties(ctx context.Context, status string, limit int) ([]Bounty, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Bounty
	for i := len(m.bounties) - 1; i >= 0 && len(out) < limit; i-- {
		if status == "" || m.bounties[i].Status == status {
			out = append(out, m.bounties[i])
		}
	}
	return out, nil
}

func (m *Memory) UpdateBountyStatus(ctx context.Context, onChainID uint64, status, claimTx string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.bounties {
		if m.bounties[i].OnChainID == onChainID {
			m.bounties[i].Status = status
			m.bounties[i].ClaimTx = claimTx
		}
	}
	return nil
}

func (m *Memory) QueueSBT(ctx context.Context, item SBTQueueItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sbtQueue = append(m.sbtQueue, item)
	return nil
}

func (m *Memory) PendingSBT(ctx context.Context, limit int) ([]SBTQueueItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []SBTQueueItem
	for _, item := range m.sbtQueue {
		if item.MintedTx == "" {
			out = append(out, item)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *Memory) MarkSBTMinted(ctx context.Context, eventID, txHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.sbtQueue {
		if m.sbtQueue[i].EventID == eventID {
			m.sbtQueue[i].MintedTx = txHash
		}
	}
	return nil
}

func (m *Memory) UpdateEvent(ctx context.Context, e domain.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.events {
		if m.events[i].ID == e.ID {
			m.events[i] = e
			return nil
		}
	}
	return nil
}

func (m *Memory) EventsByRepo(ctx context.Context, repo string, limit int) ([]domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.Event
	for i := len(m.events) - 1; i >= 0 && len(out) < limit; i-- {
		if m.events[i].Repo == repo {
			out = append(out, m.events[i])
		}
	}
	return out, nil
}

func (m *Memory) EventsBySigner(ctx context.Context, signer string, limit int) ([]domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.Event
	for i := len(m.events) - 1; i >= 0 && len(out) < limit; i-- {
		if m.events[i].Signer == signer {
			out = append(out, m.events[i])
		}
	}
	return out, nil
}

func (m *Memory) DevProfile(ctx context.Context, address string, limit int) (*DevProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var filtered []domain.Event
	for i := len(m.events) - 1; i >= 0; i-- {
		e := m.events[i]
		if strings.EqualFold(e.Signer, address) && isAttested(e) {
			filtered = append(filtered, e)
			if len(filtered) >= limit {
				break
			}
		}
	}
	prof := buildDevProfile(address, filtered)
	enrichDevProfile(prof, m.events)
	return prof, nil
}

func (m *Memory) Leaderboard(ctx context.Context, limit int) ([]LeaderboardEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return buildLeaderboardEnriched(m.events, limit), nil
}

func (m *Memory) Timeseries(ctx context.Context, address string, days int) ([]DayBucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return buildTimeseries(m.events, address, days), nil
}

func (m *Memory) RecentAttested(ctx context.Context, limit int) ([]domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return buildRecentAttested(m.events, limit), nil
}

func (m *Memory) WeeklyDelta(ctx context.Context, address string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return weeklyDelta(m.events, address), nil
}

func (m *Memory) CategoryMix(ctx context.Context, address string) ([]CategoryCount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return buildCategoryMix(m.events, address), nil
}

func SortedLeaves(hashes []string) []string {
	out := append([]string(nil), hashes...)
	sort.Strings(out)
	return out
}

// ExportEventsJSON for debugging.
func (m *Memory) ExportEventsJSON() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, _ := json.Marshal(m.events)
	return string(b)
}

var _ Store = (*Memory)(nil)
