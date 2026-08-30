# Relay - Technical Architecture & Design Specification

## 1. Overview

Relay is a webhook reliability gateway. It sits between an upstream event provider (e.g., Stripe) and downstream business logic, guaranteeing that every event is received durably, processed at-least-once, deduplicated safely, and recoverable if processing fails.

Webhook-based integrations are structurally unreliable by default: providers retry on timeout, networks drop connections mid-delivery, and downstream services fail transiently. Relay's job is to absorb that unreliability at the infrastructure layer so that business logic downstream can assume clean, exactly-once-effective delivery.

Relay is deployed and running in production on Google Cloud Run, backed by Neon Postgres. All reliability guarantees below have been verified against that live deployment, not only against local development (see Section 14).

## 2. Design Goals & Constraints

- **Durability over throughput.** At current scale, correctness (no lost or duplicated events) is prioritized over horizontal scalability. The architecture is designed to evolve toward higher throughput without a rewrite (see Section 8).
- **Minimal infrastructure surface.** Every component choice favors the smallest number of moving parts that still satisfies the durability guarantee, rather than the most scalable option available.
- **Observability by default.** Every event's lifecycle (received → processing → completed/dead-lettered) is queryable, both through a UI and programmatically.

## 3. System Architecture

```
Provider (Stripe)
      │  HTTPS POST + HMAC-SHA256 signature
      ▼
┌───────────────────┐
│  cmd/server (Go)   │  verify signature → dedupe check → transactional write
│                     │  also serves /api/* (dashboard reads + DLQ detail/replay, Section 12)
└─────────┬───────────┘
          │
          ▼
┌───────────────────┐
│     Postgres        │  events · outbox · dead_letter_events · orders
└─────────┬───────────┘
          │ poll due rows
          ▼
┌───────────────────┐
│ cmd/processor (Go) │  execute downstream effect → retry w/ backoff → DLQ on exhaustion
└─────────┬───────────┘
          │
          ▼
┌────────────────────┐      ┌─────────────────────┐
│ apps/dashboard (React) │  │ apps/mcp-server (TS)  │
└──────────┬─────────────┘  └──────────┬────────────┘
           │                            │
           └─────────────┬──────────────┘
                          ▼
              cmd/server's /api/* (shared data layer)
```

**Note on data access:** neither the dashboard nor the MCP server talks to Postgres directly - both call `cmd/server`'s `/api/*` endpoints (Section 12, Section 13), the same HTTP-facing service `/webhooks/stripe` already runs on, per Section 2's minimal-infrastructure-surface principle. The MCP server is a thin tool-calling layer over the identical API contract the dashboard uses, not a second implementation of the same queries.

## 4. Core Reliability Mechanisms

### 4.1 Idempotency Keys
Every inbound event is deduplicated on a key derived from the provider's event ID (e.g. Stripe's `evt_...`), falling back to a payload hash if no stable ID is present. A duplicate delivery is acknowledged (200 OK) without reprocessing. This is what makes provider-side retries safe by construction, rather than by convention.

**Concurrency note:** the dedup guarantee is enforced by a `UNIQUE` constraint on `events.idempotency_key` at the database layer, not by an application-level check-then-insert. Two simultaneous deliveries of the same event will both attempt the insert; the database allows exactly one to succeed and returns a constraint violation to the other, which the ingestion service catches and treats as a duplicate (200 OK, no reprocessing). This is deliberate - a `SELECT`-then-`INSERT` check in application code has a race window under concurrent delivery and would not actually hold the guarantee it claims to.

**Verified in production (ENG-56):** the `tools/chaos-test` dedup scenario, run against the live deployed instance, fired an identical event twice and confirmed exactly one row in `events` (via `/api/events`) and one corresponding order (via `/api/orders`), despite two `200 OK` HTTP responses - the deployed database enforces the same guarantee proven earlier in local integration tests, not a weaker one.

### 4.2 Transactional Outbox
On receipt, the raw event and its corresponding outbox entry are written to Postgres in a single transaction. This closes the gap between "event received" and "event queued for processing" - there is no window in which an event can be acknowledged to the provider but lost before processing begins. Durability is provided by the database write itself, not by an in-memory queue or a secondary broker.

