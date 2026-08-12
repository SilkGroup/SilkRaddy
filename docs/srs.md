# Software Requirements Specification — Silk Raddy

| Field | Value |
| --- | --- |
| Document | Silk Raddy SRS |
| Status | v0.1 — initial draft following the SaaS pivot |
| Owners | Engineering + Founder |
| Source of truth | This document. When the product and this doc disagree, this doc is wrong — fix it. |

This SRS describes **Silk Raddy** as a multi-tenant SaaS that companies
around the world use to run their own branded internet radio stations.
It is the canonical product spec. The companion documents are
`docs/critique.md` (what's wrong today) and `docs/saas-roadmap.md` (the
phased plan to close the gap).

---

## 1. Purpose and scope

### 1.1 Product summary

Silk Raddy is a hosted, multi-tenant platform for operating an
HLS-streamed internet radio station. A customer (a "**tenant**", typically
a company) signs up, uploads their music library, schedules a programme,
points a custom domain at their station, and listeners stream from a
white-labelled web player or embedded widget.

### 1.2 In scope (v1)

- Multi-tenant station hosting (one Silk Raddy deploy → many independent stations)
- Web admin (Studio) and listener player (Player)
- HLS streaming with per-station playback engines
- Track library, queues, playlists, simple scheduling
- Per-tenant branding (logo, colours, station name, custom domain)
- Per-tenant user accounts with role-based access control
- Subscription billing with metered usage
- Music licensing play logs (for tenant-owned reporting)

### 1.3 Out of scope (v1)

- Live mic-in / DJ live broadcasts (roadmap)
- DRM / Widevine for premium content
- Mobile native apps (PWA only)
- Multi-track / multi-channel stations (one main channel per tenant)
- Built-in advertising marketplace (the operator inserts their own ads via scheduling)
- Generative-AI station programming

### 1.4 Target customers

| Segment | Example use | Volume | ARPU band |
| --- | --- | --- | --- |
| Hospitality chains | Hotel lobby music, brand consistency across properties | 10–500 stations per account | $$$ |
| Event production | Branded radio for conferences, festivals | 1–20 short-lived stations | $$ |
| Retail | In-store music + voice messaging | 1 station per chain, many listening venues | $$ |
| Community / niche broadcasters | Genre-specific online radios | 1 station per account | $ |
| Internal corporate radio | Employee comms, town halls | 1 per company | $$ |

---

## 2. Glossary

- **Tenant / Workspace / Org** — interchangeable; the top-level container that owns stations, users, billing.
- **Station** — one logical radio channel: its own library, queue, playback state, branding, custom domain. A tenant has ≥1 stations.
- **Listener** — anonymous end-user who streams the public player. Not authenticated.
- **Operator** — authenticated user (admin/DJ/viewer) inside a tenant.
- **Plan** — pricing tier (Free / Starter / Pro / Enterprise).
- **Meter** — a metered usage dimension (listener-hours, storage-GB-month, ingest-minutes, seats, custom-domains).
- **Programme** — scheduled blocks (recurring or one-off) that overrides the queue.
- **Spinlog** — the per-station play log: what played, when, to how many listeners. Source for licensing reports and analytics.

---

## 3. Functional requirements

### 3.1 Identity, access, multi-tenancy

| ID | Requirement |
| --- | --- |
| FR-IAM-1 | Every persisted row referencing tenant data must carry a non-null `tenant_id`. Read paths must filter by `tenant_id` derived from the authenticated session, never from request input. |
| FR-IAM-2 | Operators authenticate via email + password with email verification on sign-up. Passwords stored with `argon2id` (or `bcrypt` cost ≥ 12). |
| FR-IAM-3 | Operators can enable TOTP-based 2FA. Admin role requires 2FA when enforced by tenant policy. |
| FR-IAM-4 | Roles: `owner` (1 per tenant, billing-only differentiator), `admin`, `dj`, `viewer`. Permissions matrix is in §3.1.1. |
| FR-IAM-5 | An owner can invite users by email and assign a role at invite time. Pending invites expire in 7 days. |
| FR-IAM-6 | An owner can issue per-user API tokens with `read`/`write` scopes and optional expiry. Tokens are bearer-only, displayed once at creation. |
| FR-IAM-7 | All admin-plane mutations write to an append-only audit log (actor, tenant, action, resource, old/new value, IP, ts). Retention ≥ 90 days. |
| FR-IAM-8 | Account recovery is email-link based, single-use, 15-minute TTL. |

