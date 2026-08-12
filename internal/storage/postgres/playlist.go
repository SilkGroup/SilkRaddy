package postgres

import (
	"database/sql"
	"fmt"

	"github.com/cheatsnake/airstation/internal/pkg/ulid"
	"github.com/cheatsnake/airstation/internal/playlist"
	"github.com/cheatsnake/airstation/internal/track"
)

type PlaylistStore struct {
	db *sql.DB
}

func NewPlaylistStore(db *sql.DB) PlaylistStore {
	return PlaylistStore{db: db}
}

func (ps *PlaylistStore) AddPlaylist(name, description string, trackIDs []string) (*playlist.Playlist, error) {
	id := ulid.New()

	tx, err := ps.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO playlist (id, name, description) VALUES ($1, $2, $3)`,
		id, name, description,
	); err != nil {
		return nil, err
	}

	for position, trackID := range trackIDs {
		if _, err := tx.Exec(
			`INSERT INTO playlist_track (playlist_id, track_id, position) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			id, trackID, position,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return ps.Playlist(id)
}

func (ps *PlaylistStore) Playlists() ([]*playlist.Playlist, error) {
	rows, err := ps.db.Query(`
		SELECT p.id, p.name, p.description, COUNT(pt.track_id) AS track_count
		FROM playlist p
		LEFT JOIN playlist_track pt ON p.id = pt.playlist_id
		GROUP BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	playlists := make([]*playlist.Playlist, 0)
	for rows.Next() {
		var p playlist.Playlist
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.TrackCount); err != nil {
			return nil, err
		}
		p.Tracks = []*track.Track{}
		playlists = append(playlists, &p)
	}

	return playlists, nil
}

func (ps *PlaylistStore) Playlist(id string) (*playlist.Playlist, error) {
	p := playlist.Playlist{Tracks: make([]*track.Track, 0)}

	err := ps.db.QueryRow(
		`SELECT id, name, description FROM playlist WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Description)
	if err != nil {
		return nil, err
	}

	rows, err := ps.db.Query(`
		SELECT t.id, t.name, t.path, t.bit_rate, t.duration
		FROM playlist_track pt
		JOIN tracks t ON pt.track_id = t.id
		WHERE pt.playlist_id = $1
		ORDER BY pt.position`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query playlist tracks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t track.Track
		if err := rows.Scan(&t.ID, &t.Name, &t.Path, &t.BitRate, &t.Duration); err != nil {
			return nil, fmt.Errorf("failed to scan track: %w", err)
		}
		p.Tracks = append(p.Tracks, &t)
	}

	p.TrackCount = len(p.Tracks)
	return &p, nil
}

func (ps *PlaylistStore) IsPlaylistExists(name string) (bool, error) {
	var exists bool
	err := ps.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM playlist WHERE name = $1)`, name,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (ps *PlaylistStore) EditPlaylist(id, name, description string, trackIDs []string) error {
	tx, err := ps.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE playlist SET name = $1, description = $2 WHERE id = $3`,
		name, description, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM playlist_track WHERE playlist_id = $1`, id); err != nil {
		return err
	}

	for position, trackID := range trackIDs {
		if _, err := tx.Exec(
			`INSERT INTO playlist_track (playlist_id, track_id, position) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			id, trackID, position,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (ps *PlaylistStore) DeletePlaylist(id string) error {
	tx, err := ps.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM playlist_track WHERE playlist_id = $1`, id); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM playlist WHERE id = $1`, id); err != nil {
		return err
	}

	return tx.Commit()
}
