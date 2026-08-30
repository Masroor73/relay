# Relay

**A self-hostable webhook reliability gateway.**

Webhook integrations fail silently and often: providers retry on timeout, networks drop mid-delivery, downstream services error transiently - and without a reliability layer in front of them, that means lost events or duplicated side effects. Relay sits between a webhook provider (e.g. Stripe) and your application, guaranteeing every event is durably received, deduplicated, retried on failure, and recoverable if it can't be processed - with full visibility into every event's lifecycle.

**Live deployment:**
- Dashboard: https://relay-dashboard-202922425380.us-east4.run.app
- API: https://relay-server-202922425380.us-east4.run.app

---

## Table of Contents

- [Architecture & System Design](#architecture--system-design)
- [Key Features & Reliability Guarantees](#key-features--reliability-guarantees)
- [Tech Stack & Ecosystem](#tech-stack--ecosystem)
- [Getting Started](#getting-started)
- [API Documentation](#api-documentation)
- [MCP Server](#mcp-server)
- [Observability & Logging](#observability--logging)
- [Testing](#testing)
- [CI/CD](#cicd)
- [Roadmap & Known Limitations](#roadmap--known-limitations)
- [License](#license)

---

## Architecture & System Design

```
Provider (Stripe)
      │  HTTPS POST + HMAC-SHA256 signature
      ▼
cmd/server (Go) → verify → dedupe → transactional write
      │
      ▼
Postgres: events · outbox · dead_letter_events · orders
      │
      ▼
cmd/processor (Go) → downstream effect → retry w/ backoff → DLQ on exhaustion
      │
      ▼
apps/dashboard (React) ◄──► apps/mcp-server (TypeScript)
      (both read cmd/server's /api/* endpoints)
```

Full design rationale, data model, and component tradeoffs are documented in [`ARCHITECTURE.md`](./ARCHITECTURE.md). The reasoning behind every major decision - including what was deliberately rejected - is documented in [`docs/DECISIONS.md`](./docs/DECISIONS.md).

## Key Features & Reliability Guarantees

- **Idempotent ingestion** - duplicate provider retries are safely absorbed via a DB-level `UNIQUE` constraint; no duplicate downstream effects.
- **Transactional outbox** - an event is never acknowledged and then silently lost before processing begins.
- **Exponential backoff retry** - transient downstream failures are retried on a fixed schedule (1s, 5s, 30s, 2m, 10m) rather than immediately or not at all.
- **Dead-letter recovery** - events that exhaust all 6 attempts are isolated, inspectable, and replayable rather than lost.
- **Live dashboard** - view of event status, processed orders, and dead-lettered events (fetches once on unlock; manual refresh to see new data - see [Known Limitations](#roadmap--known-limitations)).
- **MCP server** - dead-letter listing, inspection, and replay exposed as tools for agent-driven or programmatic operations.
- **Deployed and verified in production** - running on Google Cloud Run against a Neon Postgres instance, with dedup, retry/DLQ, and MCP-driven replay all confirmed against the live deployment, not just localhost.

## Tech Stack & Ecosystem

| Layer | Technology |
|---|---|
| Ingestion & Processing | Go 1.26.7 |
| Database | PostgreSQL (Docker locally, Neon in staging/production) |
| Dashboard | React, TypeScript, Vite |
| MCP Server | Node.js, TypeScript, official `@modelcontextprotocol/sdk` |
| Package manager (Node apps) | pnpm |
| Deployment | Docker, Google Cloud Run (server as a standard service, processor as a Worker Pool) |
| CI | GitHub Actions, with Trivy container vulnerability scanning |

## Getting Started

### Prerequisites
```
Go >= 1.26.7
Node.js >= 20.x
pnpm (for apps/dashboard and apps/mcp-server)
Docker (for local Postgres via Docker Compose)
A Neon account (staging/production only - not required for local dev)
```

### Installation
```bash
git clone https://github.com/Masroor73/relay.git
cd relay
```

### Environment Setup
Create a `.env` file at the project root (see `.env.example`):
```bash
DATABASE_URL=your_postgres_connection_string
STRIPE_WEBHOOK_SECRET=your_stripe_signing_secret
DASHBOARD_PASSWORD=your_shared_dashboard_password
DASHBOARD_ORIGIN=http://localhost:5173
```

### Running Locally

**1. Start local Postgres** (waits for healthcheck before returning):
```bash
docker compose up -d
```

**2. Run database migrations** - reads `DATABASE_URL`, works unchanged against Docker or Neon:
```bash
cd apps/ingestion
go run ./cmd/migrate
```

**3. (Optional) Seed fake events/orders for local testing:**
```bash
go run ./cmd/seed
```

**4. Start the ingestion server and processor** (two separate binaries, run in separate terminals):
```bash
go run ./cmd/server
go run ./cmd/processor
```

**5. Start the dashboard:**
```bash
cd apps/dashboard
pnpm install
pnpm run dev
```

**6. (Optional) Start the MCP server:**
```bash
cd apps/mcp-server
pnpm install
pnpm run build
pnpm run start
```
Or run it via the [MCP Inspector](https://github.com/modelcontextprotocol/inspector) for interactive testing:
```bash
pnpm dlx @modelcontextprotocol/inspector -e API_BASE_URL=http://localhost:8080 -e DASHBOARD_PASSWORD=your_dashboard_password -- node dist/index.js
```

## API Documentation

All routes are served by `cmd/server`. Base URL in production: `https://relay-server-202922425380.us-east4.run.app`.

### `GET /health`
Liveness endpoint for Cloud Run. Returns `200 OK` with no body when the service is ready to accept traffic.

### `POST /webhooks/stripe`
Accepts inbound Stripe webhook events.

**Headers:**
| Header | Required | Description |
|---|---|---|
| `Stripe-Signature` | Yes | HMAC-SHA256 signature used to verify payload authenticity |

**Responses:**
| Status | Meaning |
|---|---|
| `200 OK` | Event accepted (new or duplicate - deduplication is silent) |
| `401 Unauthorized` | Signature verification failed |

### Dashboard/API routes (require HTTP Basic Auth - any username, password must match `DASHBOARD_PASSWORD`)

| Route | Description |
|---|---|
| `GET /api/events` | Lists ingested webhook events |
| `GET /api/orders` | Lists processed orders |
| `GET /api/dlq` | Lists dead-lettered events |
| `GET /api/dlq/{id}` | Returns full detail for one dead-lettered event by `dlq_id` |
| `POST /api/dlq/{id}/replay` | Re-injects a dead-lettered event back into the outbox for reprocessing |

## MCP Server

`apps/mcp-server` exposes the DLQ surface as MCP tools over stdio, backed by the same `/api/*` contract the dashboard uses - no separate query layer.

| Tool | Input | Description |
|---|---|---|
| `list_dlq_events` | *(none)* | Lists all dead-lettered events |
| `inspect_dlq_event` | `dlq_id` (UUID) | Returns full detail for one dead-lettered event, including the error that caused it to be dead-lettered |
| `replay_dlq_event` | `dlq_id` (UUID) | Re-injects a dead-lettered event back into the processing pipeline. Marked `destructiveHint: true`, `idempotentHint: false` - a second replay of the same `dlq_id` correctly fails, since the row no longer exists after a successful replay. |

Point `API_BASE_URL` at either `http://localhost:8080` (local) or the deployed server URL above to run tools against production.

## Observability & Logging

- Structured JSON logs emitted from both `cmd/server` and `cmd/processor`, including event and idempotency-key fields for cross-service correlation.
- Dashboard provides a queryable view of event status, processed orders, and dead-letter reasons (fetch-on-unlock, not live-polling).
- Chaos-testing tooling (see [Testing](#testing)) doubles as a mechanism for observing retry/backoff and dead-letter behavior live, either locally or against the deployed instance.
- In production, `cmd/processor` runs as a Cloud Run Worker Pool (not a standard HTTP service, since it has no HTTP surface); its logs are readable via `gcloud beta run worker-pools logs read relay-processor --region=us-east4`.

## Testing

A chaos-test CLI (`tools/chaos-test/`) exercises the reliability guarantees directly against any running instance, local or deployed:

```bash
cd tools/chaos-test
./chaos-test -url <server-url> -secret <stripe-webhook-secret> -mode dedup    # fires an identical event twice
./chaos-test -url <server-url> -secret <stripe-webhook-secret> -mode failure  # fires an unparseable event; full retry→DLQ cycle takes ~13 minutes
```

Both modes have been run against the live deployed instance (not just localhost), with results confirmed via the processor's logs, the `/api/dlq` endpoint, the dashboard's DLQ view, and - for the failure scenario - a full MCP-driven `replay_dlq_event` round trip against production.

## CI/CD

GitHub Actions runs four required jobs on every PR:
- `build-lint-test` - Go build, lint, migrate, test for `apps/ingestion`
- `dashboard-build-lint` - lint and type-checked build for `apps/dashboard`
- `mcp-server-build-lint-test` - lint, build, and integration tests for `apps/mcp-server`, run against a real compiled-and-running `cmd/server` instance (not mocked)
- `trivy-scan` - container vulnerability scanning across all three Docker images, pinned to a specific commit SHA rather than a mutable tag

## Roadmap & Known Limitations

**Scaling:**
- Replace the Postgres-backed outbox poller with a dedicated event broker if throughput requirements exceed what a single-node outbox can sustain.
- Horizontal scaling of the processor with partition-aware consumption for ordering guarantees.
- Redis-backed idempotency cache, adopted only if measured lookup latency justifies it.

**Known limitations at current scale:**
- Rate limiting is in-memory and scoped per-instance, not global - under horizontal scaling this would need to move to a shared store (e.g. Redis).
- Dashboard auth is a single shared password, appropriate for a single-operator deployment - a real multi-tenant deployment would need proper user accounts and authorization.
- Dashboard fetches data once on unlock and does not auto-poll; new data requires a manual refresh.
- Local development uses Docker Postgres (`postgres:18-alpine`) while staging/production use Neon; both are addressed through the same `DATABASE_URL` contract with no code branching, but schema drift between environments is a manual discipline, not yet automated.

## License

MIT