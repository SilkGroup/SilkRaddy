package compliance

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestExportCSVHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := ExportCSV(&buf, nil); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	got := buf.String()
	want := "tenant_id,station_id,station_name,territory,played_at,title,artist,isrc,duration_seconds,concurrent_listeners"
	if !strings.HasPrefix(got, want) {
		t.Errorf("CSV header = %q, want prefix %q", got, want)
	}
}

func TestExportCSVRow(t *testing.T) {
	var buf bytes.Buffer
	ts := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	err := ExportCSV(&buf, []Spin{{
		TenantID:            "t1",
		StationID:           "s1",
		StationName:         "Lobby Radio",
		TerritoryISO:        "UG",
		PlayedAt:            ts,
		TrackTitle:          "Song",
		TrackArtist:         "Artist",
		TrackISRC:           "USRC17607839",
		DurationSeconds:     180.5,
		ConcurrentListeners: 42,
	}})
	if err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	if !strings.Contains(buf.String(), "Lobby Radio") {
		t.Errorf("CSV missing station_name; got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "2026-01-02T12:00:00Z") {
		t.Errorf("CSV missing RFC3339 played_at; got %q", buf.String())
	}
}

func TestExportCSVNilWriter(t *testing.T) {
	if err := ExportCSV(nil, nil); err == nil {
		t.Errorf("expected error for nil writer")
	}
}
