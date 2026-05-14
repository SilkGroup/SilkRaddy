package migrations

import (
	"database/sql"
	"fmt"
)

type Migration struct {
	Version int
	Name    string
	Up      func(*sql.Tx) error
	Down    func(*sql.Tx) error
}

var migrations = []Migration{
	{
		Version: 1,
		Name:    "create_main_tables",
		Up: func(tx *sql.Tx) error {
			queries := []string{
				`CREATE TABLE IF NOT EXISTS migrations (
				    version INTEGER PRIMARY KEY,
				    name TEXT NOT NULL,
				    applied_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT
				);`,
				`CREATE TABLE IF NOT EXISTS tracks (
                    id TEXT PRIMARY KEY,
                    name TEXT NOT NULL UNIQUE,
                    path TEXT NOT NULL,
                    duration DOUBLE PRECISION NOT NULL,
                    bit_rate INTEGER NOT NULL
                );`,
				`CREATE TABLE IF NOT EXISTS queue (
                    id BIGSERIAL PRIMARY KEY,
                    track_id TEXT NOT NULL UNIQUE,
                    FOREIGN KEY (track_id) REFERENCES tracks (id)
                );`,
				`CREATE TABLE IF NOT EXISTS playback_history (
                    id BIGSERIAL PRIMARY KEY,
                    played_at BIGINT NOT NULL,
                    track_name TEXT NOT NULL
                );`,
				`CREATE TABLE IF NOT EXISTS playlist (
                    id TEXT PRIMARY KEY,
                    name TEXT NOT NULL UNIQUE,
                    description TEXT
                );`,
				`CREATE TABLE IF NOT EXISTS playlist_track (
                    playlist_id TEXT NOT NULL,
                    track_id TEXT NOT NULL,
                    position INTEGER NOT NULL,
                    FOREIGN KEY (playlist_id) REFERENCES playlist (id) ON DELETE CASCADE,
                    FOREIGN KEY (track_id) REFERENCES tracks (id),
                    PRIMARY KEY (playlist_id, position),
                    UNIQUE (playlist_id, track_id)
                );`,
				`CREATE TABLE IF NOT EXISTS station_properties (
                    key VARCHAR(100) PRIMARY KEY,
                    value TEXT,
                    created_at BIGINT DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT,
                    updated_at BIGINT DEFAULT EXTRACT(EPOCH FROM NOW())::BIGINT
                );`,
			}

			for _, query := range queries {
				if _, err := tx.Exec(query); err != nil {
					return fmt.Errorf("failed to execute query: %w, query: %s", err, query)
				}
			}
			return nil
		},
	},
	{
		Version: 2,
		Name:    "create_main_indexes",
		Up: func(tx *sql.Tx) error {
			indexes := []string{
				// Postgres equivalent of sqlite's `COLLATE NOCASE` index — index
				// the lower-cased name so ILIKE / LOWER(name) lookups can use it.
				`CREATE INDEX IF NOT EXISTS idx_tracks_name_lower ON tracks (LOWER(name));`,
				`CREATE INDEX IF NOT EXISTS idx_playback_history_played_at ON playback_history (played_at);`,
				`CREATE INDEX IF NOT EXISTS idx_playlist_track_ids ON playlist_track (playlist_id, track_id);`,
			}

			for _, query := range indexes {
				if _, err := tx.Exec(query); err != nil {
					return fmt.Errorf("failed to create index: %w, query: %s", err, query)
				}
			}
			return nil
		},
	},
}
