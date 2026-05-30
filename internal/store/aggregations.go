package store

import (
	"sort"
	"strings"
	"time"

	"server/internal/domain"
)

// DayBucket aggregates rep for a single calendar day.
type DayBucket struct {
	Day   string `json:"day"`
	Score int    `json:"score"`
	Count int    `json:"count"`
}

// CategoryCount tallies commits per category.
type CategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

func attestedFor(events []domain.Event, address string) []domain.Event {
	out := make([]domain.Event, 0, len(events))
	for _, e := range events {
		if !isAttested(e) {
			continue
		}
		if address != "" && !strings.EqualFold(e.Signer, address) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func buildTimeseries(events []domain.Event, address string, days int) []DayBucket {
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -(days - 1)).Truncate(24 * time.Hour)
	buckets := make(map[string]*DayBucket)

	for _, e := range attestedFor(events, address) {
		if e.TS.Before(cutoff) {
			continue
		}
		day := e.TS.UTC().Format("2006-01-02")
		b, ok := buckets[day]
		if !ok {
			b = &DayBucket{Day: day}
			buckets[day] = b
		}
		b.Score += e.QualityScore
		b.Count++
	}

	out := make([]DayBucket, 0, days)
	for i := days - 1; i >= 0; i-- {
		day := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		if b, ok := buckets[day]; ok {
			out = append(out, *b)
		} else {
			out = append(out, DayBucket{Day: day})
		}
	}
	return out
}

func buildCategoryMix(events []domain.Event, address string) []CategoryCount {
	counts := make(map[string]int)
	for _, e := range attestedFor(events, address) {
		cat := e.Category
		if cat == "" {
			cat = "chore"
		}
		counts[cat]++
	}
	out := make([]CategoryCount, 0, len(counts))
	for cat, n := range counts {
		out = append(out, CategoryCount{Category: cat, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

func weeklyDelta(events []domain.Event, address string) int {
	cutoff := time.Now().UTC().AddDate(0, 0, -7)
	total := 0
	for _, e := range attestedFor(events, address) {
		if e.TS.After(cutoff) {
			total += e.QualityScore
		}
	}
	return total
}

func securityFlagCount(events []domain.Event, address string) int {
	n := 0
	for _, e := range attestedFor(events, address) {
		n += len(e.SecurityFlags)
	}
	return n
}

func buildRecentAttested(events []domain.Event, limit int) []domain.Event {
	var attested []domain.Event
	for i := len(events) - 1; i >= 0; i-- {
		if isAttested(events[i]) {
			attested = append(attested, events[i])
			if len(attested) >= limit {
				break
			}
		}
	}
	return attested
}

func enrichDevProfile(prof *DevProfile, all []domain.Event) {
	if prof == nil {
		return
	}
	prof.WeeklyDelta = weeklyDelta(all, prof.Address)
	prof.CategoryMix = buildCategoryMix(all, prof.Address)
	prof.Sparkline = buildTimeseries(all, prof.Address, 30)
	prof.SecurityFlagCount = securityFlagCount(all, prof.Address)
}

func buildLeaderboardEnriched(events []domain.Event, limit int) []LeaderboardEntry {
	base := buildLeaderboard(events, limit)
	if len(base) == 0 {
		return base
	}
	for i := range base {
		base[i].WeeklyDelta = weeklyDelta(events, base[i].Address)
		base[i].CategoryMix = buildCategoryMix(events, base[i].Address)
	}
	return base
}
