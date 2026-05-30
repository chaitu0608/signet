package reviewer

import (
	"context"
	"testing"

	"server/internal/domain"
)

func TestHeuristicReview(t *testing.T) {
	r := New("")
	rev, err := r.Review(context.Background(), domain.Event{
		Type: domain.TypePush,
		Repo: "/tmp",
		CommitsDetail: []domain.CommitInfo{
			{Message: "add tests for auth"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rev.Score < 0 || rev.Score > 100 {
		t.Fatalf("score out of range: %d", rev.Score)
	}
	if rev.Summary == "" {
		t.Fatal("expected summary")
	}
	if rev.Category == "" {
		t.Fatal("expected category")
	}
}
