package domain

import "time"

// Event types.
const (
	TypePush         = "push"
	TypeForcePush    = "force_push"
	TypeBranchCreate = "branch_create"
	TypeBranchDelete = "branch_delete"
	TypeTag          = "tag"
	TypeBranchWar    = "branch_war"
)

const (
	AnchorPending  = "pending"
	AnchorAnchored = "anchored"
)

// CommitInfo is a single commit in an event.
type CommitInfo struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

// Event is a normalized git operation.
type Event struct {
	ID            string       `json:"id"`
	TS            time.Time    `json:"ts"`
	Type          string       `json:"type"`
	Repo          string       `json:"repo"`
	Ref           string       `json:"ref"`
	Branch        string       `json:"branch"`
	OldSHA        string       `json:"old_sha,omitempty"`
	NewSHA        string       `json:"new_sha,omitempty"`
	Pusher        string       `json:"pusher"`
	Commits       int          `json:"commits,omitempty"`
	Force         bool         `json:"force"`
	Source        string       `json:"source"`
	LatencyMS     int64        `json:"latency_ms"`
	GitHubLagMS   int64        `json:"github_lag_ms,omitempty"`
	CommitsDetail []CommitInfo `json:"commits_detail,omitempty"`
	Attackers     []string     `json:"attackers,omitempty"`
	Signature     string       `json:"signature,omitempty"`
	Signer        string       `json:"signer,omitempty"`
	AnchorStatus  string       `json:"anchor_status,omitempty"`
	AnchorTx      string       `json:"anchor_tx,omitempty"`
	BatchID       uint64       `json:"batch_id,omitempty"`
	LeafHash       string   `json:"leaf_hash,omitempty"`
	QualityScore   int      `json:"quality_score,omitempty"`
	QualitySummary string   `json:"quality_summary,omitempty"`
	SecurityFlags  []string `json:"security_flags,omitempty"`
	Category       string   `json:"category,omitempty"`
	AttestUID      string   `json:"attest_uid,omitempty"`
	AttestTx       string   `json:"attest_tx,omitempty"`
	ReputationTx   string   `json:"reputation_tx,omitempty"`
}

func (e Event) BranchKey() string {
	return e.Repo + ":" + e.Branch
}

func (e Event) IsPushLike() bool {
	return e.Type == TypePush || e.Type == TypeForcePush
}

func (e Event) ShouldAnchor() bool {
	return e.Type != TypeBranchWar && e.LeafHash != ""
}
