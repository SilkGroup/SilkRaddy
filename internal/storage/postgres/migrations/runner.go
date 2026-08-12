package migrations

import (
	"database/sql"
	"fmt"
	"log/slog"
)

func RunMigrations(db *sql.DB, log *slog.Logger) error {
	migrationTableExists, err := tableExists(db, "migrations")
	if err != nil {
		return err
	}

	var currentVersion int

	if migrationTableExists {
		err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM migrations").Scan(&currentVersion)
		if err != nil {
			return fmt.Errorf("failed to get current version: %w", err)
		}
	} else {
		currentVersion = 0
		log.Info("Fresh database, starting migrations from the beginning")
	}

	for _, migration := range migrations {
		if migration.Version > currentVersion {
			log.Info("Applying migration", "version", migration.Version, "name", migration.Name)

			tx, err := db.Begin()
			if err != nil {
				return fmt.Errorf("failed to begin transaction for migration %d: %w",
					migration.Version, err)
			}

			defer func() {
				if tx != nil {
					tx.Rollback()
				}
			}()

			if err := migration.Up(tx); err != nil {
				return fmt.Errorf("migration %d (%s) failed: %w",
					migration.Version, migration.Name, err)
			}

			if migrationTableExists || migration.Version >= 1 {
				_, err = tx.Exec(
					"INSERT INTO migrations (version, name) VALUES ($1, $2)",
					migration.Version, migration.Name,
				)
				if err != nil {
					return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
				}
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
			}

			tx = nil

			log.Info("Migration applied successfully",
				"version", migration.Version,
				"name", migration.Name)
		}
	}

	var finalVersion int

	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM migrations").Scan(&finalVersion)
	if err != nil {
		return fmt.Errorf("failed to get final version: %w", err)
	}

	if finalVersion > currentVersion {
		log.Info("Database migration completed",
			"from_version", currentVersion,
			"to_version", finalVersion,
			"migrations_applied", finalVersion-currentVersion)
	} else {
		log.Info("Database is up to date", "version", finalVersion)
	}

	return nil
}

// tableExists checks for a table in the current schema using
// information_schema, which is the portable equivalent of sqlite's
// sqlite_master lookup.
func tableExists(db *sql.DB, tableName string) (bool, error) {
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = current_schema()
			  AND table_name = $1
		)`, tableName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}
	return exists, nil
}
