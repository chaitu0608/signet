package chain

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"server/internal/store"
)

// StartWorkers runs anchor batcher and SBT minter when chain is configured.
func StartWorkers(ctx context.Context, cfg Config, st store.Store, cli *Client) {
	if !cfg.Enabled() || cli == nil {
		slog.Info("chain workers disabled")
		return
	}
	anchorSec := envInt("ANCHOR_INTERVAL_SEC", 60)
	sbtSec := envInt("SBT_INTERVAL_SEC", 300)

	go runAnchorBatcher(ctx, st, cli, time.Duration(anchorSec)*time.Second)
	go runSBTMinter(ctx, st, cli, time.Duration(sbtSec)*time.Second)
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func runAnchorBatcher(ctx context.Context, st store.Store, cli *Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var localBatch uint64 = 1

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events, err := st.PendingForAnchor(ctx, 256)
			if err != nil || len(events) == 0 {
				continue
			}
			leaves := make([]string, 0, len(events))
			for _, e := range events {
				if e.LeafHash != "" {
					leaves = append(leaves, e.LeafHash)
				}
			}
			if len(leaves) == 0 {
				continue
			}
			rootHex, proofs := BuildMerkleTree(leaves)
			var root [32]byte
			copy(root[:], common.HexToHash(rootHex).Bytes())

			txHash, err := cli.AnchorRoot(ctx, root, uint32(len(leaves)))
			if err != nil {
				slog.Warn("anchor tx failed", "err", err)
				continue
			}

			batchID, _ := cli.BatchCount(ctx)
			if batchID > 0 {
				batchID--
			} else {
				batchID = localBatch
				localBatch++
			}

			switch pg := st.(type) {
			case *store.Postgres:
				_ = pg.SaveBatchAnchor(ctx, batchID, txHash, rootHex, leaves, proofs)
			case *store.Memory:
				_ = pg.AnchorBatchInMemory(batchID, txHash, leaves, rootHex, proofs)
			default:
				_ = st.MarkAnchored(ctx, batchID, txHash, leaves)
				_ = st.SaveBatchRoot(ctx, batchID, rootHex)
			}

			slog.Info("anchored batch", "batch_id", batchID, "tx", txHash, "leaves", len(leaves))

			if cli.SchemaUID != (common.Hash{}) {
				for _, e := range events {
					if e.Signer == "" || e.LeafHash == "" {
						continue
					}
					if e.AttestUID != "" {
						continue
					}
					if e.QualityScore == 0 {
						e.QualityScore = 50
					}
					uid, attestTx, err := cli.Attest(ctx, e)
					if err != nil {
						slog.Warn("EAS attest failed", "event", e.ID, "err", err)
						continue
					}
					e.AttestTx = attestTx
					if uid != (common.Hash{}) {
						e.AttestUID = uid.Hex()
					}
					_ = st.UpdateEvent(ctx, e)
					slog.Info("EAS attested", "event", e.ID, "uid", e.AttestUID, "tx", attestTx)
				}
			}

			for _, e := range events {
				if e.Signer != "" {
					_ = st.QueueSBT(ctx, store.SBTQueueItem{
						Address:  e.Signer,
						LeafHash: e.LeafHash,
						EventID:  e.ID,
					})
				}
			}
		}
	}
}

func runSBTMinter(ctx context.Context, st store.Store, cli *Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			items, err := st.PendingSBT(ctx, 50)
			if err != nil || len(items) == 0 {
				continue
			}
			devs := make([]common.Address, 0, len(items))
			hashes := make([][32]byte, 0, len(items))
			eventIDs := make([]string, 0, len(items))
			for _, item := range items {
				devs = append(devs, common.HexToAddress(item.Address))
				var h [32]byte
				copy(h[:], common.HexToHash(item.LeafHash).Bytes())
				hashes = append(hashes, h)
				eventIDs = append(eventIDs, item.EventID)
			}
			txHash, err := cli.MintSBTBatch(ctx, devs, hashes)
			if err != nil {
				slog.Warn("sbt mint failed", "err", err)
				continue
			}
			for _, id := range eventIDs {
				_ = st.MarkSBTMinted(ctx, id, txHash)
			}
			slog.Info("minted sbt batch", "count", len(items), "tx", txHash)
		}
	}
}