**Implementation status:** live as of Milestone 2 (`internal/eventstore.Insert`), not a design placeholder. Verified two ways: a direct SQL join confirming a real linked `events`/`outbox` row pair after a successful write, and a concurrent test firing the identical event via two simultaneous goroutines, confirming exactly one success, one duplicate result, and exactly one row in each table - proof the guarantee holds under a genuine race, not just under sequential testing.

### 4.3 Retry with Exponential Backoff
Processing failures are retried on an increasing schedule (1s, 5s, 30s, 2m, 10m). This absorbs transient downstream failures without amplifying load on a struggling dependency.

**Implementation status:** live as of Milestone 3 (`internal/outbox.scheduleRetry`, backoff schedule in `internal/outbox/backoff.go`). Attempts beyond the schedule's length reuse its final entry as a ceiling rather than growing further. Verified by unit test (schedule mapping, including the ceiling case), by a manual end-to-end run confirming genuine exponential timing against wall-clock timestamps, and again in production (Section 14) via the deployed processor's own logs, which show real timestamped retry attempts matching the schedule.

### 4.4 Dead-Letter Recovery
After a configurable maximum attempt count, an event is moved to `dead_letter_events` with its last error attached, and removed from active retry. Dead-lettered events remain fully inspectable and can be manually or programmatically replayed once the underlying issue is resolved.

**Implementation status:** live as of Milestone 3 (`internal/outbox.promoteToDeadLetter`), with `MaxAttempts = 6` (5 retries after the first failure, matching the 5-entry backoff schedule). Verified with a full real end-to-end run locally - a forced failure taken through all 6 attempts on the actual backoff schedule, including the genuine ~10-minute final window - confirming the row lands in `dead_letter_events` with the real error attached and is fully removed from `outbox`. Also covered by a fast integration test (`internal/outbox/retry_dlq_test.go`) exercising the same code path without waiting through real backoff windows. Manual replay is implemented via both the dashboard (Section 12) and the MCP server (Section 13), and the full production retry→DLQ→replay cycle has been independently reproduced against the deployed instance (Section 14).

## 5. Data Model

```sql
CREATE TABLE events (
    event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key VARCHAR(255) UNIQUE NOT NULL,
    source VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    received_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    status VARCHAR(20) DEFAULT 'pending' NOT NULL  -- pending | processing | completed | dead_letter
);

CREATE TABLE outbox (
    outbox_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(event_id),
    attempt_count INT DEFAULT 0 NOT NULL,
    next_attempt_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    last_error TEXT
);

CREATE TABLE dead_letter_events (
    dlq_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(event_id),
    final_error TEXT NOT NULL,
    moved_at TIMESTAMPTZ DEFAULT now() NOT NULL
);

-- Represents the concrete downstream effect of a successfully processed event.
CREATE TABLE orders (
    order_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stripe_event_id UUID NOT NULL REFERENCES events(event_id),
    amount_cents BIGINT NOT NULL,
    status VARCHAR(20) DEFAULT 'confirmed' NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL
);
```

**Note:** `orders.stripe_event_id` references `events.event_id` (the internal UUID), not the provider's raw event string (`idempotency_key`) - the two are related but distinct fields, joinable via `events`. `orders.stripe_event_id` also carries a `UNIQUE` constraint (migration `0002`, Milestone 3/ENG-26), protecting against a duplicate order if the processor crashes between writing the order and removing the outbox row - the same idempotency principle applied at the ingestion layer, extended one layer downstream.

## 6. Request Lifecycle

1. Provider sends a webhook to `POST /webhooks/stripe`.
2. The HMAC-SHA256 signature is verified inline; invalid signatures are rejected with 401 before any persistence occurs.
3. The idempotency key is checked against `events`.
   - If already present: return 200 OK, no further action.
   - If new: insert into `events` and `outbox` in a single transaction, then return 200 OK. (Implemented and tested under concurrent load - see Section 4.2.)
