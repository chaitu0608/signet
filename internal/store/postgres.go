package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"server/internal/domain"
)

// Postgres implements Store with PostgreSQL.
type Postgres struct {
	pool *pgxpool.Pool
}

// OpenPostgres connects and migrates schema.
func OpenPostgres(ctx context.Context, url string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	p := &Postgres{pool: pool}
	if err := p.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return p, nil
}

func (p *Postgres) migrate(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS events (
  id TEXT PRIMARY KEY,
  ts TIMESTAMPTZ NOT NULL,
  type TEXT NOT NULL,
  repo TEXT,
  branch TEXT,
  ref TEXT,
  pusher TEXT,
  signer TEXT,
  signature TEXT,
  leaf_hash TEXT,
  anchor_status TEXT DEFAULT 'pending',
  batch_id BIGINT DEFAULT 0,
  anchor_tx TEXT,
  payload JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_anchor ON events(anchor_status) WHERE anchor_status = 'pending';
CREATE INDEX IF NOT EXISTS idx_events_signer ON events(signer);

CREATE TABLE IF NOT EXISTS batch_roots (
  batch_id BIGINT PRIMARY KEY,
  root TEXT NOT NULL,
  anchor_tx TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS merkle_proofs (
  leaf_hash TEXT PRIMARY KEY,
  batch_id BIGINT NOT NULL,
  proof JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS bounties (
  id SERIAL PRIMARY KEY,
  issue_hash TEXT NOT NULL,
  funder TEXT NOT NULL,
  amount_wei TEXT NOT NULL,
  token TEXT DEFAULT 'ETH',
  status TEXT DEFAULT 'active',
  claim_tx TEXT,
  on_chain_id BIGINT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sbt_queue (
  id SERIAL PRIMARY KEY,
  address TEXT NOT NULL,
  leaf_hash TEXT NOT NULL,
  event_id TEXT NOT NULL UNIQUE,
  minted_tx TEXT
);
`)
	return err
}

func (p *Postgres) Close() {
	p.pool.Close()
}

func (p *Postgres) Insert(ctx context.Context, e domain.Event) error {
	if e.AnchorStatus == "" {
		e.AnchorStatus = domain.AnchorPending
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `
INSERT INTO events (id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (id) DO NOTHING`,
		e.ID, e.TS, e.Type, e.Repo, e.Branch, e.Ref, e.Pusher, e.Signer, e.Signature,
		e.LeafHash, e.AnchorStatus, e.BatchID, e.AnchorTx, payload)
	return err
}

func (p *Postgres) scanEvent(row interface{ Scan(...any) error }) (*domain.Event, error) {
	var payload []byte
	var e domain.Event
	err := row.Scan(&e.ID, &e.TS, &e.Type, &e.Repo, &e.Branch, &e.Ref, &e.Pusher,
		&e.Signer, &e.Signature, &e.LeafHash, &e.AnchorStatus, &e.BatchID, &e.AnchorTx, &payload)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(payload, &e)
	return &e, nil
}

func (p *Postgres) RecentEvents(ctx context.Context, limit int) ([]domain.Event, error) {
	rows, err := p.pool.Query(ctx, `
SELECT id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload
FROM events ORDER BY ts DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		e, err := p.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (p *Postgres) PendingForAnchor(ctx context.Context, limit int) ([]domain.Event, error) {
	rows, err := p.pool.Query(ctx, `
SELECT id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload
FROM events WHERE anchor_status = 'pending' AND type != 'branch_war' AND leaf_hash != ''
ORDER BY ts ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		e, err := p.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (p *Postgres) MarkAnchored(ctx context.Context, batchID uint64, txHash string, leafHashes []string) error {
	_, err := p.pool.Exec(ctx, `
UPDATE events SET anchor_status = 'anchored', batch_id = $1, anchor_tx = $2
WHERE leaf_hash = ANY($3)`, batchID, txHash, leafHashes)
	return err
}

func (p *Postgres) GetByID(ctx context.Context, id string) (*domain.Event, error) {
	row := p.pool.QueryRow(ctx, `
SELECT id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload
FROM events WHERE id = $1`, id)
	return p.scanEvent(row)
}

func (p *Postgres) SaveBatchRoot(ctx context.Context, batchID uint64, root string) error {
	_, err := p.pool.Exec(ctx, `
INSERT INTO batch_roots (batch_id, root) VALUES ($1,$2)
ON CONFLICT (batch_id) DO UPDATE SET root = EXCLUDED.root`, batchID, root)
	return err
}

func (p *Postgres) GetBatchRoot(ctx context.Context, batchID uint64) (string, error) {
	var root string
	err := p.pool.QueryRow(ctx, `SELECT root FROM batch_roots WHERE batch_id = $1`, batchID).Scan(&root)
	return root, err
}

func (p *Postgres) GetProof(ctx context.Context, id string) (*Proof, error) {
	e, err := p.GetByID(ctx, id)
	if err != nil || e == nil || e.BatchID == 0 {
		return nil, err
	}
	root, _ := p.GetBatchRoot(ctx, e.BatchID)
	var proofJSON []byte
	err = p.pool.QueryRow(ctx, `SELECT proof FROM merkle_proofs WHERE leaf_hash = $1`, e.LeafHash).Scan(&proofJSON)
	if err != nil {
		return &Proof{EventID: e.ID, BatchID: e.BatchID, LeafHash: e.LeafHash, Root: root, AnchorTx: e.AnchorTx}, nil
	}
	var proof []string
	_ = json.Unmarshal(proofJSON, &proof)
	return &Proof{
		EventID:  e.ID,
		BatchID:  e.BatchID,
		LeafHash: e.LeafHash,
		Root:     root,
		Proof:    proof,
		AnchorTx: e.AnchorTx,
	}, nil
}

// SaveProofs stores merkle proofs for a batch.
func (p *Postgres) SaveProofs(ctx context.Context, batchID uint64, proofs map[string][]string) error {
	for leaf, proof := range proofs {
		b, _ := json.Marshal(proof)
		_, err := p.pool.Exec(ctx, `
INSERT INTO merkle_proofs (leaf_hash, batch_id, proof) VALUES ($1,$2,$3)
ON CONFLICT (leaf_hash) DO UPDATE SET proof = EXCLUDED.proof, batch_id = EXCLUDED.batch_id`,
			leaf, batchID, b)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) InsertBounty(ctx context.Context, b Bounty) error {
	_, err := p.pool.Exec(ctx, `
INSERT INTO bounties (issue_hash, funder, amount_wei, token, status, on_chain_id)
VALUES ($1,$2,$3,$4,$5,$6)`,
		b.IssueHash, b.Funder, b.AmountWei, b.Token, b.Status, b.OnChainID)
	return err
}

func (p *Postgres) ListBounties(ctx context.Context, status string, limit int) ([]Bounty, error) {
	if status != "" {
		return p.listBountiesQuery(ctx, `
SELECT id, issue_hash, funder, amount_wei, token, status, claim_tx, on_chain_id, created_at
FROM bounties WHERE status = $1 ORDER BY created_at DESC LIMIT $2`, status, limit)
	}
	return p.listBountiesQuery(ctx, `
SELECT id, issue_hash, funder, amount_wei, token, status, claim_tx, on_chain_id, created_at
FROM bounties ORDER BY created_at DESC LIMIT $1`, limit)
}

func (p *Postgres) listBountiesQuery(ctx context.Context, q string, args ...any) ([]Bounty, error) {
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bounty
	for rows.Next() {
		var b Bounty
		if err := rows.Scan(&b.ID, &b.IssueHash, &b.Funder, &b.AmountWei, &b.Token, &b.Status, &b.ClaimTx, &b.OnChainID, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (p *Postgres) UpdateBountyStatus(ctx context.Context, onChainID uint64, status, claimTx string) error {
	_, err := p.pool.Exec(ctx, `UPDATE bounties SET status = $1, claim_tx = $2 WHERE on_chain_id = $3`, status, claimTx, onChainID)
	return err
}

func (p *Postgres) QueueSBT(ctx context.Context, item SBTQueueItem) error {
	_, err := p.pool.Exec(ctx, `
INSERT INTO sbt_queue (address, leaf_hash, event_id) VALUES ($1,$2,$3)
ON CONFLICT (event_id) DO NOTHING`, item.Address, item.LeafHash, item.EventID)
	return err
}

func (p *Postgres) PendingSBT(ctx context.Context, limit int) ([]SBTQueueItem, error) {
	rows, err := p.pool.Query(ctx, `
SELECT address, leaf_hash, event_id, COALESCE(minted_tx,'') FROM sbt_queue
WHERE minted_tx IS NULL OR minted_tx = '' LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SBTQueueItem
	for rows.Next() {
		var item SBTQueueItem
		if err := rows.Scan(&item.Address, &item.LeafHash, &item.EventID, &item.MintedTx); err != nil {
			return nil, err
		}
		if item.MintedTx == "" {
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

func (p *Postgres) MarkSBTMinted(ctx context.Context, eventID, txHash string) error {
	_, err := p.pool.Exec(ctx, `UPDATE sbt_queue SET minted_tx = $1 WHERE event_id = $2`, txHash, eventID)
	return err
}

func (p *Postgres) UpdateEvent(ctx context.Context, e domain.Event) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `
UPDATE events SET anchor_status=$2, batch_id=$3, anchor_tx=$4, payload=$5
WHERE id=$1`,
		e.ID, e.AnchorStatus, e.BatchID, e.AnchorTx, payload)
	return err
}

func (p *Postgres) EventsByRepo(ctx context.Context, repo string, limit int) ([]domain.Event, error) {
	rows, err := p.pool.Query(ctx, `
SELECT id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload
FROM events WHERE repo = $1 ORDER BY ts DESC LIMIT $2`, repo, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		e, err := p.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (p *Postgres) EventsBySigner(ctx context.Context, signer string, limit int) ([]domain.Event, error) {
	rows, err := p.pool.Query(ctx, `
SELECT id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload
FROM events WHERE signer = $1 ORDER BY ts DESC LIMIT $2`, signer, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		e, err := p.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (p *Postgres) DevProfile(ctx context.Context, address string, limit int) (*DevProfile, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `
SELECT id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload
FROM events
WHERE lower(signer) = lower($1)
  AND COALESCE(payload->>'attest_uid','') <> ''
ORDER BY ts DESC LIMIT $2`, address, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.Event
	for rows.Next() {
		e, err := p.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	allForAddr, err := p.attestedEventsFor(ctx, address)
	if err != nil {
		return nil, err
	}
	prof := buildDevProfile(address, events)
	enrichDevProfile(prof, allForAddr)
	return prof, nil
}

func (p *Postgres) allAttestedEvents(ctx context.Context) ([]domain.Event, error) {
	rows, err := p.pool.Query(ctx, `
SELECT id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload
FROM events WHERE COALESCE(payload->>'attest_uid','') <> '' ORDER BY ts ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		e, err := p.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (p *Postgres) attestedEventsFor(ctx context.Context, address string) ([]domain.Event, error) {
	rows, err := p.pool.Query(ctx, `
SELECT id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload
FROM events
WHERE lower(signer) = lower($1) AND COALESCE(payload->>'attest_uid','') <> ''
ORDER BY ts ASC`, address)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		e, err := p.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (p *Postgres) Leaderboard(ctx context.Context, limit int) ([]LeaderboardEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	all, err := p.allAttestedEvents(ctx)
	if err != nil {
		return nil, err
	}
	return buildLeaderboardEnriched(all, limit), nil
}

func (p *Postgres) Timeseries(ctx context.Context, address string, days int) ([]DayBucket, error) {
	events, err := p.attestedEventsFor(ctx, address)
	if err != nil {
		return nil, err
	}
	return buildTimeseries(events, address, days), nil
}

func (p *Postgres) RecentAttested(ctx context.Context, limit int) ([]domain.Event, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := p.pool.Query(ctx, `
SELECT id, ts, type, repo, branch, ref, pusher, signer, signature, leaf_hash, anchor_status, batch_id, anchor_tx, payload
FROM events WHERE COALESCE(payload->>'attest_uid','') <> ''
ORDER BY ts DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		e, err := p.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (p *Postgres) WeeklyDelta(ctx context.Context, address string) (int, error) {
	events, err := p.attestedEventsFor(ctx, address)
	if err != nil {
		return 0, err
	}
	return weeklyDelta(events, address), nil
}

func (p *Postgres) CategoryMix(ctx context.Context, address string) ([]CategoryCount, error) {
	events, err := p.attestedEventsFor(ctx, address)
	if err != nil {
		return nil, err
	}
	return buildCategoryMix(events, address), nil
}

// OpenFromEnv opens Postgres if DATABASE_URL set, else memory store.
func OpenFromEnv(ctx context.Context) (Store, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return NewMemory(), nil
	}
	return OpenPostgres(ctx, url)
}

// SaveBatchAnchor completes anchor with proofs (postgres).
func (p *Postgres) SaveBatchAnchor(ctx context.Context, batchID uint64, txHash, root string, leafHashes []string, proofs map[string][]string) error {
	if err := p.SaveBatchRoot(ctx, batchID, root); err != nil {
		return err
	}
	if err := p.SaveProofs(ctx, batchID, proofs); err != nil {
		return err
	}
	_, err := p.pool.Exec(ctx, `UPDATE batch_roots SET anchor_tx = $1 WHERE batch_id = $2`, txHash, batchID)
	if err != nil {
		return fmt.Errorf("batch_roots tx: %w", err)
	}
	return p.MarkAnchored(ctx, batchID, txHash, leafHashes)
}
