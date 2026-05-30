package reviewer

import (
	"context"

	"server/internal/domain"
)

// Review is an AI/heuristic code review result.
type Review struct {
	Score    int      `json:"score"`
	Summary  string   `json:"summary"`
	Security []string `json:"security"`
	Tests    bool     `json:"adds_tests"`
	Category string   `json:"category"`
}

// Reviewer scores git push events.
type Reviewer interface {
	Review(ctx context.Context, e domain.Event) (Review, error)
}

// New creates a reviewer (OpenAI if key set, else heuristic).
func New(openAIKey string) Reviewer {
	if openAIKey != "" {
		return &openAI{apiKey: openAIKey}
	}
	return &heuristic{}
}
