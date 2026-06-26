# Silk Raddy — SaaS pivot roadmap

This roadmap translates `docs/critique.md` into an engineering plan
expressed against `docs/srs.md`. It lives alongside the original
upstream `docs/roadmap.md` (which is preserved as the hobbyist feature
list); this document is the **product-pivot** plan.

Each phase has a goal, an exit criterion ("done means…"), and the SRS
sections it satisfies. Phases are sequenced so the system is shippable
at the end of each, not just at the end of the whole plan.

---

## Phase A — Foundations

**Goal:** make the existing single-tenant app safe to operate as a
managed service, with the seams in place for multi-tenancy and billing
to land later without rewriting handlers.

**Exit criterion:** `/healthz`, `/readyz`, structured JSON logs with
`request_id`, CI on every push, runtime-configurable station branding,
and a brand-rename to `silkraddy`. The app is still single-tenant
internally but every code path that will need `tenant_id` later is
threaded with a request-scoped context object that we can hang it off
in Phase B.

**SRS sections satisfied:** FR-OPS-1, FR-OPS-2, FR-OPS-3 (partial),
FR-BRD-1 (partial), most of §4.3 (security baseline).

### Tasks

- A1. `GET /healthz` (liveness) and `GET /readyz` (readiness: DB ping, object-store ping). Public, no auth.
- A2. Switch `internal/logger/` to JSON output. Add `request_id` middleware that creates/propagates `X-Request-Id`.
- A3. GitHub Actions CI: `go build ./...`, `go vet ./...`, `go test ./...`, `npm run build` in both `web/player` and `web/studio`. Required on PRs to `master`.
- A4. Runtime-configurable player branding: read `AIRSTATION_PLAYER_TITLE` at runtime, expose via a `/api/v1/station/info` field the player already calls.
- A5. Address open items from the original code review: orphan `preparedTrackPath` cleanup on `AddTrack` failure, consistent slog usage in `track.LoadTracksFromDisk`, line-151 logging style.
- A6. Rename Go module from `github.com/cheatsnake/airstation` to a Silk Raddy-owned module path; rename binary; keep `AIRSTATION_*` env vars (backward compat) but also accept `SILKRADDY_*` aliases.
- A7. Replace `log.Fatal` in `internal/config/config.go` with a returned error → main-loop logs and exits.

---

## Phase B — Multi-tenancy

**Goal:** add the `tenant_id` axis that everything in Phase C–F
assumes. Until this lands, billing is meaningless because there is
nothing to bill.

**Exit criterion:** a Postgres-only deployment where two tenants can be
provisioned via an internal admin tool, each gets isolated stations,
queues, playlists, and audit logs, and a row from tenant A is provably
invisible to a session for tenant B via both application filtering and
DB row-level security.

**SRS sections satisfied:** FR-IAM-1 through FR-IAM-8 (except 2FA),
FR-STN-1, FR-STN-2, FR-OPS-2 (`tenant_id` on every log line).

### Tasks

- B1. Schema migration: add `tenants`, `users`, `memberships`, `api_tokens`, `audit_log`. Add `tenant_id NOT NULL` to every existing data table. Backfill the single existing tenant before enabling the constraint.
- B2. Postgres row-level security policies keyed off a session GUC (`silkraddy.tenant_id`) set on connection checkout.
- B3. Replace the single shared `AIRSTATION_SECRET_KEY` login with email + password (`argon2id`), email verification, password reset.
- B4. Session cookie carries `user_id`, `tenant_id`, `role`. Middleware enforces RBAC per FR-IAM-4 matrix.
- B5. Audit log writer used by every admin-plane mutation.
- B6. Per-user API tokens with scoped permissions.
- B7. The playback engine becomes per-tenant: a registry of `playback.State` keyed by `(tenant_id, station_id)` with a single-writer guarantee.
- B8. SQLite store is dropped from production (kept for tests/dev only). The dual-backend code we have today becomes test-only.

---

## Phase C — Object storage and CDN

**Goal:** take the listener data path off the application server.

**Exit criterion:** track uploads write to GCS, the playback engine
writes HLS segments to GCS under `tenants/<tid>/stations/<sid>/…`, and
listeners receive signed segment URLs that fetch directly from the
CDN. The Cloud Run service `--max-instances` cap is lifted.

**SRS sections satisfied:** FR-STN-4, FR-PB-2, scale assumptions in
§4.2.

### Tasks

- C1. Define `pkg/filestore.FileStore` with `Put`, `Get`, `Delete`, `SignedURL`, `Exists` methods. Local-FS impl wraps `os`. GCS impl uses the Google Cloud SDK.
- C2. Refactor `track.Service.PrepareTrack` and the HLS segment writer in `playback` to use the interface.
- C3. Replace the `GET /static/tracks/...` static-dir handler with a redirect to signed URLs.
- C4. Put Cloud CDN in front of the GCS bucket. Cache HLS segments aggressively, cache m3u8 with a short TTL.
- C5. Background reaper deletes orphaned objects (object exists, no DB row).

---

## Phase D — Self-serve onboarding

**Goal:** a stranger can land on the marketing site, sign up, and have
a working station with a custom subdomain in five minutes.

