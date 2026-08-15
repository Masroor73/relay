# Relay - Technical Architecture & Design Specification

## 1. Overview

Relay is a webhook reliability gateway. It sits between an upstream event provider (e.g., Stripe) and downstream business logic, guaranteeing that every event is received durably, processed at-least-once, deduplicated safely, and recoverable if processing fails.

Webhook-based integrations are structurally unreliable by default: providers retry on timeout, networks drop connections mid-delivery, and downstream services fail transiently. Relay's job is to absorb that unreliability at the infrastructure layer so that business logic downstream can assume clean, exactly-once-effective delivery.

## 2. Design Goals & Constraints

- **Durability over throughput.** At current scale, correctness (no lost or duplicated events) is prioritized over horizontal scalability. The architecture is designed to evolve toward higher throughput without a rewrite (see Section 8).
- **Minimal infrastructure surface.** Every component choice favors the smallest number of moving parts that still satisfies the durability guarantee, rather than the most scalable option available.
- **Observability by default.** Every event's lifecycle (received → processing → completed/dead-lettered) is queryable, both through a UI and programmatically.

## 3. System Architecture

```
Provider (Stripe)
      │  HTTPS POST + HMAC-SHA256 signature
      ▼
┌─────────────────┐
│ Ingestion (Go)   │  verify signature → dedupe check → transactional write
└────────┬─────────┘
         │
         ▼
┌─────────────────┐
│  Postgres        │  events · outbox · dead_letter_events · orders
└────────┬─────────┘
         │  poll due rows
         ▼
┌─────────────────┐
│ Processor (Go)   │  execute downstream effect → retry w/ backoff → DLQ on exhaustion
└────────┬─────────┘
         │
         ▼
┌─────────────────┐      ┌──────────────────┐
│ Dashboard (React)│◄────►│  MCP Server (TS)  │
└─────────────────┘      └──────────────────┘
```

## 4. Core Reliability Mechanisms

### 4.1 Idempotency Keys
Every inbound event is deduplicated on a key derived from the provider's event ID (e.g. Stripe's `evt_...`), falling back to a payload hash if no stable ID is present. A duplicate delivery is acknowledged (200 OK) without reprocessing. This is what makes provider-side retries safe by construction, rather than by convention.

**Concurrency note:** the dedup guarantee is enforced by a `UNIQUE` constraint on `events.idempotency_key` at the database layer, not by an application-level check-then-insert. Two simultaneous deliveries of the same event will both attempt the insert; the database allows exactly one to succeed and returns a constraint violation to the other, which the ingestion service catches and treats as a duplicate (200 OK, no reprocessing). This is deliberate - a `SELECT`-then-`INSERT` check in application code has a race window under concurrent delivery and would not actually hold the guarantee it claims to.

### 4.2 Transactional Outbox
On receipt, the raw event and its corresponding outbox entry are written to Postgres in a single transaction. This closes the gap between "event received" and "event queued for processing" - there is no window in which an event can be acknowledged to the provider but lost before processing begins. Durability is provided by the database write itself, not by an in-memory queue or a secondary broker.

**Implementation status:** live as of Milestone 2 (`internal/eventstore.Insert`), not a design placeholder. Verified two ways: a direct SQL join confirming a real linked `events`/`outbox` row pair after a successful write, and a concurrent test firing the identical event via two simultaneous goroutines, confirming exactly one success, one duplicate result, and exactly one row in each table - proof the guarantee holds under a genuine race, not just under sequential testing.

### 4.3 Retry with Exponential Backoff
Processing failures are retried on an increasing schedule (e.g. 1s, 5s, 30s, 2m, 10m). This absorbs transient downstream failures without amplifying load on a struggling dependency.

**Implementation status:** live as of Milestone 3 (`internal/outbox.scheduleRetry`, backoff schedule in `internal/outbox/backoff.go`). Attempts beyond the schedule's length reuse its final entry as a ceiling rather than growing further. Verified both by unit test (schedule mapping, including the ceiling case) and by a manual end-to-end run confirming genuine exponential timing against wall-clock timestamps.

### 4.4 Dead-Letter Recovery
After a configurable maximum attempt count, an event is moved to `dead_letter_events` with its last error attached, and removed from active retry. Dead-lettered events remain fully inspectable and can be manually or programmatically replayed once the underlying issue is resolved.

**Implementation status:** live as of Milestone 3 (`internal/outbox.promoteToDeadLetter`), with `MaxAttempts = 6` (5 retries after the first failure, matching the 5-entry backoff schedule). Verified with a full real end-to-end run - a forced failure taken through all 6 attempts on the actual backoff schedule, including the genuine ~10-minute final window - confirming the row lands in `dead_letter_events` with the real error attached and is fully removed from `outbox`. Also covered by a fast integration test (`internal/outbox/retry_dlq_test.go`) that exercises the same code path without waiting through real backoff windows. Manual replay tooling itself is not yet implemented - that's Milestone 4/5 scope (dashboard replay button, MCP `replay_dlq_event` tool).

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

## 6. Request Lifecycle

