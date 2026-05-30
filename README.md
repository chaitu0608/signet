# Signet — Proof of Code

**GitHub stars don't pay rent. Your commits should.**

Every signed git push is **AI-reviewed**, **Merkle-anchored**, and posted as a **canonical EAS attestation** on Base Sepolia — recipient = your wallet. View your portable reputation at `/dev/0xYou` or embed a live Signet badge anywhere.

[![Deploy with Vercel](https://vercel.com/button)](https://vercel.com/new/clone?repository-url=https%3A%2F%2Fgithub.com%2Fchaitu0608%2Fsignet&project-name=signet-proof-of-code&env=SIGNET_DEV_SEED&envDescription=Seed%20demo%20attested%20events%20on%20boot&envValue=1)

| Resource | Link |
|----------|------|
| **Full technical guide** | **[SIGNET_GUIDE.md](./SIGNET_GUIDE.md)** — architecture, code walkthrough, mermaid diagrams, roadmap |
| GitHub | https://github.com/chaitu0608/signet |

## Live demo (Vercel)

After deploying (one-click button above, or `vercel deploy --prod`):

| URL | What |
|-----|------|
| `/dev` | Signet home — live ticker + how to verify |
| `/dev/leaderboard` | Top developers by attested rep |
| `/dev/0xabc0000000000000000000000000000000000001` | Demo profile (seeded data) |
| `/embed/0xAddress?v=standard` | Embeddable badge iframe |

> **Note:** WebSockets (`/live`) are disabled on Vercel serverless; the UI polls `/api/dev/recent` instead. On-chain workers require a long-running host (Railway, Fly, Docker).

## Quick start (local)

```bash
go mod tidy
go run ./cmd/server
```

Optional demo data:

```bash
SIGNET_DEV_SEED=1 go run ./cmd/server
```

| URL | What |
|-----|------|
| `/dev` | Signet home — live ticker + how to verify |
| `/dev/leaderboard` | Top developers by attested rep |
| `/dev/0xAddress` | Public dev profile + EAS links |
| `/embed/0xAddress?v=standard` | Embeddable badge iframe |

### Embed variants

```html
<!-- compact 200×60 -->
<iframe src="https://your-host/embed/0xYou?v=compact" width="200" height="60" style="border:none" title="Signet"></iframe>

<!-- standard 320×80 (default) -->
<iframe src="https://your-host/embed/0xYou?v=standard" width="320" height="80" style="border:none" title="Signet"></iframe>

<!-- detailed 440×140 with sparkline -->
<iframe src="https://your-host/embed/0xYou?v=detailed" width="440" height="140" style="border:none" title="Signet"></iframe>
```

### EAS schema (one-time)

```bash
cd contracts
forge script script/RegisterSchema.s.sol --rpc-url $BASE_SEPOLIA_RPC --broadcast
```

Copy `EAS_SCHEMA_UID` into `.env`.

### Sign your pushes

```bash
cd scripts && npm install
export FORGEPULSE_PRIVATE_KEY=0x...
node sign-push.mjs /path/to/repo refs/heads/main 0000... abc123...
```

## Environment

| Variable | Description |
|----------|-------------|
| `PORT` | HTTP port (default `8080`) |
| `OPENAI_API_KEY` | AI reviewer (optional; heuristic fallback) |
| `RPC_URL` | Base Sepolia RPC |
| `RELAYER_PRIVATE_KEY` | Relayer for anchor + EAS attest |
| `ANCHOR_ADDR` | ForgePulseAnchor contract |
| `EAS_SCHEMA_UID` | Registered Signet schema UID |
| `EAS_ADDR` | Default `0x4200…0021` (canonical on Base) |
| `EAS_SCHEMA_REGISTRY` | Default `0x4200…0020` |
| `SIGNET_DEV_SEED` | Set to `1` to inject demo attested events |
| `DATABASE_URL` | Postgres (optional) |
| `REDIS_URL` | Redis pub/sub (optional) |
| `VERCEL` | Set automatically on Vercel; enables demo seed + disables chain workers |

## Deploy to Vercel

```bash
npm i -g vercel
vercel login
vercel deploy --prod
```

Or import from GitHub: [vercel.com/new/clone → signet-proof-of-code](https://vercel.com/new/clone?repository-url=https%3A%2F%2Fgithub.com%2Fchaitu0608%2Fsignet&project-name=signet-proof-of-code)

> If Vercel says the name is taken, use **`signet-proof-of-code`** (or any unique name like `signet-chaitu0608`) — the product name stays Signet.

## Verification

```bash
go run ./cmd/verify
```

See **[DEMO.md](DEMO.md)** for the 30-second hackathon script.

## API

| Route | Description |
|-------|-------------|
| `POST /hook` | Ingest signed git events |
| `GET /live` | WebSocket stream |
| `GET /api/dev/{address}` | Dev profile JSON (sparkline, category mix, weekly delta) |
| `GET /api/dev/{address}/timeseries?days=30` | Rep per day |
| `GET /api/dev/{address}/og.svg` | OG share card (1200×630) |
| `GET /api/dev/recent?limit=20` | Recent attestations ticker |
| `GET /api/dev/leaderboard` | Top 50 by rep |
| `GET /api/dev/{address}/badge.svg` | SVG badge |
| `GET /embed/{address}?v=compact\|standard\|detailed` | Embeddable HTML badge |

<details>
<summary>Legacy ForgePulse v3/v4 (shelved)</summary>

War room, SBTs, and paymaster routes remain in the codebase but are not part of the MVP demo path. `/web3` redirects to `/dev/leaderboard`.

```bash
docker compose up --build
forge script script/Deploy.s.sol --rpc-url $BASE_SEPOLIA_RPC --broadcast
```

</details>

## Stillroom

Meditation app preserved at `/zendo` and `/still`.