**Exit criterion:** the entire flow in FR-ONB-1 works end-to-end
without engineer intervention.

**SRS sections satisfied:** FR-ONB-1 through FR-ONB-5, FR-BRD-1
through FR-BRD-3.

### Tasks

- D1. Public marketing site at `silkraddy.app` (Next.js or Astro static site). Pricing page, "Start free" CTA.
- D2. Sign-up form, email verification.
- D3. First-run wizard inside studio: station name, colours, logo upload, optional custom domain.
- D4. `*.silkraddy.app` wildcard subdomain routing in the API.
- D5. Custom domain flow: hostname add → DNS validation → ACME cert issuance via Caddy / Cloud Run domain mappings.
- D6. Embeddable player widget: `<script src="https://silkraddy.app/embed.js" data-station="abc">`.
- D7. Sample-content seeder for new stations.

---

## Phase E — Billing

**Goal:** charge customers.

**Exit criterion:** a tenant on the Free plan can enter card details,
upgrade to Starter or Pro, get invoiced on cycle, and hit overage that
appears on the next invoice.

**SRS sections satisfied:** FR-BIL-1 through FR-BIL-6, §6.

### Tasks

- E1. Stripe integration: customers, subscriptions, prices, tax, billing portal.
- E2. `plans` and `entitlements` tables; enforcement middleware reads them.
- E3. Nightly usage meter aggregator: listener-hours from `spinlog`, storage from object-store inventory, seats from `memberships`, custom domains from `domains`.
- E4. Stripe usage records pushed nightly. Dunning state machine wired to the SCA failure webhook.
- E5. In-studio billing page with plan picker, invoices, usage charts.
- E6. Free-trial entitlement flag on Pro signup (14-day, no card required).

---

## Phase F — Product depth

**Goal:** features that turn the product from "viable" to "preferred."

**Exit criterion:** scheduler, analytics, and bulk import shipped and
documented; SLA can be raised to 99.9%.

**SRS sections satisfied:** FR-STN-5, FR-STN-6, FR-STN-7, FR-PB-3
(operator analytics surface), §3.5 widget polish.

### Tasks

- F1. Scheduler: cron-like recurring blocks + one-off overrides, with conflict resolution and a calendar UI.
- F2. Analytics: concurrent-listener time series, track popularity, geography (from CDN logs), skip rate.
- F3. Bulk importer: S3 / GCS / Dropbox / Drive read-only credential → async ingest job → per-file result.
- F4. Crossfade and gapless playback transitions.
- F5. Mobile-optimised studio (PWA install prompt, offline-tolerant queue editor).

---

## Phase G — Compliance and legal

**Goal:** be safe to sell to a mid-market hospitality chain or
European company.

**Exit criterion:** ToS, Privacy Policy, DPA, GDPR data-export and
delete flows, DMCA process, and an exportable spinlog format that maps
to ASCAP / BMI / PRS / SOCAN / SACEM monthly reporting templates.

**SRS sections satisfied:** FR-CMP-1 through FR-CMP-6, FR-IAM-7
retention, §4.3 security baseline upgrades.

### Tasks

- G1. Legal docs site section, version-stamped, with consent capture on signup.
- G2. Spinlog exporter (CSV + provider-template formats).
- G3. GDPR self-serve data export and delete endpoints; admin "right to be forgotten" tooling.
- G4. DMCA takedown email, form, audit trail.
- G5. SOC 2 readiness items: access reviews, secret rotation runbook, encryption-at-rest verification, vendor list.
- G6. Public status page at `status.silkraddy.app` driven by uptime probes.

---

## Phase H — Scale-out

**Goal:** lift the constraints that the v1 architecture leaves in
place.

**Exit criterion:** API tier is horizontally scaled; ingest tier is a
separate Cloud Run service; per-region replicas behind a global LB;
RTO ≤ 30 min.

**SRS sections satisfied:** §4.1 (RTO), §4.2 (concurrent listener
ceiling).

### Tasks

- H1. Split ingest from API into a separate Cloud Run service consuming a pub/sub queue.
- H2. Playback-engine leader election per `(tenant_id, station_id)` so multiple API replicas can run while only one owns each playback state.
- H3. Multi-region deployment with regional Cloud SQL read replicas; failover playbook.
- H4. Synthetic listener probes from multiple regions feeding the status page.

---

## Sequencing rules

- Phase A is non-negotiable; everything else assumes JSON logs, health endpoints, and CI exist.
- Phase B before Phase E. Billing without tenants is theatre.
- Phase C can land in parallel with Phase B (different files).
- Phase D before public launch.
- Phase G before signing any B2B contract above $1k MRR.
- Phase H is demand-driven; do not over-invest in it before customer evidence justifies it.

## What we are explicitly *not* doing first

- Live DJ broadcast / mic-in. Hard engineering, niche audience for the
  initial customer segments.
- Native mobile apps. PWA is the v1 surface.
- Generative-AI features. Distraction from the durability work.
- DRM. Not required by the v1 target segments.

If a customer pulls one of these forward with real ARR, we'll
re-sequence — but on present information, building them first would
delay every revenue-bearing phase above.
