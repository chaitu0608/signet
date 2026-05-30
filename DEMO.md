# Signet — Proof of Code

**30-second demo script for ETHGlobal judges**

## The pitch (15 seconds)

GitHub reputation is platform-locked vanity. **Signet** turns every **EIP-712 signed git push** into an **AI-graded EAS attestation** on **Base Sepolia**, issued to the developer's wallet. Reputation is portable — view it on a public profile or embed a live badge anywhere.

> *"Signet — proof of code."*

## Live demo (30 seconds)

1. **Open home** — `http://localhost:8080/dev`
   - Live attestations ticker
   - 3-step "how to get verified" strip

2. **Push signed code** (or use seeded data):
   ```bash
   SIGNET_DEV_SEED=1 go run ./cmd/server
   ```

3. **Open profile** — `/dev/0xabc0000000000000000000000000000000000001`
   - 30-day heatmap + sparkline
   - Repo breakdown + category mix
   - Click **Verify on-chain** on any attestation

4. **Leaderboard** — `/dev/leaderboard`
   - Search by address
   - 7d delta column + category mini-mix per dev

5. **Embed badge** — copy from profile page (compact / standard / detailed):
   ```html
   <iframe src="http://localhost:8080/embed/0xYou?v=standard" width="320" height="80" style="border:none" title="Signet"></iframe>
   ```

6. **Share card** — `/api/dev/0xYou/og.svg` (1200×630 OG image)

## Tech stack

| Layer | Tech |
|-------|------|
| Backend | Go, WebSocket, Postgres (optional) |
| AI review | OpenAI gpt-4o-mini (heuristic fallback) |
| Chain | Base Sepolia, EAS canonical, Merkle anchor |
| Auth | EIP-712 signed git pushes |
| Frontend | Vanilla JS, server-rendered SVG |

## Setup

```bash
go mod tidy
go run ./cmd/server
# optional demo data:
SIGNET_DEV_SEED=1 go run ./cmd/server
```

Register EAS schema once:

```bash
cd contracts
forge script script/RegisterSchema.s.sol --rpc-url $BASE_SEPOLIA_RPC --broadcast
```

## Verify everything works

```bash
go run ./cmd/verify
```

Expected: `SCORE: 10.0 / 10`

## What judges should look for

- **Canonical EAS** — not a custom attestation contract
- **Portable rep** — embed badge works on any site
- **AI grading** — every push gets a score + category
- **On-chain verify** — inline button calls EAS view on Base Sepolia
- **Dense UI** — heatmap, sparkline, category mix, weekly delta
