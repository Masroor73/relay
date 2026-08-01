# Relay — Decisions Log


Context for why Relay is scoped and built the way it is. Read alongside ARCHITECTURE.md and README.md, which describe *what* was built; this describes *why*, including what was deliberately rejected.


## Origin


Relay was scoped as project #1 in a portfolio-building effort, replacing an earlier, much larger project (a fintech fraud-risk platform) that was abandoned in its current form specifically because it tried to cover too much ground in one system: five language runtimes, a Kafka-class broker, multi-tenant RLS, PCI-DSS-adjacent compliance surface, and a four-tier MCP plan, none of which got fully built. The lesson carried forward: prove one capability well and finish it, rather than spec many capabilities and finish none.


Relay deliberately reuses one idea from that earlier project — reliable event ingestion — and stops there, with no fraud/risk decisioning, no regulated data, no multi-service sprawl.


## Core scope decision


Relay is infrastructure, not a product with business logic. It receives, deduplicates, retries, and recovers webhook events. The `orders` table exists only to make processing visibly real (not as a genuine e-commerce feature) — this was a deliberate choice to make a backend-heavy project demoable, not scope creep into building an order system.


## Stack decisions and why


- **Go, not C# or Python** — no honest justification for either in this project; Go fits the concurrent ingestion/processing role directly. C# and Python are reserved for future portfolio projects where they're the actual right answer, not decorative additions here.
- **React, not Vue** — proven experience from a prior team project (V.A.K capstone) means lower friction to actually finish. Vue was considered and deliberately deferred to a future project where trying a second framework is itself the point, not bolted on here for the sake of breadth.
- **Postgres-backed outbox, not Kafka/Redpanda** — avoids standing up a broker before it's justified by actual throughput needs. This is the direct correction of the earlier project's mistake (broker adopted on day one with no justification). Explicitly documented as a Phase II option if throughput ever demands it — not a v1 requirement.
- **One MCP server, three tools** (`list_dlq_events`, `inspect_dlq_event`, `replay_dlq_event`) — chosen because it's a natural byproduct of data already being built for the DLQ, not a new subsystem. Rejected: the four-tier MCP plan from the earlier project, which was scope bloat.
- **v0 (Vercel) for dashboard UI scaffolding** — accelerates admin-UI boilerplate; data-wiring and logic still hand-built. Not something to hide when discussing the project — a legitimate, current workflow.
- **Local dev: Docker Postgres (postgres:18-alpine, matching Neon's default version) / Staging+prod: Neon** — avoids Neon cold-start latency during rapid iteration, allows free table resets, works offline. Both environments read the same `DATABASE_URL` contract with no code branching.
- **Idempotency enforced via DB `UNIQUE` constraint, not application check-then-insert** — a sequential check has a race window under concurrent delivery. This is why Milestone 2 requires a *concurrent* dedup test (fire the same event twice simultaneously), not just a sequential one — a sequential test would pass even with a broken implementation.
- **Stripe** chosen as the demo provider — most commonly requested integration pattern, well-documented real signature scheme (used as-is, not simplified).
- **Dashboard auth: single shared password via env var** — appropriate for a single-operator public portfolio deployment; explicitly not multi-user auth, documented as a known limitation rather than a gap to silently fix later.


## Rejected alternatives (considered and explicitly not chosen)


- **A generic SEO/site-audit tool** — rejected because tools like Lighthouse, PageSpeed Insights, and GTmetrix already do this well for free; building another wouldn't demonstrate anything they don't already show better.
- **A full web + mobile app in the same timeframe as this project** — rejected as unrealistic solo in the intended timeframe without the parallelism a team (like the V.A.K capstone's 7 members) provides. Also somewhat redundant with V.A.K, which already demonstrates that capability; Relay was chosen specifically to prove a *different* skill (distributed-systems reliability) that V.A.K's stack didn't touch.
- **Kafka/Redpanda, full multi-tenant RLS, PCI-scoped data handling** — all present in the earlier fintech project; none needed here, and their absence is deliberate, not an oversight.


## Tooling / process setup


- **Linear:** workspace "The Batcave" → team "ENG" → project "Relay" → initiative "Portfolio 2026" (groups all future portfolio projects). Six milestones (Ingestion+schema → Outbox+idempotency → Processor/retry/DLQ → Dashboard → MCP server → Deploy+docs). Labels: Type (feature/bug/chore/docs) × Area (ingestion/processor/dashboard/mcp/infra). 1-week cycles. GitHub repo linked one-way (GitHub→Linear), branch format `username/identifier-title`, magic-word commit linking enabled.
- **Repo:** `relay` on GitHub, public, MIT license, combined Go+Node `.gitignore` (no dead rules for tools not actually used in this project, e.g. no `pgdata/` — Compose uses a named Docker volume, not a bind mount).


## Live status


This log does not track completion state — it only records *why* decisions were made, and is updated when a real decision-worthy fork occurs, not on a milestone or task cadence. Current build progress lives in Linear (The Batcave → ENG → Relay project → Milestones). If picking this up in a new chat, ask what's currently done rather than assuming from this file.

