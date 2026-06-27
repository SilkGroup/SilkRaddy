// Package scale is the scaffold for Phase H (docs/saas-roadmap.md):
// horizontal scale-out for the API tier, ingest/API split via Cloud Run
// Jobs + Pub/Sub, leader election per (tenant_id, station_id) for the
// playback engine, multi-region replicas behind a global LB.
//
// This package is intentionally empty. The Phase H work refactors
// internal/playback into a Leader interface backed by a coordinator (LeaseDB
// row in Postgres, or a managed lock) and splits ffmpeg-heavy ingest into a
// separate Cloud Run service. Both are large enough that their own design
// docs land before any code does.
package scale
