package payapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"server/internal/domain"
	"server/internal/store"
)

// AttestationEvidence is a single attested push in a reputation report.
type AttestationEvidence struct {
	EventID       string   `json:"event_id"`
	Repo          string   `json:"repo"`
	Branch        string   `json:"branch"`
	QualityScore  int      `json:"quality_score"`
	Category      string   `json:"category"`
	AttestUID     string   `json:"attest_uid,omitempty"`
	ReputationTx  string   `json:"reputation_tx,omitempty"`
	LeafHash      string   `json:"leaf_hash,omitempty"`
	BatchID       uint64   `json:"batch_id,omitempty"`
	SecurityFlags []string `json:"security_flags,omitempty"`
	TS            string   `json:"ts"`
}

// ReputationReport is the machine-readable oracle payload.
type ReputationReport struct {
	Address           string                `json:"address"`
	TotalScore        int                   `json:"total_score"`
	CommitCount       int                   `json:"commit_count"`
	WeeklyDelta       int                   `json:"weekly_delta"`
	SecurityFlagCount int                   `json:"security_flag_count"`
	TopRepos          []store.RepoScore     `json:"top_repos"`
	CategoryMix       []store.CategoryCount `json:"category_mix"`
	Attestations      []AttestationEvidence `json:"attestations,omitempty"`
	ReportHash        string                `json:"report_hash"`
	IssuedAt          string                `json:"issued_at"`
	Kind              string                `json:"kind"`
}

// SignedResponse wraps a report with oracle signature metadata.
type SignedResponse struct {
	Report    ReputationReport `json:"report"`
	Signature string           `json:"signature"`
	Signer    string           `json:"signer"`
	Schema    string           `json:"schema"`
}

// LeaderboardReport is a signed snapshot of top developers.
type LeaderboardReport struct {
	Entries   []store.LeaderboardEntry `json:"entries"`
	ReportHash string                  `json:"report_hash"`
	IssuedAt   string                  `json:"issued_at"`
	Kind       string                  `json:"kind"`
}

// BuildSummaryReport aggregates profile data into a summary reputation report.
func BuildSummaryReport(ctx context.Context, st store.Store, address string) (ReputationReport, error) {
	prof, err := st.DevProfile(ctx, address, 100)
	if err != nil {
		return ReputationReport{}, err
	}
	if prof == nil {
		prof = &store.DevProfile{Address: address}
	}

	delta, _ := st.WeeklyDelta(ctx, address)
	mix, _ := st.CategoryMix(ctx, address)
	if len(mix) > 0 {
		prof.CategoryMix = mix
	}
	prof.WeeklyDelta = delta

	secFlags := 0
	for _, e := range prof.Attestations {
		secFlags += len(e.SecurityFlags)
	}

	report := ReputationReport{
		Address:           prof.Address,
		TotalScore:        prof.TotalScore,
		CommitCount:       prof.CommitCount,
		WeeklyDelta:       prof.WeeklyDelta,
		SecurityFlagCount: secFlags,
		TopRepos:          limitRepos(prof.TopRepos, 5),
		CategoryMix:       prof.CategoryMix,
		IssuedAt:          time.Now().UTC().Format(time.RFC3339),
		Kind:              "summary",
	}
	report.ReportHash = hashReport(report)
	return report, nil
}

// BuildFullReport includes per-attestation evidence.
func BuildFullReport(ctx context.Context, st store.Store, address string) (ReputationReport, error) {
	report, err := BuildSummaryReport(ctx, st, address)
	if err != nil {
		return ReputationReport{}, err
	}

	prof, err := st.DevProfile(ctx, address, 200)
	if err != nil {
		return ReputationReport{}, err
	}
	if prof != nil {
		report.Attestations = mapAttestations(prof.Attestations)
	}
	report.Kind = "full"
	report.ReportHash = hashReport(report)
	return report, nil
}

// BuildLeaderboardReport returns a ranked snapshot.
func BuildLeaderboardReport(ctx context.Context, st store.Store, limit int) (LeaderboardReport, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	entries, err := st.Leaderboard(ctx, limit)
	if err != nil {
		return LeaderboardReport{}, err
	}
	report := LeaderboardReport{
		Entries:  entries,
		IssuedAt: time.Now().UTC().Format(time.RFC3339),
		Kind:     "leaderboard",
	}
	report.ReportHash = hashLeaderboard(report)
	return report, nil
}

func mapAttestations(events []domain.Event) []AttestationEvidence {
	out := make([]AttestationEvidence, 0, len(events))
	for _, e := range events {
		out = append(out, AttestationEvidence{
			EventID:       e.ID,
			Repo:          e.Repo,
			Branch:        e.Branch,
			QualityScore:  e.QualityScore,
			Category:      e.Category,
			AttestUID:     e.AttestUID,
			ReputationTx:  e.ReputationTx,
			LeafHash:      e.LeafHash,
			BatchID:       e.BatchID,
			SecurityFlags: e.SecurityFlags,
			TS:            e.TS.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func limitRepos(repos []store.RepoScore, n int) []store.RepoScore {
	if len(repos) <= n {
		return repos
	}
	return repos[:n]
}

func hashReport(r ReputationReport) string {
	payload := struct {
		Address     string `json:"address"`
		TotalScore  int    `json:"total_score"`
		CommitCount int    `json:"commit_count"`
		WeeklyDelta int    `json:"weekly_delta"`
		Kind        string `json:"kind"`
		IssuedAt    string `json:"issued_at"`
	}{
		Address:     strings.ToLower(r.Address),
		TotalScore:  r.TotalScore,
		CommitCount: r.CommitCount,
		WeeklyDelta: r.WeeklyDelta,
		Kind:        r.Kind,
		IssuedAt:    r.IssuedAt,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return "0x" + hex.EncodeToString(sum[:])
}

func hashLeaderboard(r LeaderboardReport) string {
	addrs := make([]string, 0, len(r.Entries))
	for _, e := range r.Entries {
		addrs = append(addrs, strings.ToLower(e.Address)+":"+strconv.Itoa(e.TotalScore))
	}
	sort.Strings(addrs)
	payload := strings.Join(addrs, "|") + "|" + r.IssuedAt
	sum := sha256.Sum256([]byte(payload))
	return "0x" + hex.EncodeToString(sum[:])
}
