package store

import (
	"context"
	"fmt"
	"os"
	"time"

	"server/internal/domain"
)

// MaybeSeed inserts demo attested events when SIGNET_DEV_SEED=1 or on Vercel.
func MaybeSeed(ctx context.Context, st Store) {
	if os.Getenv("SIGNET_DEV_SEED") != "1" && os.Getenv("VERCEL") != "1" {
		return
	}
	now := time.Now().UTC()
	addrs := []string{
		"0xabc0000000000000000000000000000000000001",
		"0xdef0000000000000000000000000000000000002",
		"0x1230000000000000000000000000000000000003",
	}
	repos := []string{"signet/core", "signet/cli", "acme/webapp", "acme/api"}
	cats := []string{"feature", "fix", "refactor", "docs", "test", "chore"}

	for i, addr := range addrs {
		for j := 0; j < 5; j++ {
			ts := now.Add(-time.Duration(j*2+i) * 24 * time.Hour)
			cat := cats[(i+j)%len(cats)]
			score := 60 + (i*10) + (j * 7)
			flags := []string{}
			if j == 2 && i == 0 {
				flags = []string{"suspicious: eval("}
			}
			_ = st.Insert(ctx, domain.Event{
				ID:             seedID(i, j),
				TS:             ts,
				Type:           domain.TypePush,
				Repo:           repos[(i+j)%len(repos)],
				Branch:         "main",
				Ref:            "refs/heads/main",
				NewSHA:         "abcd1234",
				Signer:         addr,
				QualityScore:   score,
				QualitySummary: "seed: " + cat + " work",
				SecurityFlags:  flags,
				Category:       cat,
				AttestUID:      "0xseed" + seedID(i, j),
				AttestTx:       "0xtx" + seedID(i, j),
				AnchorStatus:   domain.AnchorAnchored,
			})
		}
	}
}

func seedID(i, j int) string {
	return fmt.Sprintf("seed-%d-%d", i, j)
}
