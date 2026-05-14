package postgres

import (
	"database/sql"
	"errors"

	"github.com/cheatsnake/airstation/internal/station"
)

type StationStore struct {
	db *sql.DB
}

func NewStationStore(db *sql.DB) StationStore {
	return StationStore{db: db}
}

func (ss *StationStore) StationProperties() ([]*station.Property, error) {
	rows, err := ss.db.Query(`SELECT key, value FROM station_properties ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var properties []*station.Property
	for rows.Next() {
		var prop station.Property
		if err := rows.Scan(&prop.Key, &prop.Value); err != nil {
			return nil, err
		}
		properties = append(properties, &prop)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return properties, nil
}

func (ss *StationStore) UpsertStationProperty(key, value string) (*station.Property, error) {
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}

	_, err := ss.db.Exec(`
		INSERT INTO station_properties (key, value)
		VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT`,
		key, value,
	)
	if err != nil {
		return nil, err
	}

	return &station.Property{Key: key, Value: value}, nil
}

func (ss *StationStore) DeleteStationProperty(key string) error {
	if key == "" {
		return errors.New("key cannot be empty")
	}

	result, err := ss.db.Exec(`DELETE FROM station_properties WHERE key = $1`, key)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("property not found")
	}

	return nil
}