4. The processor polls `outbox` for due rows and executes the downstream effect (a `payment_intent.succeeded` event results in a row written to `orders`). (Implemented and verified end-to-end - see Section 4.3/4.4.)
5. On failure: increment `attempt_count`, compute the next backoff window, record `last_error`. (Implemented - see Section 4.3.)
6. On exhausting max attempts: move the event to `dead_letter_events`. (Implemented - see Section 4.4.)
7. The dashboard reflects event, order, and dead-letter state, and supports manual dead-letter replay (Section 12). Fetch-on-unlock, not polling - see Section 12.3.
8. The MCP server exposes the same dead-letter data through `list_dlq_events`, `inspect_dlq_event`, and `replay_dlq_event`, allowing programmatic or agent-driven inspection and recovery (Section 13). Calls the identical `/api/*` contract the dashboard uses, not a separate data path.

## 7. Component Decisions & Tradeoffs

| Component | Choice | Rationale | Known limitation |
|---|---|---|---|
| Ingestion/processing language | Go 1.26.7 | Lightweight concurrency model, low memory footprint, well suited to a request-heavy ingestion path | Smaller library ecosystem than Node for some integrations |
| Delivery guarantee | At-least-once + idempotency dedup | True exactly-once delivery is not achievable in general distributed systems; at-least-once with dedup is the standard practical substitute | Requires the dedup layer itself to be correct - a bug here reintroduces the problem it solves |
| Queue mechanism | Postgres-backed outbox | Avoids operating a separate broker at a scale that doesn't require one; keeps the durability guarantee inside a single transactional system | Not horizontally scalable to very high throughput without further work (see Section 8) |
| Dashboard framework | React + TypeScript + Vite | Mature ecosystem, strong tooling support | No polling/live updates yet - see Section 12.3 |
| MCP server language | Node/TypeScript, official `@modelcontextprotocol/sdk` | Standard SDK for the protocol; consistent with the dashboard's TypeScript stack | Package-local TypeScript pinned to `^6.0.0` - see Section 13.4 |
| Database | PostgreSQL (Neon in staging/production) | ACID guarantees required for outbox correctness | Serverless scale-to-zero introduces cold-start latency; mitigated by using local Docker Postgres in dev (Section 9) |
| Deployment target | Google Cloud Run | Serverless container hosting with per-request billing and scale-to-zero. `cmd/server` runs as a standard HTTP service; `cmd/processor` runs as a Cloud Run **Worker Pool**, since it is a pure polling loop with no HTTP surface (see Section 9.1) | Cold starts on infrequent invocation for `cmd/server`; Worker Pool tooling is comparatively newer than standard Cloud Run services |

## 8. Scaling Considerations

The current design intentionally trades maximum throughput for minimal operational surface. The following changes represent the natural evolution path if throughput requirements exceed what a single Postgres-backed outbox can sustain:

- Replace the outbox poller with a dedicated event broker (e.g. Kafka-compatible streaming platform) for the processor's input, while keeping Postgres as the durability layer for the outbox write.
- Horizontally scale the processor with partition-aware consumption to preserve per-event ordering guarantees where required.
- Introduce a dedicated cache (e.g. Redis) in front of the idempotency check if lookup latency becomes a bottleneck under load - this should be adopted only once measured, not preemptively.
- Add lock-wait telemetry on the outbox row-claiming path to surface contention proactively rather than discovering it reactively, as happened during the self-deadlock found in ENG-27's manual verification.

## 9. Environments & Deployment

| Environment | Postgres | Rationale |
|---|---|---|
| Local development | Docker Compose (`postgres:18-alpine`) | Matches Neon's default major version. Avoids serverless cold-start latency during rapid iteration on outbox/idempotency logic, allows unrestricted table resets, and works offline. |
| Staging / Production | Neon (serverless Postgres) | Managed, scale-to-zero, zero infrastructure to operate. |

Both environments are addressed through a single `DATABASE_URL` environment variable. Application code, including the migration runner (`go run ./cmd/migrate`), is environment-agnostic - it reads `DATABASE_URL` from the process environment and connects identically regardless of target, with no environment-specific branching in code.

### 9.1 Production deployment (Google Cloud Run)

