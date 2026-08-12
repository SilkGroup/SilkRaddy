// Package scheduler is the scaffold for Phase F (docs/saas-roadmap.md). It
// defines the Programme / Block / RecurrenceRule shapes the runtime resolver
// will use, but contains no playback wiring. Today this package is unused at
// runtime; it exists so the SRS-referenced shapes are pinned in code.
package scheduler

import "time"

// Programme is a scheduled block of audio that overrides the manual queue
// when its window is active. Per SRS FR-STN-5 / FR-STN-6.
type Programme struct {
	ID        string
	TenantID  string
	StationID string
	Name      string
	// Source — playlist ID or track ID; resolver picks based on Type.
	SourceID string
	Type     ProgrammeType
	// Recurrence may be nil for one-off programmes.
	Recurrence *RecurrenceRule
	StartsAt   time.Time
	EndsAt     time.Time
	// Exclusive blocks override the queue; additive blocks mix in.
	Exclusive bool
}

// ProgrammeType distinguishes the kind of source backing a Programme.
type ProgrammeType string

const (
	ProgrammeTypePlaylist ProgrammeType = "playlist"
	ProgrammeTypeTrack    ProgrammeType = "track"
)

// RecurrenceRule mirrors a subset of RFC 5545 RRULE — enough for daily,
// weekly, and weekday/weekend patterns. Anything richer can be added when a
// customer asks for it.
type RecurrenceRule struct {
	Freq     Frequency
	ByDay    []time.Weekday
	Interval int
	Until    *time.Time
}

// Frequency enumerates the supported recurrence cadences.
type Frequency string

const (
	FreqDaily  Frequency = "daily"
	FreqWeekly Frequency = "weekly"
)

// Resolver picks the Programme that should be on-air at t for a given
// station, or returns nil if the manual queue should play. Implementation
// lands in Phase F.
type Resolver interface {
	OnAirAt(stationID string, t time.Time) (*Programme, error)
}
