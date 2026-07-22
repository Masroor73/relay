# Relay

**A self-hostable webhook reliability gateway.**

Webhook integrations fail silently and often: providers retry on timeout, networks drop mid-delivery, downstream services error transiently — and without a reliability layer in front of them, that means lost events or duplicated side effects. Relay sits between a webhook provider (e.g. Stripe) and your application, guaranteeing every event is durably received, deduplicated, retried on failure, and recoverable if it can't be processed — with full visibility into every event's lifecycle.

---

## Table of Contents

- [Architecture & System Design](#architecture--system-design)
- [Key Features & Reliability Guarantees](#key-features--reliability-guarantees)
- [Tech Stack & Ecosystem](#tech-stack--ecosystem)
- [Getting Started](#getting-started)
- [API Documentation](#api-documentation)
- [Observability & Logging](#observability--logging)
- [Testing](#testing)
- [Roadmap & Known Limitations](#roadmap--known-limitations)
- [License](#license)

---

## Architecture & System Design

```
Provider (Stripe)
      │  HTTPS POST + HMAC-SHA256 signature
      ▼
Ingestion (Go) → verify → dedupe → transactional write
      │
      ▼
Postgres: events · outbox · dead_letter_events · orders
      │
      ▼
Processor (Go) → downstream effect → retry w/ backoff → DLQ on exhaustion
      │
      ▼
Dashboard (React) ◄──► MCP Server (TypeScript)
```

Full design rationale, data model, and component tradeoffs are documented in [`ARCHITECTURE.md`](./ARCHITECTURE.md).

## Key Features & Reliability Guarantees

- **Idempotent ingestion** — duplicate provider retries are safely absorbed; no duplicate downstream effects.
- **Transactional outbox** — an event is never acknowledged and then silently lost before processing begins.
- **Exponential backoff retry** — transient downstream failures are retried on an increasing schedule rather than immediately or not at all.
- **Dead-letter recovery** — events that exhaust retries are isolated, inspectable, and replayable rather than lost.
- **Live dashboard** — real-time view of event status, processed orders, and dead-lettered events.
- **MCP server** — dead-letter inspection and replay exposed as tools for agent-driven or programmatic operations.

## Tech Stack & Ecosystem

| Layer | Technology |
|---|---|
| Ingestion & Processing | Go |
| Database | PostgreSQL (Neon) |
| Dashboard | React, TypeScript |
| MCP Server | Node.js, TypeScript, official Anthropic SDK |
| Deployment | Docker, Google Cloud Run |
| CI | GitHub Actions |

## Getting Started

### Prerequisites
```
Go >= 1.22
Node.js >= 20.x
Docker (for local Postgres via Docker Compose)
A Neon account (staging/production only — not required for local dev)
```

### Installation
```bash
git clone https://github.com/Masroor73/relay.git
cd relay
```

### Environment Setup
Create a `.env` file at the project root:
```bash
DATABASE_URL=your_postgres_connection_string
STRIPE_WEBHOOK_SECRET=your_stripe_signing_secret
```

### Running Locally
```bash
# Start local Postgres (waits for healthcheck before returning)
docker compose up -d

# Run database migrations — reads DATABASE_URL, works unchanged against Docker or Neon
go run ./cmd/migrate

# Seed fake events/orders for local testing
go run ./cmd/seed

# Start the ingestion + processor services
go run ./apps/ingestion
go run ./apps/processor

# Start the dashboard
cd apps/dashboard && npm install && npm run dev
```

## API Documentation

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
| `200 OK` | Event accepted (new or duplicate) |
| `401 Unauthorized` | Signature verification failed |

### MCP Tools
| Tool | Description |
|---|---|
| `list_dlq_events(filter)` | Lists dead-lettered events matching a filter |
| `inspect_dlq_event(event_id)` | Returns full payload, error history, and retry count for one event |
| `replay_dlq_event(event_id)` | Re-injects a dead-lettered event back into the processing pipeline |

## Observability & Logging

- Structured JSON logs emitted from both the ingestion and processor services, including `event_id` and `idempotency_key` for cross-service correlation.
- Dashboard provides a real-time queryable view of event status, retry counts, and dead-letter reasons.
- Chaos-testing tooling (see [Testing](#testing)) doubles as a mechanism for observing retry/backoff and dead-letter behavior live.

## Testing

A chaos-test CLI (`tools/chaos-test/`) is included to exercise the reliability guarantees directly:
- Fires an identical event twice to demonstrate idempotent deduplication.
- Fires an event with a forced-failure flag enabled to demonstrate retry/backoff and eventual dead-lettering.

## Roadmap & Known Limitations

**Scaling:**
- Replace the Postgres-backed outbox poller with a dedicated event broker if throughput requirements exceed what a single-node outbox can sustain.
- Horizontal scaling of the processor with partition-aware consumption for ordering guarantees.
- Redis-backed idempotency cache, adopted only if measured lookup latency justifies it.

**Known limitations at current scale:**
- Rate limiting is in-memory and scoped per-instance, not global — under horizontal scaling this would need to move to a shared store (e.g. Redis).
- Dashboard auth is a single shared password, appropriate for a single-operator deployment — a real multi-tenant deployment would need proper user accounts and authorization.
- Local development uses Docker Postgres (`postgres:18-alpine`) while staging/production use Neon; both are addressed through the same `DATABASE_URL` contract with no code branching, but schema drift between environments is a manual discipline, not yet automated.

## License

MIT