| Component | Cloud Run resource type | Why |
|---|---|---|
| `cmd/server` | Standard service (`gcloud run deploy`) | Has a real HTTP surface (`/webhooks/stripe`, `/health`, `/api/*`) requiring a load-balanced endpoint |
| `cmd/processor` | Worker Pool (`gcloud run worker-pools deploy`) | Pure continuous polling loop, no HTTP surface at all - a standard service's startup probe (which expects a listening port) times out against it; Worker Pools are the purpose-built primitive for this shape of workload |
| `apps/dashboard` | Standard service, static nginx | Serves the built Vite bundle; genuinely public static content, protected one layer in by the `PasswordGate` and `cmd/server`'s Basic Auth, not by Cloud Run IAM |

Live URLs:
- API (`cmd/server`): `https://relay-server-202922425380.us-east4.run.app`
- Dashboard: `https://relay-dashboard-202922425380.us-east4.run.app`

**Docker images** are cross-compiled explicitly for `linux/amd64` (`docker build --platform linux/amd64 ...`) regardless of build-machine architecture, since Apple Silicon development machines default to `linux/arm64`, which Cloud Run's runtime does not accept.

**Secrets** (`DATABASE_URL`) are supplied to both `cmd/server` and `cmd/processor` via Google Secret Manager (`--set-secrets`), not as plaintext `--set-env-vars` - the Cloud Run service account is granted `roles/secretmanager.secretAccessor` on the relevant secret explicitly, separate from any developer's personal `gcloud auth login` identity.

**CORS/`DASHBOARD_ORIGIN`:** because `cmd/server`'s allowed origin must reference the dashboard's real Cloud Run URL, and that URL doesn't exist until the dashboard is first deployed, deployment order matters: the dashboard is deployed first, its URL is retrieved, and `cmd/server` is then (re)deployed with `DASHBOARD_ORIGIN` pointed at that real value.

## 10. Security

- All inbound payloads are cryptographically verified via HMAC-SHA256 before any persistence or processing occurs.
- Inbound request bodies are capped (`http.MaxBytesReader`) to prevent oversized-payload abuse.
- Basic in-memory token-bucket rate limiting is applied at the ingestion endpoint. This is scoped per-instance, not global - acceptable at portfolio scale, and noted as a known limitation under horizontal scaling (Section 8).
- No cardholder or other regulated financial data is ingested, stored, or transmitted - only transaction metadata (amount, event ID, status) required to demonstrate the downstream effect.
- Secrets (signing keys, database credentials, dashboard password) are held in environment configuration or Google Secret Manager in production (Section 9.1), never committed to source control.
- **API access** (both the dashboard and the MCP server) is gated behind a single shared password via HTTP Basic Auth (`internal/server/auth.go`), compared using `crypto/subtle.ConstantTimeCompare` rather than a plain string comparison, to avoid the timing-attack risk a short-circuiting `==` carries for secret comparisons. This is deliberately lightweight - appropriate for a public single-operator portfolio deployment where the goal is preventing casual/incidental access to event and order data, not multi-user access control. The MCP server authenticates with the identical `DASHBOARD_PASSWORD` credential the dashboard uses - not a separate secret or auth mechanism.
- **CORS** on `/api/*` is scoped to a single configurable origin (`DASHBOARD_ORIGIN`), not a wildcard - a wildcard combined with credentialed/authenticated requests is a real security anti-pattern, not just unnecessary here. In production this is set to the deployed dashboard's real Cloud Run URL (Section 9.1).
- The dashboard password is stored client-side in `sessionStorage`, not `localStorage` - cleared automatically when the browser tab closes, an appropriate lifetime for a shared credential on an internal tool rather than persisting indefinitely on the machine.
- **Container image scanning:** all three Docker images (`relay-server`, `relay-processor`, `relay-dashboard`) are scanned in CI via Trivy (`trivy-scan` job), configured to fail the build on any unresolved CRITICAL/HIGH severity finding with an available fix. The scanning action itself is pinned to a specific commit SHA rather than a mutable version tag, following disclosure of a real supply-chain compromise affecting the majority of that action's published tags.

## 11. Operational Resilience