1. Provider sends a webhook to the ingestion endpoint.
2. The HMAC-SHA256 signature is verified inline; invalid signatures are rejected with 401 before any persistence occurs.
3. The idempotency key is checked against `events`.
   - If already present: return 200 OK, no further action.
   - If new: insert into `events` and `outbox` in a single transaction, then return 200 OK. (Implemented and tested under concurrent load - see §4.2.)
4. The processor polls `outbox` for due rows and executes the downstream effect (a `payment_intent.succeeded` event results in a row written to `orders`). (Implemented and verified end-to-end as of Milestone 3 - see §4.2/§4.3/§4.4.)
5. On failure: increment `attempt_count`, compute the next backoff window, record `last_error`. (Implemented - see §4.3.)
6. On exhausting max attempts: move the event to `dead_letter_events`. (Implemented - see §4.4.)
7. The dashboard reflects event, order, and dead-letter state in near-real-time.
8. The MCP server exposes the same dead-letter data through `list_dlq_events`, `inspect_dlq_event`, and `replay_dlq_event`, allowing programmatic or agent-driven inspection and recovery.

## 7. Component Decisions & Tradeoffs

| Component | Choice | Rationale | Known limitation |
|---|---|---|---|
| Ingestion/processing language | Go | Lightweight concurrency model, low memory footprint, well suited to a request-heavy ingestion path | Smaller library ecosystem than Node for some integrations |
| Delivery guarantee | At-least-once + idempotency dedup | True exactly-once delivery is not achievable in general distributed systems; at-least-once with dedup is the standard practical substitute | Requires the dedup layer itself to be correct - a bug here reintroduces the problem it solves |
| Queue mechanism | Postgres-backed outbox | Avoids operating a separate broker at a scale that doesn't require one; keeps the durability guarantee inside a single transactional system | Not horizontally scalable to very high throughput without further work (see Section 8) |
| Dashboard framework | React + TypeScript | Mature ecosystem, strong tooling support | - |
| Database | PostgreSQL (Neon) | ACID guarantees required for outbox correctness | Serverless scale-to-zero introduces cold-start latency; disabled in any deployment where this matters |
| Deployment target | Google Cloud Run | Serverless container hosting with per-request billing and scale-to-zero | Cold starts on infrequent invocation |

## 8. Scaling Considerations

The current design intentionally trades maximum throughput for minimal operational surface. The following changes represent the natural evolution path if throughput requirements exceed what a single Postgres-backed outbox can sustain:

- Replace the outbox poller with a dedicated event broker (e.g. Kafka-compatible streaming platform) for the processor's input, while keeping Postgres as the durability layer for the outbox write.
- Horizontally scale the processor with partition-aware consumption to preserve per-event ordering guarantees where required.
- Introduce a dedicated cache (e.g. Redis) in front of the idempotency check if lookup latency becomes a bottleneck under load - this should be adopted only once measured, not preemptively.

## 9. Environments

| Environment | Postgres | Rationale |
|---|---|---|
| Local development | Docker Compose (`postgres:18-alpine`) | Matches Neon's default major version. Avoids serverless cold-start latency during rapid iteration on outbox/idempotency logic, allows unrestricted table resets, and works offline. |
| Staging / Production | Neon (serverless Postgres) | Managed, scale-to-zero, zero infrastructure to operate. |

Both environments are addressed through a single `DATABASE_URL` environment variable. Application code, including the migration runner (`go run ./cmd/migrate`), is environment-agnostic - it reads `DATABASE_URL` from the process environment and connects identically regardless of target, with no environment-specific branching in code.

## 10. Security

- All inbound payloads are cryptographically verified via HMAC-SHA256 before any persistence or processing occurs.
- Inbound request bodies are capped (`http.MaxBytesReader`) to prevent oversized-payload abuse.
- Basic in-memory token-bucket rate limiting is applied at the ingestion endpoint. This is scoped per-instance, not global - acceptable at portfolio scale, and noted as a known limitation under horizontal scaling (Section 8).
- No cardholder or other regulated financial data is ingested, stored, or transmitted - only transaction metadata (amount, event ID, status) required to demonstrate the downstream effect.
- Secrets (signing keys, database credentials, dashboard password) are held in environment configuration, never committed to source control.
- **Dashboard access** is gated behind a single shared password, set via environment variable. This is deliberately lightweight - appropriate for a public single-operator portfolio deployment where the goal is preventing casual/incidental access to event and order data, not multi-user access control.

## 11. Operational Resilience

- **Connection pooling:** the Go services set explicit `SetMaxOpenConns`/`SetMaxIdleConns` limits so a burst of retry activity cannot exhaust Postgres's connection cap.
- **Graceful shutdown:** the processor service listens for termination signals (`signal.NotifyContext`) and drains in-flight work before exiting, so a Cloud Run scale-down or redeploy cannot kill a webhook mid-write. **Correction as of Milestone 3:** this section previously stated both services had shutdown handling; the processor's was implemented and verified in Milestone 3 (ENG-29), but `cmd/server` does not yet have equivalent handling - a real gap discovered while implementing the processor's, tracked as a separate issue (related to ENG-29 and ENG-5) rather than silently left inconsistent with this document.
- **Correlated logging:** every log line touching a given event - from ingestion through retry attempts to completion or dead-lettering - carries the same `event_id`, making a single event's full lifecycle traceable end to end with a single grep or log query.