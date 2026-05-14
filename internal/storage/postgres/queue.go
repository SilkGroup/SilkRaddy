package postgres

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/cheatsnake/airstation/internal/track"
)

type QueueStore struct {
	db *sql.DB
}

func NewQueueStore(db *sql.DB) QueueStore {
	return QueueStore{db: db}
}

func (qs *QueueStore) Queue() ([]*track.Track, error) {
	tracks := make([]*track.Track, 0, 10)

	rows, err := qs.db.Query(`
		SELECT t.id, t.name, t.path, t.duration, t.bit_rate
		FROM tracks t
		JOIN queue q ON t.id = q.track_id
		ORDER BY q.id ASC`)
	if err != nil {
		return tracks, fmt.Errorf("failed to query tracks in queue: %w", err)
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

func (qs *QueueStore) AddToQueue(tracks []*track.Track) error {
	for _, t := range tracks {
		_, err := qs.db.Exec(
			`INSERT INTO queue (track_id) VALUES ($1) ON CONFLICT (track_id) DO NOTHING`,
			t.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to add track to queue: %w", err)
		}
	}
	return nil
}

func (qs *QueueStore) RemoveFromQueue(trackIDs []string) error {
	for _, id := range trackIDs {
		if _, err := qs.db.Exec(`DELETE FROM queue WHERE track_id = $1`, id); err != nil {
			return fmt.Errorf("failed to remove track from queue: %w", err)
		}
	}
	return nil
}

func (qs *QueueStore) ReorderQueue(trackIDs []string) error {
	if _, err := qs.db.Exec(`DELETE FROM queue`); err != nil {
		return fmt.Errorf("failed to clear queue: %w", err)
	}

	for _, id := range trackIDs {
		if _, err := qs.db.Exec(`INSERT INTO queue (track_id) VALUES ($1)`, id); err != nil {
			return fmt.Errorf("failed to reorder queue: %w", err)
		}
	}
	return nil
}

func (qs *QueueStore) CurrentAndNextTrack() (*track.Track, *track.Track, error) {
	rows, err := qs.db.Query(`
		SELECT t.id, t.name, t.path, t.duration, t.bit_rate
		FROM tracks t
		JOIN queue q ON t.id = q.track_id
		ORDER BY q.id ASC
		LIMIT 2`)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query first and second tracks: %w", err)
	}
	defer rows.Close()

	var firstTrack, secondTrack track.Track
	count := 0

	for rows.Next() {
		if count == 0 {
			if err := rows.Scan(&firstTrack.ID, &firstTrack.Name, &firstTrack.Path, &firstTrack.Duration, &firstTrack.BitRate); err != nil {
				return nil, nil, fmt.Errorf("failed to scan first track: %w", err)
			}
		} else if count == 1 {
			if err := rows.Scan(&secondTrack.ID, &secondTrack.Name, &secondTrack.Path, &secondTrack.Duration, &secondTrack.BitRate); err != nil {
				return nil, nil, fmt.Errorf("failed to scan second track: %w", err)
			}
		}
		count++
	}

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	switch count {
	case 0:
		return nil, nil, nil
	case 1:
		return &firstTrack, &firstTrack, nil
	default:
		return &firstTrack, &secondTrack, nil
	}
}

func (qs *QueueStore) SpinQueue() error {
	tx, err := qs.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var firstTrackID string
	var firstTrackQueueID int64

	err = tx.QueryRow(`SELECT id, track_id FROM queue ORDER BY id ASC LIMIT 1`).
		Scan(&firstTrackQueueID, &firstTrackID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil // Queue is empty
		}
		return fmt.Errorf("failed to get first track: %w", err)
	}

	var maxID int64
	if err := tx.QueryRow(`SELECT MAX(id) FROM queue`).Scan(&maxID); err != nil {
		return fmt.Errorf("failed to get max ID: %w", err)
	}

	if _, err := tx.Exec(`UPDATE queue SET id = $1 WHERE id = $2`, maxID+1, firstTrackQueueID); err != nil {
		return fmt.Errorf("failed to update first track ID: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
