package store

import (
	"sort"
	"strings"

	"server/internal/domain"
)

// isAttested returns true once an EAS attestation UID has been issued.
func isAttested(e domain.Event) bool {
	return e.AttestUID != ""
}

// buildDevProfile aggregates a developer's attested events into a profile.
// Address comparison is case-insensitive (EIP-55 vs lower-cased forms).
func buildDevProfile(address string, events []domain.Event) *DevProfile {
	repoScores := make(map[string]*RepoScore)
	attestations := make([]domain.Event, 0, len(events))
	total := 0

	for _, e := range events {
		if !isAttested(e) || !strings.EqualFold(e.Signer, address) {
			continue
		}
		attestations = append(attestations, e)
		total += e.QualityScore

		rs, ok := repoScores[e.Repo]
		if !ok {
			rs = &RepoScore{Repo: e.Repo}
			repoScores[e.Repo] = rs
		}
		rs.Score += e.QualityScore
		rs.Count++
	}

	top := make([]RepoScore, 0, len(repoScores))
	for _, rs := range repoScores {
		top = append(top, *rs)
	}
	sort.Slice(top, func(i, j int) bool { return top[i].Score > top[j].Score })

	return &DevProfile{
		Address:      address,
		TotalScore:   total,
		CommitCount:  len(attestations),
		TopRepos:     top,
		Attestations: attestations,
	}
}

// buildLeaderboard ranks signers by total quality score across all attested events.
func buildLeaderboard(events []domain.Event, limit int) []LeaderboardEntry {
	type agg struct {
		score int
		count int
	}

	byAddr := make(map[string]*agg)
	for _, e := range events {
		if !isAttested(e) || e.Signer == "" {
			continue
		}
		key := strings.ToLower(e.Signer)
		a, ok := byAddr[key]
		if !ok {
			a = &agg{}
			byAddr[key] = a
		}
		a.score += e.QualityScore
		a.count++
	}

	out := make([]LeaderboardEntry, 0, len(byAddr))
	for addr, a := range byAddr {
		out = append(out, LeaderboardEntry{
			Address:     addr,
			TotalScore:  a.score,
			CommitCount: a.count,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalScore > out[j].TotalScore })

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
