// Package compliance is the scaffold for Phase G (docs/saas-roadmap.md). It
// defines the Spinlog shape used by licensing-body reporting (ASCAP, BMI,
// PRS, SOCAN, SACEM) and provides a CSV exporter that is functional but
// uses a generic schema — the per-society templates land in Phase G proper.
package compliance

import (
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"time"
)

// Spin is one play event recorded by the playback engine. Every track that
// goes on air across the fleet produces one of these rows. The schema is
// the union of fields the major licensing bodies require.
type Spin struct {
	TenantID         string
	StationID        string
	StationName      string
	TerritoryISO     string // ISO 3166-1 alpha-2 of the station's country, for licensing splits.
	PlayedAt         time.Time
	TrackTitle       string
	TrackArtist      string
	TrackISRC        string // International Standard Recording Code, when known.
	DurationSeconds  float64
	ConcurrentListeners int
}

// ExportCSV writes spins as a generic CSV with one header row. The format
// is the same one tenants can re-shape into their licensing body's expected
// columns. Phase G adds per-society templates.
func ExportCSV(w io.Writer, spins []Spin) error {
	if w == nil {
		return errors.New("compliance: nil writer")
	}
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{
		"tenant_id", "station_id", "station_name", "territory",
		"played_at", "title", "artist", "isrc", "duration_seconds", "concurrent_listeners",
	}); err != nil {
		return err
	}
	for _, s := range spins {
		row := []string{
			s.TenantID,
			s.StationID,
			s.StationName,
			s.TerritoryISO,
			s.PlayedAt.UTC().Format(time.RFC3339),
			s.TrackTitle,
			s.TrackArtist,
			s.TrackISRC,
			strconv.FormatFloat(s.DurationSeconds, 'f', 3, 64),
			strconv.Itoa(s.ConcurrentListeners),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}