#### 3.1.1 RBAC matrix

| Capability | owner | admin | dj | viewer |
| --- | --- | --- | --- | --- |
| Manage billing | ✓ | — | — | — |
| Manage users / invites | ✓ | ✓ | — | — |
| Manage stations, branding, domains | ✓ | ✓ | — | — |
| Upload / delete tracks | ✓ | ✓ | ✓ | — |
| Edit queue, hit play/pause | ✓ | ✓ | ✓ | — |
| View analytics & spinlog | ✓ | ✓ | ✓ | ✓ |

### 3.2 Stations and content

| ID | Requirement |
| --- | --- |
| FR-STN-1 | A tenant may create up to the station limit allowed by its plan. |
| FR-STN-2 | Each station has its own library, queue, playlists, branding, custom domain, listeners, and play log. No cross-station mixing. |
| FR-STN-3 | Tracks accept MP3, AAC, WAV, FLAC. Server normalises to a canonical encoded form (existing pipeline). |
| FR-STN-4 | Track storage is object-storage backed (Cloud Storage in production; `localFS` for self-host). The `FileStore` interface must not leak provider details upwards. |
| FR-STN-5 | A station may have one of: a live programme block, a recurring schedule, or fall-through to a manual queue. The scheduler resolves which is "on air" at any tick. |
| FR-STN-6 | Programmes can be exclusive (override queue) or additive (mix into queue). |
| FR-STN-7 | Bulk import: an operator may import a library from S3 / GCS / Dropbox / Drive by giving a read-only credential; import runs async and surfaces per-file success/failure in the studio. |

### 3.3 Playback and streaming

| ID | Requirement |
| --- | --- |
| FR-PB-1 | Each station's playback state is a singleton process. The system must guarantee exactly one active playback writer per station at all times. |
| FR-PB-2 | Listener-facing playlists and segments are served via signed, short-lived URLs from object storage. The application server is not on the listener data path. |
| FR-PB-3 | Listener counts are sampled every ≤ 5 s and exposed via SSE to the studio and the player. |
| FR-PB-4 | A station can be paused and resumed without losing position in the current track. |
| FR-PB-5 | A station can be put into "maintenance" mode that returns a tenant-configurable fallback stream (or silence). |
| FR-PB-6 | Crossfade and gapless playback are supported between tracks when both tracks are in canonical encoded form. |

### 3.4 Self-serve onboarding

| ID | Requirement |
| --- | --- |
| FR-ONB-1 | A first-time visitor can sign up with email + password and reach a working empty station in ≤ 5 minutes without operator intervention. |
| FR-ONB-2 | First-run wizard collects: station name, default genre/tags, default branding (logo upload, colour pick), optional custom domain. |
| FR-ONB-3 | Custom domain flow: tenant enters a hostname; system issues a TLS cert via ACME; ready/not-ready state surfaced in studio with DNS instructions. |
| FR-ONB-4 | Free plan grants a `*.silkraddy.app` subdomain immediately without DNS work. |
| FR-ONB-5 | Sample content seed: optionally seed the new station with 3 royalty-free demo tracks so the operator can hit play immediately. |

### 3.5 Branding and embedding

| ID | Requirement |
| --- | --- |
| FR-BRD-1 | Logo, favicon, primary/secondary colours, station name, tagline, social links — all editable at runtime in the studio, no rebuild. |
| FR-BRD-2 | A tenant on Pro+ may upload custom CSS that is sandboxed (no external `@import`, no `expression(...)`). |
| FR-BRD-3 | The player exposes a `<script src=…>` widget that mounts a styled iframe on any site, with `postMessage` API for play/pause/volume. |

### 3.6 Billing

| ID | Requirement |
| --- | --- |
| FR-BIL-1 | Plans: Free, Starter, Pro, Enterprise. Plan defines hard and metered limits (see §6). |
| FR-BIL-2 | Card-on-file via Stripe (or equivalent), SCA-compliant. |
| FR-BIL-3 | Usage meters report nightly to the billing provider. Overage is invoiced at end of cycle. |
| FR-BIL-4 | Tenants can self-serve plan changes, view invoices, manage tax IDs (Stripe Tax). |
| FR-BIL-5 | Dunning: 3 retry attempts over 14 days; on final failure the tenant is downgraded to a read-only state for 7 days, then suspended. |
| FR-BIL-6 | Annual contracts and Enterprise quotes are flagged in-app and routed off-line. |

