# Entrepreneurial critique — Silk Raddy

Posture: this document grades the product against the goal stated by the
founder: **a SaaS used by companies across the world to run their own
branded internet radio stations**. The original upstream (`airstation`)
positions itself explicitly as a self-hosted hobby tool ("Made for fun" —
see `README.md`). Almost every finding below stems from that gap: the
code is good *for what it was built to be* and is not yet shaped for
what it is being repositioned as.

Severity legend: **P0** = blocks the business; **P1** = blocks scale;
**P2** = blocks growth or NPS; **P3** = polish.

---

## 1 — Technical design

| # | Finding | Severity | Evidence |
| --- | --- | --- | --- |
| T1 | **Single-tenant data model.** There is no `tenant_id` or `organization_id` on any row. One database = one station. To serve N companies you have to deploy N copies. | **P0** | `internal/storage/sqlite/migrations/*.sql`, every `*.go` Store implementation |
| T2 | **No user accounts; single shared admin password.** Login is "send the `AIRSTATION_SECRET_KEY` env var as a password." No per-user identity, no audit log of who did what, no revocation without rotating the secret and re-deploying. | **P0** | `internal/http/handlers.go` `handleLogin`, `internal/config/config.go` `AIRSTATION_SECRET_KEY` |
| T3 | **No RBAC.** Every admin can do every admin thing. No "DJ can only manage queue, not delete tracks" or "Viewer can see analytics but not change anything." | **P0** | `internal/http/server.go` `jwtAuth` middleware (single tier) |
| T4 | **In-process singleton playback state.** `playback.State` lives in one Go process; the playback goroutine is the source of truth for which segment is on-air. You cannot horizontally scale, you cannot fail over, and `--max-instances 1` is a hard ceiling. | **P0** | `internal/playback/state.go`, `cmd/main.go` (one `httpServer.Run()`) |
| T5 | **Track and HLS storage on ephemeral container disk.** Already on the roadmap as Phase 2, but worth re-flagging — any cold start / new revision orphans every track row in the DB. | P0 (until Phase 2) | `static/tracks/`, `static/tmp/`, `internal/config/config.go` defaults |
| T6 | **No API contract.** No OpenAPI / Swagger / Protobuf; route table is hand-rolled at `server.go:62-91`. Companies integrating their CMS / ad server / EAS can't trust the surface. | P1 | `internal/http/server.go` |
| T7 | **No observability primitives.** No metrics endpoint, no traces, no `request_id` propagation, no SLO budget. `slog` output is human-readable text, not JSON, so it doesn't ingest cleanly into Cloud Logging / Datadog without a parser. | P1 | absence; `internal/logger/` |
| T8 | **No rate limiting, no abuse protection.** A public-facing multi-tenant service with public listener URLs and no per-tenant ingress controls is trivial to weaponize as bandwidth-amp / hotlinking. | P1 | absence |
| T9 | **No CI/CD.** No `.github/workflows/`, no test gate on PRs. The original code review for issue #26 surfaced a small but real bug; future regressions will land the same way. | P1 | absence |
| T10 | **Monolithic deployable.** Ingestion (ffmpeg, CPU-heavy, bursty) and streaming (HTTP segment serving, long-lived, lightweight) are the same binary. For a SaaS you want them split so the streaming tier scales by listener count and the ingest tier scales by upload volume — independently. | P2 | `cmd/main.go` |
| T11 | **No backup / DR plan.** Cloud SQL has snapshots if enabled, but there's no documented restore drill and no off-region copy. | P2 | absence |
| T12 | **Secrets validation is `log.Fatal`.** `AIRSTATION_JWT_SIGN`/`AIRSTATION_SECRET_KEY` missing-or-short → process exits. Fine for self-host, undignified for a managed service where startup errors should surface as structured logs and a non-zero exit code, not `log.Fatal` from a config package. | P3 | `internal/config/config.go` `getSecret` |
| T13 | **Player title hard-baked into the JS bundle at build time** (`AIRSTATION_PLAYER_TITLE` is a Dockerfile `ARG`). Per-tenant branding requires a rebuild per tenant. | P1 | `Dockerfile:5-7` |
| T14 | **CORS is wide-open (`cors.Default()`).** Acceptable for an embeddable player on someone else's site, but not configurable per-tenant. | P2 | `internal/http/server.go:104` |
| T15 | **No content protection.** HLS playlists and segments are reachable by anyone who guesses the URL. For commercial radio with licensed content, this is a hard licensing blocker. | P1 | `internal/http/handlers.go` `handleHLSPlaylist`, `handleStaticDir` |

## 2 — Business model