- **Connection pooling:** the Go services set explicit `SetMaxOpenConns`/`SetMaxIdleConns` limits so a burst of retry activity cannot exhaust Postgres's connection cap.
- **Graceful shutdown:** both `cmd/server` and `cmd/processor` listen for termination signals (`signal.NotifyContext`) and drain in-flight work before exiting, so a Cloud Run scale-down or redeploy cannot kill a webhook mid-write. `cmd/processor`'s was implemented first (Milestone 3, ENG-29); a gap was then discovered where `cmd/server` had no equivalent handling - closed in a follow-up (ENG-32) using `net/http.Server`'s built-in `Shutdown(ctx)`. Both services now genuinely implement this.
- **Correlated logging:** every log line touching a given event - from ingestion through retry attempts to completion or dead-lettering - carries the same `event_id`, making a single event's full lifecycle traceable end to end with a single grep or log query. In production, `cmd/processor`'s logs (including retry/backoff and dead-letter promotion events) are readable via `gcloud beta run worker-pools logs read relay-processor --region=us-east4`, since Worker Pools are not standard HTTP services and are not browsed the same way as `cmd/server`'s logs in the Cloud Run console.

## 12. Dashboard API (Milestone 4)

### 12.1 Endpoints
All served by `cmd/server`, gated behind the Basic Auth middleware described in Section 10:

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/events` | GET | List recent events (capped at 100 rows, no pagination yet) |
| `/api/orders` | GET | List recent orders (capped at 100 rows) |
| `/api/dlq` | GET | List dead-lettered events |
| `/api/dlq/{id}` | GET | Fetch one dead-letter event's full detail (404 if not found) - added in Milestone 5 (ENG-42) specifically to back the MCP server's `inspect_dlq_event` tool |
| `/api/dlq/{id}/replay` | POST | Atomically move a dead-lettered event back into `outbox` (`attempt_count` reset to 0, `next_attempt_at = now()`) and remove it from `dead_letter_events` |

### 12.2 Frontend
`apps/dashboard` - Vite + React + TypeScript, sibling to `apps/ingestion`. Component shells (`EventsTable`, `OrdersTable`, `DlqView`) were initially generated via v0 (Vercel), then rewritten to drop v0's default shadcn/ui + Base UI + Next.js App Router dependencies - v0's sandbox defaults to a Next.js project regardless of the target project's actual framework - down to plain Tailwind v4 + native HTML elements, preserving the same component structure, props, and TypeScript types without the disproportionate dependency footprint of a full shadcn stack for three tables.

Auth: a `PasswordGate` component blocks the dashboard until a password is entered; a 401 response from any API call clears the stored password and returns to the gate.

### 12.3 Known Limitations
- No pagination on list endpoints (acceptable at current data volume).
- No live updates - fetch-on-unlock only, no polling or subscription. Confirmed in production: data written directly to the database while the dashboard is open requires a manual browser refresh to appear.
- Single shared password, not multi-user auth - see Section 10.

## 13. MCP Server (Milestone 5)

### 13.1 Tools
`apps/mcp-server`, Node/TypeScript, official `@modelcontextprotocol/sdk`, communicating over stdio transport (the standard transport for a locally-run MCP server invoked as a subprocess by a client, distinct from `cmd/server`'s own HTTP server):

| Tool | Backing endpoint | Input | Notes |
|---|---|---|---|
| `list_dlq_events` | `GET /api/dlq` | *(none)* | Returns all dead-lettered events |
| `inspect_dlq_event` | `GET /api/dlq/{id}` | `dlq_id` | Validated via Zod (`z.string().uuid()`) before any network call |
| `replay_dlq_event` | `POST /api/dlq/{id}/replay` | `dlq_id` | Marked with MCP annotations `destructiveHint: true`, `idempotentHint: false` - the protocol's own mechanism for signaling a state-changing action to a well-behaved client, rather than a custom confirmation pattern. `idempotentHint: false` is deliberately accurate: replaying an already-replayed `dlq_id` correctly returns a not-found error on a second call, since the row no longer exists in `dead_letter_events` |

All three tools call `cmd/server`'s `/api/*` endpoints through a shared `ApiClient`, authenticated with the same `DASHBOARD_PASSWORD` credential the dashboard uses (Section 10) - no separate data access path or duplicated query logic.

### 13.2 Error Handling
A `404` from the underlying API is caught explicitly and returned as a normal MCP tool response with `isError: true` and a clear message, rather than an uncaught exception - giving an AI agent using these tools a legible signal to reason about ("no such record") instead of a raw stack trace. Malformed input (e.g. an invalid `dlq_id`) is rejected by the SDK's own Zod-based schema validation before a tool's handler ever executes, surfaced as a standard JSON-RPC `-32602` invalid-params error at the protocol level.

### 13.3 Testing
Automated tests use an in-memory MCP client/server pair (`InMemoryTransport`) - no subprocess, no stdio - exercising the real registered tool handlers against a live `cmd/server` instance. Deliberately does not re-test the full plant→replay→verify-in-Postgres cycle, already proven via manual verification (ENG-46), to avoid duplicating coverage without adding value. Manual verification via the official MCP Inspector (`@modelcontextprotocol/inspector`) remains the closing proof that the tools are genuinely invokable by a real client, not just correct in isolation - this was repeated against the production deployment in Milestone 6 (Section 14).

**CI:** the `mcp-server-build-lint-test` job compiles the real `cmd/server` binary, runs it in the background against the job's Postgres service container, polls its `/health` endpoint until ready, and only then runs the MCP tool test suite against that live instance - closing what was originally a documented gap (install/lint/build only, no live-server integration test). `golangci-lint` is not re-run in this job, since `build-lint-test` already covers it for the same Go source.

### 13.4 Known Limitations
- This package's TypeScript is pinned to `^6.0.0`, one major version behind the rest of the toolchain's default (`typescript-eslint` does not yet support TypeScript 7 - confirmed via the library's own tracking issue at the time of writing). Revisit once upstream support ships.

## 14. Deployed End-to-End Verification (Milestone 6)

All reliability guarantees described in Section 4 have been independently reproduced against the live production deployment (Section 9.1), not only against local development or CI. This section records what was verified and how.

### 14.1 Dedup, in production
`tools/chaos-test -mode dedup`, run against the deployed `cmd/server` URL, fired an identical event twice. Both HTTP responses returned `200 OK`; `/api/events` confirmed exactly one row for the fired idempotency key, and `/api/orders` confirmed exactly one corresponding order - the same guarantee proven in local integration tests (Section 4.1), now confirmed against real deployed Postgres (Neon), not Docker.

### 14.2 Failure, retry, backoff, and dead-letter, in production
`tools/chaos-test -mode failure`, run against the deployed `cmd/server` URL, fired an event with an unparseable payload. The full ~13-minute retry cycle was observed three ways simultaneously:
- **Processor logs** (`gcloud beta run worker-pools logs read relay-processor`), showing timestamped entries consistent with the documented backoff schedule (1s, 5s, 30s, 2m, 10m) before dead-letter promotion.
- **The deployed dashboard's DLQ panel**, which moved from `0 blocked` to `1 blocked` with the real error message displayed.
- **`GET /api/dlq`**, returning the same dead-letter row directly, confirming the dashboard and the API agree.

### 14.3 MCP server against production
The MCP Inspector was connected to a locally-run `apps/mcp-server` process configured with `API_BASE_URL` pointed at the deployed `cmd/server` URL rather than `localhost` - the first time the MCP layer was exercised against production rather than local development.

- `list_dlq_events` returned the real dead-lettered event from Section 14.2.
- `replay_dlq_event` was called against that event's `dlq_id`. The Inspector UI independently surfaced a "DESTRUCTIVE" badge sourced from the tool's `destructiveHint: true` annotation, confirming - again, now against production - that the annotation is read correctly by a real client. The call succeeded, and `GET /api/dlq` confirmed the row was deleted.
- `replay_dlq_event` was called a second time with the same `dlq_id`. This correctly failed with a not-found error, confirming `idempotentHint: false` was the accurate annotation: a second replay of the same dead-letter row is not a repeatable, idempotent operation.

This closes the loop on all five components - `cmd/server`, `cmd/processor`, Neon Postgres, the dashboard, and the MCP server - demonstrated working together against real deployed infrastructure, not simulated or assumed from local behavior.