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
				    applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
				);`,
				`CREATE TABLE IF NOT EXISTS tracks (
                    id TEXT PRIMARY KEY,
                    name TEXT NOT NULL UNIQUE,
                    path TEXT NOT NULL,
                    duration REAL NOT NULL,
                    bitRate INTEGER NOT NULL
                );`,
				`CREATE TABLE IF NOT EXISTS queue (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    track_id TEXT NOT NULL UNIQUE,
                    FOREIGN KEY (track_id) REFERENCES tracks (id)
                );`,
				`CREATE TABLE IF NOT EXISTS playback_history (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    played_at INTEGER NOT NULL,
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
                    created_at INTEGER DEFAULT (strftime('%s', 'now')),
                    updated_at INTEGER DEFAULT (strftime('%s', 'now'))
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
				`CREATE INDEX IF NOT EXISTS idx_tracks_name ON tracks (name COLLATE NOCASE);`,
				`CREATE INDEX IF NOT EXISTS idx_playback_history_played_at ON playback_history(played_at);`,
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
	{
		// Phase B foundation: tenants, users, memberships, api_tokens,
		// audit_log. The tables exist whether or not the binary is running
		// in multi-tenant mode; legacy single-tenant paths simply don't
		// reference them.
		Version: 3,
		Name:    "create_multi_tenant_tables",
		Up: func(tx *sql.Tx) error {
			queries := []string{
				`CREATE TABLE IF NOT EXISTS tenants (
				    id TEXT PRIMARY KEY,
				    name TEXT NOT NULL,
				    slug TEXT NOT NULL UNIQUE,
				    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
				);`,
				`CREATE TABLE IF NOT EXISTS users (
				    id TEXT PRIMARY KEY,
				    email TEXT NOT NULL UNIQUE,
				    password_hash TEXT NOT NULL,
				    email_verified INTEGER NOT NULL DEFAULT 0,
				    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
				);`,
				`CREATE TABLE IF NOT EXISTS memberships (
				    user_id TEXT NOT NULL,
				    tenant_id TEXT NOT NULL,
				    role TEXT NOT NULL,
				    PRIMARY KEY (user_id, tenant_id),
				    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
				    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
				);`,
				`CREATE TABLE IF NOT EXISTS api_tokens (
				    id TEXT PRIMARY KEY,
				    user_id TEXT NOT NULL,
				    token_hash TEXT NOT NULL UNIQUE,
				    scopes TEXT NOT NULL,
				    expires_at INTEGER,
				    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
				    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				);`,
				`CREATE TABLE IF NOT EXISTS audit_log (
				    id INTEGER PRIMARY KEY AUTOINCREMENT,
				    ts INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
				    actor_id TEXT,
				    tenant_id TEXT,
				    action TEXT NOT NULL,
				    resource TEXT,
				    old_value TEXT,
				    new_value TEXT,
				    ip TEXT
				);`,
				`CREATE INDEX IF NOT EXISTS idx_audit_log_tenant_ts ON audit_log(tenant_id, ts DESC);`,
				`CREATE INDEX IF NOT EXISTS idx_memberships_tenant ON memberships(tenant_id);`,
			}
			for _, q := range queries {
				if _, err := tx.Exec(q); err != nil {
					return fmt.Errorf("failed to execute query: %w, query: %s", err, q)
				}
			}
			return nil
		},
	},
}
