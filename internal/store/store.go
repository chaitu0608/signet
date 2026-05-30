package store

import (
	"context"
	"time"

	"server/internal/domain"
)

// Bounty represents an on-chain bounty mirrored in the database.
type Bounty struct {
	ID        int64     `json:"id"`
	IssueHash string    `json:"issue_hash"`
	Funder    string    `json:"funder"`
	AmountWei string    `json:"amount_wei"`
	Token     string    `json:"token"`
	Status    string    `json:"status"`
	ClaimTx   string    `json:"claim_tx,omitempty"`
	OnChainID uint64    `json:"on_chain_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SBTQueueItem is a pending SBT mint.
type SBTQueueItem struct {
	Address  string `json:"address"`
	LeafHash string `json:"leaf_hash"`
	EventID  string `json:"event_id"`
	MintedTx string `json:"minted_tx,omitempty"`
}

// Proof bundles Merkle proof data for an event.
type Proof struct {
	EventID  string   `json:"event_id"`
	BatchID  uint64   `json:"batch_id"`
	LeafHash string   `json:"leaf_hash"`
	Root     string   `json:"root"`
	Proof    []string `json:"proof"`
	AnchorTx string   `json:"anchor_tx"`
}

// Store persists events and web3 metadata.
type Store interface {
	Close()
	Insert(ctx context.Context, e domain.Event) error
	RecentEvents(ctx context.Context, limit int) ([]domain.Event, error)
	PendingForAnchor(ctx context.Context, limit int) ([]domain.Event, error)
	MarkAnchored(ctx context.Context, batchID uint64, txHash string, leafHashes []string) error
	GetByID(ctx context.Context, id string) (*domain.Event, error)
	GetProof(ctx context.Context, id string) (*Proof, error)
	SaveBatchRoot(ctx context.Context, batchID uint64, root string) error
	GetBatchRoot(ctx context.Context, batchID uint64) (string, error)

	InsertBounty(ctx context.Context, b Bounty) error
	ListBounties(ctx context.Context, status string, limit int) ([]Bounty, error)
	UpdateBountyStatus(ctx context.Context, onChainID uint64, status, claimTx string) error

	QueueSBT(ctx context.Context, item SBTQueueItem) error
	PendingSBT(ctx context.Context, limit int) ([]SBTQueueItem, error)
	MarkSBTMinted(ctx context.Context, eventID, txHash string) error
	EventsBySigner(ctx context.Context, signer string, limit int) ([]domain.Event, error)
	UpdateEvent(ctx context.Context, e domain.Event) error
	EventsByRepo(ctx context.Context, repo string, limit int) ([]domain.Event, error)
	DevProfile(ctx context.Context, address string, limit int) (*DevProfile, error)
	Leaderboard(ctx context.Context, limit int) ([]LeaderboardEntry, error)
	Timeseries(ctx context.Context, address string, days int) ([]DayBucket, error)
	RecentAttested(ctx context.Context, limit int) ([]domain.Event, error)
	WeeklyDelta(ctx context.Context, address string) (int, error)
	CategoryMix(ctx context.Context, address string) ([]CategoryCount, error)
}

// DevProfile is aggregated on-chain reputation for a wallet.
type DevProfile struct {
	Address           string         `json:"address"`
	TotalScore        int            `json:"total_score"`
	CommitCount       int            `json:"commit_count"`
	WeeklyDelta       int            `json:"weekly_delta"`
	SecurityFlagCount int            `json:"security_flag_count"`
	TopRepos          []RepoScore    `json:"top_repos"`
	CategoryMix       []CategoryCount `json:"category_mix"`
	Sparkline         []DayBucket    `json:"sparkline"`
	Attestations      []domain.Event `json:"attestations"`
}

// RepoScore ranks repos by cumulative quality score.
type RepoScore struct {
	Repo  string `json:"repo"`
	Score int    `json:"score"`
	Count int    `json:"count"`
}

// LeaderboardEntry is a ranked developer.
type LeaderboardEntry struct {
	Address     string          `json:"address"`
	TotalScore  int             `json:"total_score"`
	CommitCount int             `json:"commit_count"`
	WeeklyDelta int             `json:"weekly_delta"`
	CategoryMix []CategoryCount `json:"category_mix"`
}