### 3.7 Observability and operations

| ID | Requirement |
| --- | --- |
| FR-OPS-1 | `GET /healthz` returns 200 once the process can serve requests; `GET /readyz` returns 200 only when DB and object storage are reachable. |
| FR-OPS-2 | Logs are emitted as JSON to stdout, with `tenant_id`, `request_id`, `actor_id` (where applicable) on every line. |
| FR-OPS-3 | Per-request `request_id` is generated if not present and echoed in `X-Request-Id` response header. |
| FR-OPS-4 | Prometheus-format metrics at `/metrics` (gated to a trusted source). Required series: `http_requests_total{tenant,route,status}`, `playback_listeners{tenant,station}`, `ingest_jobs_total{tenant,status}`. |
| FR-OPS-5 | Public status page (`status.silkraddy.app`) with at minimum: API uptime, ingest queue health, streaming p95 latency. |
| FR-OPS-6 | DB has automated daily backups retained ≥ 35 days; restore drill documented and executed quarterly. |

### 3.8 Security controls (baseline)

| ID | Requirement |
| --- | --- |
| FR-SEC-1 | The login endpoint enforces a per-client-IP token-bucket rate limit (default 10 requests burst, 5 per minute refill). Rejections return `429 Too Many Requests` with a `Retry-After` header. Configurable via `SILKRADDY_LOGIN_RATE_BURST` / `SILKRADDY_LOGIN_RATE_REFILL_PER_MIN`. |
| FR-SEC-2 | Multipart upload endpoints cap total request body size via `http.MaxBytesReader` (default 2 GiB). Excess returns `413 Request Entity Too Large`. Configurable via `SILKRADDY_MAX_UPLOAD_BYTES`; `0` disables the cap. |
| FR-SEC-3 | Uploaded filenames are sanitised via `filepath.Base`; empty, `.`, or `/` names are rejected. Collisions on the target path receive a short random suffix before the extension so concurrent uploads with the same name do not overwrite one another. |
| FR-SEC-4 | The SSE broadcaster is non-blocking. A subscriber whose channel is full has its event dropped rather than stalling every other consumer. Slow subscribers are logged for operator diagnosis. |
| FR-SEC-5 | HLS playlist responses set `Cache-Control: no-cache, no-store, must-revalidate` and return `503 Service Unavailable` when playback is stopped, so listeners retry rather than loop a stale window. |
| FR-SEC-6 | CORS is configurable per-deployment via `SILKRADDY_CORS_ORIGINS` (comma-separated origin allowlist). Empty preserves the legacy permissive default for backward compatibility with self-host deploys; production deploys must set this. |
| FR-SEC-7 | The authentication cookie is `HttpOnly`, `SameSite=Strict`, path-scoped to `/`, and `Secure` when `AIRSTATION_SECURE_COOKIE=true`. |
| FR-SEC-8 | Failed uploads roll back partial writes so aborted requests do not litter the tracks directory with half-processed files. Multipart temp files are cleaned via `deferred MultipartForm.RemoveAll()`. |

### 3.9 Compliance and legal

| ID | Requirement |
| --- | --- |
| FR-CMP-1 | Spinlog is exportable as CSV/JSON per station per month — the artefact a tenant submits to their licensing body. |
| FR-CMP-2 | Tenant can submit a GDPR data export request; system delivers a ZIP of their tenant data within 30 days. |
| FR-CMP-3 | Tenant can self-serve account deletion; data is hard-deleted within 30 days, audit log retained per FR-IAM-7. |
| FR-CMP-4 | DMCA takedown email and form linked from every public player footer. |
| FR-CMP-5 | Data is encrypted at rest (provider default) and in transit (TLS 1.2+). |
| FR-CMP-6 | No PII is logged in stdout logs at info level or above. |

---

## 4. Non-functional requirements

### 4.1 Availability and performance

- **API availability**: 99.9% monthly for paid plans.
- **Streaming availability**: 99.95% monthly (the listener data path is on object storage + CDN; the app server is not in the critical hot path).
- **Cold start to first request**: ≤ 5 s.
- **Studio admin p95 response time**: ≤ 500 ms.
- **Listener time-to-first-segment**: ≤ 2 s from clicking play.
- **Recovery point objective (RPO)**: ≤ 24 h. **Recovery time objective (RTO)**: ≤ 4 h.

