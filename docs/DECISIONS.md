# Relay - Decisions Log

Context for why Relay is scoped and built the way it is. Read alongside ARCHITECTURE.md and README.md, which describe *what* was built; this describes *why*, including what was deliberately rejected.

## Origin

Relay was scoped as project #1 in a portfolio-building effort, replacing an earlier, much larger project (a fintech fraud-risk platform) that was abandoned in its current form specifically because it tried to cover too much ground in one system: five language runtimes, a Kafka-class broker, multi-tenant RLS, PCI-DSS-adjacent compliance surface, and a four-tier MCP plan, none of which got fully built. The lesson carried forward: prove one capability well and finish it, rather than spec many capabilities and finish none.

Relay deliberately reuses one idea from that earlier project - reliable event ingestion - and stops there, with no fraud/risk decisioning, no regulated data, no multi-service sprawl.

## Core scope decision

Relay is infrastructure, not a product with business logic. It receives, deduplicates, retries, and recovers webhook events. The `orders` table exists only to make processing visibly real (not as a genuine e-commerce feature) - this was a deliberate choice to make a backend-heavy project demoable, not scope creep into building an order system.

## Stack decisions and why

- **Go, not C# or Python** - no honest justification for either in this project; Go fits the concurrent ingestion/processing role directly. C# and Python are reserved for future portfolio projects where they're the actual right answer, not decorative additions here.
- **React, not Vue** - proven experience from a prior team project (V.A.K capstone) means lower friction to actually finish. Vue was considered and deliberately deferred to a future project where trying a second framework is itself the point, not bolted on here for the sake of breadth.
- **Postgres-backed outbox, not Kafka/Redpanda** - avoids standing up a broker before it's justified by actual throughput needs. This is the direct correction of the earlier project's mistake (broker adopted on day one with no justification). Explicitly documented as a Phase II option if throughput ever demands it - not a v1 requirement.
- **One MCP server, three tools** (`list_dlq_events`, `inspect_dlq_event`, `replay_dlq_event`) - chosen because it's a natural byproduct of data already being built for the DLQ, not a new subsystem. **Result:** implemented in Milestone 5, all three tools calling `cmd/server`'s existing `/api/*` contract - the same data layer the dashboard uses - rather than reimplementing the underlying queries a second time in TypeScript. Proven against production, not just local, in Milestone 6 (ENG-56).
- **`replay_dlq_event` marked `destructiveHint: true`, `idempotentHint: false`** - MCP's own annotation mechanism for signaling a state-changing, non-repeatable action to a client. Verified via the Inspector's own UI independently surfacing a "DESTRUCTIVE" badge, confirmed again against production in ENG-56.
- **v0 (Vercel) for dashboard UI scaffolding** - accelerates admin-UI boilerplate. **Result:** v0's default output targets Next.js regardless of the target project's real framework; generated components were rewritten to plain Tailwind v4 + native HTML for the actual Vite project.
- **Local dev: Docker Postgres (postgres:18-alpine) / Staging+prod: Neon** - both environments read the same `DATABASE_URL` contract with no code branching. **Discovered via a real crash-loop on first setup:** Postgres 18's image expects a mount at the parent `/var/lib/postgresql` directory, not directly at `.../data`.
- **Idempotency enforced via DB `UNIQUE` constraint, not application check-then-insert** - proven in Milestone 2 via a real concurrent test, and again against production in Milestone 6 (`tools/chaos-test -mode dedup` against live Cloud Run + Neon, exactly one canonical event despite two HTTP deliveries).
- **`MaxAttempts = 6`** - gives the 5-entry backoff schedule (1s, 5s, 30s, 2m, 10m) an exact 1-through-5 mapping. Proven locally in Milestone 3 and again in production (ENG-56) via three independent sources - processor logs, dashboard, API - agreeing on the same real ~13-minute timeline.
- **Outbox row claiming via `SELECT ... FOR UPDATE SKIP LOCKED`, lock held for the full claim→handle→delete lifecycle in one `sql.Tx`** - an initial version that only held the lock across the claiming `SELECT` was caught and corrected before merge (ENG-25).
- **Graceful shutdown implemented for the processor first, `cmd/server` initially not bundled in** - discovered as a gap in already-shipped Milestone 1 code while building ENG-29. Closed via ENG-32.
- **Dashboard API served by `cmd/server`, not a separate service** - this same endpoint set backs both the dashboard and the MCP server.
- **CORS scoped to a single configurable origin, not a wildcard.**
- **Dashboard password compared via `crypto/subtle.ConstantTimeCompare`, not `==`.**
- **Dashboard fetches once on unlock, does not poll** - confirmed as a real, deliberate limitation during both ENG-38's local verification and ENG-56's production verification.
- **`apps/mcp-server`'s TypeScript pinned to `^6.0.0`** - `typescript-eslint@8.67.0` does not support TypeScript 7 (confirmed via the library's own tracking issue).
- **Cloud Run Worker Pool for `cmd/processor`, not a standard service** - `cmd/processor` has no HTTP surface at all; a standard-service deployment attempt was tried during ENG-51 and genuinely failed with a startup-probe timeout (no listening port for the probe to check), concrete proof the poller needed a non-HTTP-shaped deployment target, not just a theoretical preference. Worker Pools (GA) are the purpose-built Cloud Run primitive for continuous, non-HTTP, pull-based workloads.
- **Secrets via Google Secret Manager, not plaintext `--set-env-vars`, for `DATABASE_URL` in production** - reversed mid-ENG-51 after a real credential exposure during debugging (plaintext env values persist in shell history and terminal scrollback). The Cloud Run service account's IAM identity was granted `secretmanager.secretAccessor` explicitly, distinct from any developer's personal `gcloud auth` identity. Neon password rotated twice as a direct consequence.
- **Neon region (`us-east-1`) deliberately co-located with Cloud Run's region (`us-east4`)** - chosen specifically to minimize DB round-trip latency on every request, not picked arbitrarily.
- **Dashboard deployed before `cmd/server`'s final redeploy, not the other way around** - `DASHBOARD_ORIGIN` needs the dashboard's real Cloud Run URL, which doesn't exist until the dashboard itself is deployed; resolved by deploying the dashboard first, then redeploying `cmd/server` with the real value.
- **`tools/chaos-test` fires real signed HTTP requests to `/webhooks/stripe`, not direct SQL inserts** - a genuinely more honest end-to-end proof than the manual `psql INSERT` pattern used throughout Milestones 2–5, since it exercises the guarantee from the actual public entry point. Built as its own Go module (not folded into `apps/ingestion`'s), since it calls the deployed HTTP API rather than importing internal packages.
- **Trivy scanning action pinned to a commit SHA, never a version tag** - pre-flight research before writing any CI code surfaced a real supply-chain attack: 76 of `aquasecurity/trivy-action`'s 77 published release tags were retroactively repointed in March 2026 to a credential-stealing payload running silently before the real scanner. SHA verification required a second, non-obvious step: `v0.36.0` is an annotated tag, so `git ls-remote refs/tags/v0.36.0` alone returns the tag object's SHA, not the actual commit - the dereferenced form (`refs/tags/v0.36.0^{}`) was needed to get the real commit being pinned. **Result:** the scanner's first real CI run found genuine findings (12 HIGH Go CVEs, 1 HIGH OpenSSL CVE in the dashboard's `nginx:alpine` base image) - fixed via a Go 1.26.7 bump and an explicit `apk update && apk upgrade` in the dashboard's final image stage, not a theoretical exercise.
- **`mcp-server-build-lint-test` CI job compiles and backgrounds a real `cmd/server` binary, polling `/health` before running MCP tests** - closes the gap explicitly documented as deferred since ENG-47, rather than leaving it permanently unaddressed. Uses the real compiled binary, not `go run`, consistent with the ENG-29 signal-handling lesson.
- **Stripe** chosen as the demo provider - most commonly requested integration pattern, well-documented real signature scheme (used as-is, not simplified).
- **Dashboard auth: single shared password via env var** - appropriate for a single-operator public portfolio deployment; explicitly not multi-user auth.

