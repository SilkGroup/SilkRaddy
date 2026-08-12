package postgres

import (
	"database/sql"
	"errors"
	"fmt"

	sqltool "github.com/cheatsnake/airstation/internal/pkg/sql"
	"github.com/cheatsnake/airstation/internal/pkg/ulid"
	"github.com/cheatsnake/airstation/internal/track"
)

type TrackStore struct {
	db *sql.DB
}

func NewTrackStore(db *sql.DB) TrackStore {
	return TrackStore{db: db}
}

func (ts *TrackStore) Tracks(page, limit int, search, sortBy, sortOrder string) ([]*track.Track, int, error) {
	tracks := make([]*track.Track, 0, limit)

	countQuery := "SELECT COUNT(*) FROM tracks"
	var countArgs []interface{}
	if search != "" {
		countQuery += " WHERE name ILIKE $1"
		countArgs = append(countArgs, "%"+search+"%")
	}

	var total int
	if err := ts.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return tracks, 0, fmt.Errorf("failed to get total track count: %w", err)
	}

	args := []interface{}{}
	nextArg := func(v interface{}) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	query := "SELECT id, name, path, duration, bit_rate FROM tracks"
	if search != "" {
		query += " WHERE name ILIKE " + nextArg("%"+search+"%")
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)
	query += " LIMIT " + nextArg(limit)
	query += " OFFSET " + nextArg((page-1)*limit)

	rows, err := ts.db.Query(query, args...)
	if err != nil {
		return tracks, 0, fmt.Errorf("failed to query tracks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t track.Track
		if err := rows.Scan(&t.ID, &t.Name, &t.Path, &t.Duration, &t.BitRate); err != nil {
			return tracks, 0, fmt.Errorf("failed to scan track: %w", err)
		}
		tracks = append(tracks, &t)
	}

	if err = rows.Err(); err != nil {
		return tracks, 0, fmt.Errorf("error iterating over rows: %w", err)
	}

	return tracks, total, nil
}

func (ts *TrackStore) AddTrack(name, path string, duration float64, bitRate int) (*track.Track, error) {
	t := &track.Track{
		ID:       ulid.New(),
		Name:     name,
		Path:     path,
		Duration: duration,
		BitRate:  bitRate,
	}

	_, err := ts.db.Exec(
		`INSERT INTO tracks (id, name, path, duration, bit_rate) VALUES ($1, $2, $3, $4, $5)`,
		t.ID, t.Name, t.Path, t.Duration, t.BitRate,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert track: %w", err)
	}

	return t, nil
}

func (ts *TrackStore) DeleteTracks(IDs []string) error {
	for _, id := range IDs {
		if _, err := ts.db.Exec(`DELETE FROM tracks WHERE id = $1`, id); err != nil {
			return fmt.Errorf("failed to delete track with ID %s: %w", id, err)
		}
	}
	return nil
}

func (ts *TrackStore) EditTrack(t *track.Track) (*track.Track, error) {
	_, err := ts.db.Exec(
		`UPDATE tracks SET name = $1, path = $2, duration = $3, bit_rate = $4 WHERE id = $5`,
		t.Name, t.Path, t.Duration, t.BitRate, t.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update track: %w", err)
	}
	return t, nil
}

func (ts *TrackStore) TrackByID(ID string) (*track.Track, error) {
	var t track.Track
	err := ts.db.QueryRow(
		`SELECT id, name, path, duration, bit_rate FROM tracks WHERE id = $1`, ID,
	).Scan(&t.ID, &t.Name, &t.Path, &t.Duration, &t.BitRate)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("track with ID %s not found", ID)
		}
		return nil, fmt.Errorf("failed to scan track: %w", err)
	}
	return &t, nil
}

func (ts *TrackStore) TracksByIDs(IDs []string) ([]*track.Track, error) {
	tracks := make([]*track.Track, 0, len(IDs))

	whereClause := sqltool.BuildInClausePg("id", len(IDs))
	query := fmt.Sprintf("SELECT id, name, path, duration, bit_rate FROM tracks WHERE %s", whereClause)
	args := make([]interface{}, len(IDs))
	for i, id := range IDs {
		args[i] = id
	}

	rows, err := ts.db.Query(query, args...)
	if err != nil {
		return tracks, fmt.Errorf("failed to query tracks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t track.Track
		if err := rows.Scan(&t.ID, &t.Name, &t.Path, &t.Duration, &t.BitRate); err != nil {
			return tracks, fmt.Errorf("failed to scan track: %w", err)
		}
		tracks = append(tracks, &t)
	}

	if err = rows.Err(); err != nil {
		return tracks, fmt.Errorf("error iterating over rows: %w", err)
	}

	return tracks, nil
}