| # | Finding | Severity |
| --- | --- | --- |
| B1 | **No business model is encoded anywhere.** No `plans`, no `subscriptions`, no `usage_meters`. The product currently produces revenue of $0 and has no machinery to start. | **P0** |
| B2 | **No pricing tiers, no billing integration.** No Stripe / Paddle / Lemon Squeezy, no invoices, no tax handling. | **P0** |
| B3 | **No usage metering.** A SaaS for radio needs to meter: listener-hours, total ingest minutes, storage GB, custom-domain count, team-seat count. None exist. | **P0** |
| B4 | **No marketing / landing site.** The player IS the homepage. There is no "Sign up" CTA, no comparison table, no testimonials, no SEO surface. | P1 |
| B5 | **No trial mechanism.** There is no time-bounded free tier or feature gate. | P1 |
| B6 | **No customer support tooling.** No in-app help, no ticketing integration, no status page link. | P2 |
| B7 | **No legal scaffolding.** No ToS, no Privacy Policy, no DPA template, no DMCA takedown contact, no GDPR data-export endpoint. Required for paid B2B sales. | **P0** for sales |
| B8 | **No SLA / status page.** Enterprise buyers will not sign a contract without a published uptime SLA and a public status page. | P1 |
| B9 | **No music licensing reporting.** ASCAP / BMI / PRS / SOCAN / SACEM all require play logs by territory. Without this, the product cannot legally serve licensed music in most markets. | **P0** for any paying customer streaming licensed music |
| B10 | **No partner / referral / agency model.** Companies running multiple stations (event production, hospitality chains, retail) will not self-serve every one. | P2 |
| B11 | **Brand confusion: "Silk Raddy" vs `airstation` vs upstream `cheatsnake/airstation`.** Repo metadata, Go module path (`github.com/cheatsnake/airstation`), env var prefix (`AIRSTATION_*`), and Dockerfile artefacts still reference the upstream. Inconsistent branding hurts trust. | P2 |
| B12 | **No analytics / attribution.** No PostHog / Mixpanel / GA4 anywhere — you cannot answer "which marketing channel converts" or "where do users drop off in onboarding." | P1 |

## 3 — Self-serve UX

| # | Finding | Severity |
| --- | --- | --- |
| U1 | **No sign-up flow.** Onboarding today = clone the repo, write a `.env`, run `docker-compose up`. That is engineer-only. | **P0** |
| U2 | **No tenant provisioning.** Even if a sign-up form existed, there's no code path to create an isolated workspace for a new customer. | **P0** |
| U3 | **No custom domain support.** Companies want `radio.acme.com`, not `silkraddy.com/t/acme`. No DNS validation flow, no automatic TLS issuance. | **P0** |
| U4 | **No team invites / multi-user.** Same root cause as T2: there is no user model. | **P0** |
| U5 | **No self-serve password reset / account recovery / email verification.** Same root cause. | **P0** |
| U6 | **No 2FA.** Table stakes for B2B. | P1 |
| U7 | **No API tokens.** Companies cannot integrate from their own CMS / scheduler without a programmatic credential. | P1 |
| U8 | **No bulk import.** Cannot import an existing library from S3, Google Drive, Dropbox, or a CSV. Onboarding a station with 10k tracks is uploads-one-at-a-time. | P1 |
| U9 | **No scheduler.** Cannot say "play morning show 06:00–10:00, ads at :15 and :45." The roadmap mentions this as planned. | P1 |
| U10 | **No analytics surface for the operator.** No concurrent-listener graph, no track popularity, no geographic breakdown, no skip-rate. Operators need this to run a station. | P1 |
| U11 | **No white-label branding controls at runtime.** Logo, colours, player skin, custom CSS, player title are config-file or build-time, not in-app. | P1 |
| U12 | **No embeddable player widget.** Companies want a `<script>` snippet for their corporate site, not "go to our URL." | P1 |
| U13 | **No mobile app / PWA install prompt.** | P2 |
| U14 | **No notifications.** Stream went down? Disk filled? Track upload failed? Nothing emails or pages the operator. | P1 |
| U15 | **Studio (admin) UI is functional but minimal.** Acceptable for hobbyists, thin for paying operators who want a dashboard. | P2 |

---

## What's actually good

For balance, the parts that hold up under enterprise scrutiny:

- Clean Go package separation (`internal/track`, `internal/queue`, `internal/playback`, …) with `Store` interfaces — multi-backend swap (sqlite → postgres) was a clean drop-in.
- HLS pipeline is correctly thought-through: segment-duration rounding, fixed-size segments, listener buffer math. The hard streaming engineering is already done.
- The Cloud Run prep we just landed (Phase 0 + Phase 1) gives a real path to managed hosting without a rewrite.
- The frontends already ship as built static bundles — easy to put behind a CDN.
- Issue #26 fix shows the team applies sensible production patterns (don't let one bad file kill the loop).

The bones are good. The product around them needs to be built.