### 4.2 Scale targets (v1)

- 1,000 active tenants, 5,000 stations, 50,000 concurrent listeners across the fleet.
- 99th-percentile station has ≤ 50,000 tracks (≤ 500 GB).

### 4.3 Security

- All endpoints HTTPS-only. HSTS preload.
- Per-tenant rate limits on auth, upload, and API endpoints.
- Public listener endpoints rate-limited per IP and per station to prevent hotlinking abuse.
- Dependency scanning (Dependabot / Snyk) on a weekly cadence.
- Annual external penetration test once paying revenue > $X.

### 4.4 Cost model assumptions

| Cost line | v1 budget assumption |
| --- | --- |
| Cloud Run | ≤ $50 / 1000 active tenants / month (autoscaled) |
| Cloud SQL Postgres | one regional HA instance, ≤ $200 / month |
| Cloud Storage (incl. egress to CDN) | dominant variable cost — modelled per listener-hour |
| CDN egress | the unit that pricing must price-in; see §6 |

---

## 5. System architecture (target state)

```
                                  ┌──────────────────────┐
                  ┌──────────────►│   CDN (segments)     │◄── listeners
                  │               └──────────────────────┘
                  │                          ▲ signed URLs
┌────────────┐    │               ┌──────────┴───────────┐
│  Player    │────┘               │   Cloud Storage      │
│  (Svelte)  │                    │   (per-tenant prefix)│
└────────────┘                    └──────────────────────┘
                                             ▲ writes
                                             │
┌────────────┐   HTTPS    ┌──────────────────┴─────┐    ┌────────────────┐
│  Studio    │──────────► │  silkraddy-api         │    │ silkraddy-     │
│  (Svelte)  │            │  (Go, Cloud Run)       │◄──►│ ingest         │
└────────────┘            │  - control plane       │    │ (Go, Cloud Run │
                          │  - playback engine     │    │  Jobs / Worker │
                          │  - SSE, REST           │    │  pool, ffmpeg) │
                          └────────────┬───────────┘    └────────────────┘
                                       │
                          ┌────────────┴──────────────┐
                          │  Cloud SQL Postgres       │
                          │  (multi-tenant w/ RLS)    │
                          └───────────────────────────┘
```

Key separations from today:

- **Ingest** (CPU-heavy ffmpeg) split from **API** (lightweight HTTP) so they scale independently.
- **Listener data path** bypasses the API entirely (CDN → GCS), so listener count does not stress the app tier.
- **Postgres row-level security** enforces `tenant_id` at the DB layer as a defence in depth on top of application filtering.

---

## 6. Plan limits (initial proposal)

| Limit | Free | Starter | Pro | Enterprise |
| --- | --- | --- | --- | --- |
| Stations | 1 | 1 | 5 | unlimited |
| Storage | 1 GB | 25 GB | 250 GB | custom |
| Monthly listener-hours | 100 | 5,000 | 50,000 | custom |
| Custom domains | — | 1 | 5 | unlimited |
| Seats | 1 | 3 | 15 | unlimited |
| API tokens | — | ✓ | ✓ | ✓ |
| Bulk import | — | — | ✓ | ✓ |
| Custom CSS | — | — | ✓ | ✓ |
| SSO / SAML | — | — | — | ✓ |
| SLA | — | — | 99.5% | 99.9% |
| Support | community | email | priority email | dedicated |

These are **placeholders for the founder to challenge** — they exist so
the billing schema and limit-enforcement code have a concrete target.

---

## 7. Candidate next features (post-v1 backlog)

Ranked by an entrepreneurial value/effort ratio, not by engineering
interest. Each candidate is written so a product manager can ask
"is this the next thing to build?" and get a defensible answer.

### 7.1 Live DJ mic-in over WebRTC  **[High value / High effort]**

Operators can go on-air with a browser mic, cross-fading over the
scheduled programme. Unlocks the "morning show" use case that hospitality
and community broadcasters actually pay for. Effort is high because it
adds a WebRTC gateway (Pion), a mixer stage in ffmpeg, and a talk-over
UI in the studio. Ships two customer segments (community broadcasters,
event radio).

