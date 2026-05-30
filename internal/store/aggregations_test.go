package store

import (
	"context"
	"testing"
	"time"

	"server/internal/domain"
)

func seedMemory(m *Memory) {
	now := time.Now().UTC()
	addr := "0xabc0000000000000000000000000000000000001"
	events := []domain.Event{
		{ID: "1", TS: now.Add(-1 * time.Hour), Type: domain.TypePush, Repo: "alice/foo", Branch: "main", Signer: addr, AttestUID: "0xuid1", QualityScore: 80, Category: "feature"},
		{ID: "2", TS: now.Add(-2 * time.Hour), Type: domain.TypePush, Repo: "alice/foo", Branch: "main", Signer: addr, AttestUID: "0xuid2", QualityScore: 60, Category: "fix"},
		{ID: "3", TS: now.Add(-3 * time.Hour), Type: domain.TypePush, Repo: "alice/bar", Branch: "main", Signer: addr, AttestUID: "0xuid3", QualityScore: 50, Category: "feature"},
		{ID: "4", TS: now, Type: domain.TypePush, Repo: "alice/foo", Branch: "main", Signer: addr, QualityScore: 100, Category: "fix"},
	}
	for _, e := range events {
		_ = m.Insert(context.Background(), e)
	}
}

func TestMemoryDevProfile(t *testing.T) {
	m := NewMemory()
	seedMemory(m)
	prof, err := m.DevProfile(context.Background(), "0xabc0000000000000000000000000000000000001", 100)
	if err != nil {
		t.Fatal(err)
	}
	if prof.CommitCount != 3 {
		t.Fatalf("want 3 attested commits, got %d", prof.CommitCount)
	}
	if prof.TotalScore != 190 {
		t.Fatalf("want total 190, got %d", prof.TotalScore)
	}
	if len(prof.TopRepos) != 2 {
		t.Fatalf("want 2 repos, got %d", len(prof.TopRepos))
	}
	if len(prof.Sparkline) != 30 {
		t.Fatalf("want 30-day sparkline, got %d", len(prof.Sparkline))
	}
}

func TestMemoryLeaderboard(t *testing.T) {
	m := NewMemory()
	seedMemory(m)
	now := time.Now().UTC()
	other := "0xdef0000000000000000000000000000000000002"
	_ = m.Insert(context.Background(), domain.Event{
		ID: "x", TS: now, Type: domain.TypePush, Signer: other,
		AttestUID: "0xuidX", QualityScore: 200,
	})

	lb, err := m.Leaderboard(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lb) != 2 {
		t.Fatalf("want 2 entries, got %d", len(lb))
	}
	if lb[0].Address != other {
		t.Fatalf("ordering wrong: %+v", lb)
	}
	if lb[0].WeeklyDelta != 200 {
		t.Fatalf("weekly delta wrong: %d", lb[0].WeeklyDelta)
	}
}

func TestMemoryTimeseries(t *testing.T) {
	m := NewMemory()
	seedMemory(m)
	buckets, err := m.Timeseries(context.Background(), "0xabc0000000000000000000000000000000000001", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 7 {
		t.Fatalf("want 7 buckets, got %d", len(buckets))
	}
	total := 0
	for _, b := range buckets {
		total += b.Score
	}
	if total != 190 {
		t.Fatalf("timeseries total wrong: %d", total)
	}
}

func TestMemoryCategoryMix(t *testing.T) {
	m := NewMemory()
	seedMemory(m)
	mix, err := m.CategoryMix(context.Background(), "0xabc0000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if len(mix) == 0 {
		t.Fatal("expected category mix")
	}
}

func TestMemoryRecentAttested(t *testing.T) {
	m := NewMemory()
	seedMemory(m)
	recent, err := m.RecentAttested(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 3 {
		t.Fatalf("want 3 attested, got %d", len(recent))
	}
}

func TestMemoryDevProfile_AddressCaseInsensitive(t *testing.T) {
	m := NewMemory()
	seedMemory(m)
	prof, err := m.DevProfile(context.Background(), "0xABC0000000000000000000000000000000000001", 100)
	if err != nil {
		t.Fatal(err)
	}
	if prof.CommitCount == 0 {
		t.Fatal("address lookup must be case-insensitive")
	}
}

func TestBuildLeaderboardEnriched(t *testing.T) {
	now := time.Now().UTC()
	addr := "0xabc0000000000000000000000000000000000001"
	events := []domain.Event{
		{ID: "a", TS: now.Add(-24 * time.Hour), Signer: addr, AttestUID: "0x1", QualityScore: 50, Category: "fix"},
		{ID: "b", TS: now.Add(-48 * time.Hour), Signer: addr, AttestUID: "0x2", QualityScore: 30, Category: "feature"},
	}
	lb := buildLeaderboardEnriched(events, 10)
	if len(lb) != 1 {
		t.Fatalf("want 1 entry, got %d", len(lb))
	}
	if lb[0].WeeklyDelta != 80 {
		t.Fatalf("weekly delta: got %d want 80", lb[0].WeeklyDelta)
	}
	if len(lb[0].CategoryMix) != 2 {
		t.Fatalf("category mix: got %d", len(lb[0].CategoryMix))
	}
}
