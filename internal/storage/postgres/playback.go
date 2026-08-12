package postgres

import (
	"database/sql"
	"fmt"

	"github.com/cheatsnake/airstation/internal/playback"
)

type PlaybackStore struct {
	db *sql.DB
}

func NewPlaybackStore(db *sql.DB) PlaybackStore {
	return PlaybackStore{db: db}
}

func (ps *PlaybackStore) AddPlaybackHistory(playedAt int64, trackName string) error {
	_, err := ps.db.Exec(
		`INSERT INTO playback_history (played_at, track_name) VALUES ($1, $2)`,
		playedAt, trackName,
	)
	if err != nil {
		return fmt.Errorf("failed to insert playback entry: %v", err)
	}
	return nil
}

func (ps *PlaybackStore) RecentPlaybackHistory(limit int) ([]*playback.History, error) {
	rows, err := ps.db.Query(
		`SELECT id, played_at, track_name FROM playback_history ORDER BY played_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []*playback.History
	for rows.Next() {
		var item playback.History
		if err := rows.Scan(&item.ID, &item.PlayedAt, &item.TrackName); err != nil {
			return nil, err
		}
		history = append(history, &item)
	}
	return history, nil
}

func (ps *PlaybackStore) DeleteOldPlaybackHistory() (int64, error) {
	result, err := ps.db.Exec(
		`DELETE FROM playback_history WHERE played_at < (EXTRACT(EPOCH FROM NOW())::BIGINT - 30 * 24 * 60 * 60)`,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old entries: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %v", err)
	}

	return rowsAffected, nil
}