## Rejected alternatives (considered and explicitly not chosen)

- **A generic SEO/site-audit tool** - rejected because tools like Lighthouse, PageSpeed Insights, and GTmetrix already do this well for free.
- **A full web + mobile app in the same timeframe** - rejected as unrealistic solo without a team's parallelism; also redundant with an existing capstone project.
- **Kafka/Redpanda, full multi-tenant RLS, PCI-scoped data handling** - all present in the earlier fintech project; none needed here, deliberate absence.
- **GCP Secret Manager considered from the start, initially deferred to plaintext env vars for speed, then reversed** - see the Secrets decision above; the plaintext approach was a real, if brief, choice, not skipped over - worth recording that it was tried and actively corrected, not that Secret Manager was the plan all along.

## Tooling / process setup

- **Linear:** workspace "The Batcave" → team "ENG" → project "Relay" → initiative "Portfolio 2026." Six milestones, ~65 issues total. Labels: Type × Area. GitHub↔Linear one-way integration, magic-word commit linking.
- **Repo:** `relay` on GitHub, public, MIT license, combined Go+Node `.gitignore`.
- **Package manager:** `pnpm` for both Node packages, matching the project's Local Dev-Ex Architecture doc.
- **CI coverage - extended incrementally, each gap tracked rather than left silent:** `build-lint-test` (ENG-16) → `dashboard-build-lint` (ENG-41) → `mcp-server-build-lint-test` (ENG-47, extended to run against a live server in ENG-55) → `trivy-scan` (ENG-54). Four required jobs total by project completion.
- **Vitest test discovery excluding `dist/`** - fixed a silent double-count of every reported test run (ENG-47).
- **Known, deliberately unfixed inconsistency at project completion:** `tools/chaos-test/go.mod` remains pinned to `go 1.26.3`, not bumped alongside `apps/ingestion`'s `1.26.7` during ENG-54's CVE remediation - a separate module, not confirmed vulnerable, flagged rather than silently left unnoticed.

## Live status

Project complete as of ENG-57 (Milestone 6). This log's job - recording *why*, not tracking completion - is done in the sense that no further milestones are planned for Relay itself. If work resumes (e.g., addressing the chaos-test Go version, adding pagination, live-updating dashboard), continue appending entries here following the same pattern.