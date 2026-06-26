// Package postgres provides a Postgres-backed implementation of the
// storage.Storage interface for Cloud SQL deployments. It mirrors the sqlite
// package one-to-one so the driver can be swapped at startup via
// AIRSTATION_DB_DRIVER without changing application code.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/cheatsnake/airstation/internal/storage/postgres/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Instance struct {
	TrackStore
	QueueStore
	PlaybackStore
	PlaylistStore
	StationStore

	db  *sql.DB
	log *slog.Logger
}

// New opens a Postgres connection, runs migrations, and constructs an
// Instance that satisfies storage.Storage. dsn accepts either the URL form
// (postgres://user:pass@host:port/dbname?sslmode=disable) or pgx key-value
// form (host=/cloudsql/PROJECT:REGION:INSTANCE user=... dbname=...).
func New(dsn string, log *slog.Logger) (*Instance, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info("Postgres database connected")

	if err := migrations.RunMigrations(db, log); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	instance := &Instance{
		db:  db,
		log: log,
	}

	instance.TrackStore = NewTrackStore(db)
	instance.QueueStore = NewQueueStore(db)
	instance.PlaybackStore = NewPlaybackStore(db)
	instance.PlaylistStore = NewPlaylistStore(db)
	instance.StationStore = NewStationStore(db)

	return instance, nil
}

func (ins *Instance) Ping(ctx context.Context) error {
	return ins.db.PingContext(ctx)
}

func (ins *Instance) Close() error {
	return ins.db.Close()
}
