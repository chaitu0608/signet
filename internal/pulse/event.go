package pulse

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"server/internal/domain"
)

// Re-export types for API compatibility.
type (
	Event      = domain.Event
	CommitInfo = domain.CommitInfo
)

const (
	TypePush         = domain.TypePush
	TypeForcePush    = domain.TypeForcePush
	TypeBranchCreate = domain.TypeBranchCreate
	TypeBranchDelete = domain.TypeBranchDelete
	TypeTag          = domain.TypeTag
	TypeBranchWar    = domain.TypeBranchWar
	AnchorPending  = domain.AnchorPending
	AnchorAnchored = domain.AnchorAnchored
)

const (
	SourceGitHub = "github"
	SourceNative = "native"
	SourceWatch  = "watch"
)

// NewEvent creates an event with ID and timestamp set.
func NewEvent() Event {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return Event{
		ID:           hex.EncodeToString(b[:]),
		TS:           time.Now().UTC(),
		AnchorStatus: AnchorPending,
	}
}

// Finalize sets latency from event timestamp.
func Finalize(e *Event) {
	e.LatencyMS = time.Since(e.TS).Milliseconds()
	if e.LatencyMS < 0 {
		e.LatencyMS = 0
	}
	if e.AnchorStatus == "" {
		e.AnchorStatus = AnchorPending
	}
}

// ToDomain converts to store domain type.
func ToDomain(e Event) domain.Event {
	return domain.Event(e)
}

// FromDomain converts from store domain type.
func FromDomain(e domain.Event) Event {
	return Event(e)
}

// BranchKey returns a unique key for branch activity tracking.
func BranchKey(e Event) string {
	return e.BranchKey()
}

// RefBranch extracts branch or tag name from a git ref.
func RefBranch(ref string) string {
	const heads = "refs/heads/"
	const tags = "refs/tags/"
	switch {
	case len(ref) > len(heads) && ref[:len(heads)] == heads:
		return ref[len(heads):]
	case len(ref) > len(tags) && ref[:len(tags)] == tags:
		return ref[len(tags):]
	default:
		return ref
	}
}

// IsPushLike returns true for push types that can trigger branch wars.
func IsPushLike(e Event) bool {
	return e.IsPushLike()
}