### 7.2 Podcasting / RSS feed publishing  **[High value / Medium effort]**

Any station can also expose a podcast RSS feed of its recorded shows
(iTunes Connect-compatible). Zero new engineering on ingestion (we
already have processed audio); needs an RSS generator, an episode
model, and Apple/Spotify submission docs. Doubles the addressable
market — "internet radio + podcast host" beats "just internet radio"
in a pricing comparison.

### 7.3 Listener requests & voting  **[Medium value / Low effort]**

Listeners on the player can request tracks or up/down-vote what's
playing. Operators see aggregated results in the studio and choose
whether to act. Cheap engineering (a `requests` table, a REST endpoint,
an SSE event) that dramatically increases listener engagement metrics
and gives operators content for social posts ("your top-voted tracks
this week").

### 7.4 Ad insertion / dynamic swap  **[High value / High effort]**

Operators can define ad slots (pre-roll, mid-roll every N minutes) and
either upload creatives or plug in a programmatic ad server. Required
by any broadcaster who wants to monetise the stream directly. Effort
is high because it requires precise segment splicing in the playback
engine and an ad-serving abstraction; regulatory nuance around
targeting rules in different territories.

### 7.5 Now-playing webhooks / Discord / Slack  **[Medium value / Low effort]**

Every track change fires a webhook. Prebuilt integrations for Discord,
Slack, and generic HTTP. Community stations get free promotion into
their Discord servers; corporate stations post now-playing into
`#music` channels. Effort is a webhook subscription table + a
delivery worker with retries.

### 7.6 Smart speaker / Alexa / Google Home skills  **[Medium value / High effort]**

Listeners tell their smart speaker "play Acme Radio." Requires per-
platform certification, per-station SkillIDs, and a routing layer. High
effort per platform, but strategically important for hospitality
customers (hotel-room voice assistants).

### 7.7 AI-assisted programming  **[Low value / Medium effort]**

BPM matching, mood detection, "auto-DJ" for filler blocks between
scheduled shows. Nice differentiator; probably not the reason anyone
pays. Effort is medium: bring in a tag/classify pipeline (Whisper for
speech, a music-tagging model for audio features).

### 7.8 Native mobile apps  **[Medium value / Very high effort]**

iOS + Android native apps for both operators (studio) and listeners
(player). Deferred because the PWA covers 80% of it at 5% of the cost;
revisit when a mid-market customer signs an ACV that justifies a
$150k+ mobile investment.

### 7.9 Multi-language player UI  **[Medium value / Low effort]**

Player copy localised via i18next; operator picks default locale per
station; listener can override. Unlocks non-English markets. Real
work is the translations, not the code.

### 7.10 Embeddable partner widgets  **[Medium value / Low effort]**

A `<script src="…/embed.js">` snippet that mounts a styled player on
any site with `postMessage` control API. Turns every operator's blog /
press site into a distribution channel. Ships with Phase D branding
work naturally.

### 7.11 Analytics dashboard  **[High value / Medium effort]**

Beyond the operator-facing analytics in SRS §3.5, ship a public,
tenant-branded stats page ("we broadcast to 1.2M listeners this month
across 47 countries") that operators can embed on their marketing
site. Free growth loop.

### 7.12 Prioritisation framework

For each candidate, the founder decides on **three axes** before
scheduling engineering:

1. **Which target-segment(s)** does it unlock?
2. **Does it change the pricing tier ceiling** or move customers up-plan?
3. **What is the smallest shippable slice** (thin vertical, not full feature)?

The default sequencing that falls out of that framework today is:
**7.2 podcasting → 7.3 requests → 7.10 embed widget → 7.9 i18n → 7.5
webhooks → 7.1 live DJ → 7.4 ads → 7.6 smart speakers → 7.11 public
analytics → 7.7 AI → 7.8 native mobile.** This is a starting point,
not a commitment.

## 8. Cross-references

- Current-state critique: `docs/critique.md`
- Phased corrective roadmap: `docs/saas-roadmap.md`
- Operator deploy runbook (Cloud Run): `deploy/cloud-run.md`
- Hobbyist self-host install (upstream): `docs/installation.md`
- Upstream hobbyist roadmap (preserved for now): `docs/roadmap.md`

When code changes invalidate a requirement above, the requirement is
updated in the same PR that ships the change. Drift between this doc
and the running product is a defect.
