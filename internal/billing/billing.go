// Package billing is the scaffold for Phase E (docs/saas-roadmap.md). It
// declares the Plan, Entitlement, and Meter types referenced by SRS §6 and
// §3.6 so handler code in later PRs can be written against a stable surface.
// No Stripe integration is wired here yet; nothing in this package is
// invoked at runtime.
package billing

import "time"

// PlanID is the canonical identifier for one of the SRS-defined plans.
type PlanID string

const (
	PlanFree       PlanID = "free"
	PlanStarter    PlanID = "starter"
	PlanPro        PlanID = "pro"
	PlanEnterprise PlanID = "enterprise"
)

// Plan describes the limits and entitlements of a subscription tier.
// These mirror SRS §6. They live in code so handler-level enforcement is
// type-checked; the values are the source of truth for the marketing page.
type Plan struct {
	ID                  PlanID
	DisplayName         string
	StationLimit        int   // -1 = unlimited
	StorageBytesLimit   int64 // -1 = custom
	MonthlyListenerHrs  int
	CustomDomainLimit   int
	SeatLimit           int
	APITokens           bool
	BulkImport          bool
	CustomCSS           bool
	SSOSAML             bool
	SLA                 string
}

// Catalogue is the in-memory price book consulted by entitlement checks. It
// is intentionally a function rather than a top-level var so the values can
// be sourced from configuration in a follow-up.
func Catalogue() map[PlanID]Plan {
	return map[PlanID]Plan{
		PlanFree:    {ID: PlanFree, DisplayName: "Free", StationLimit: 1, StorageBytesLimit: 1 << 30, MonthlyListenerHrs: 100, SeatLimit: 1},
		PlanStarter: {ID: PlanStarter, DisplayName: "Starter", StationLimit: 1, StorageBytesLimit: 25 << 30, MonthlyListenerHrs: 5_000, CustomDomainLimit: 1, SeatLimit: 3, APITokens: true},
		PlanPro:     {ID: PlanPro, DisplayName: "Pro", StationLimit: 5, StorageBytesLimit: 250 << 30, MonthlyListenerHrs: 50_000, CustomDomainLimit: 5, SeatLimit: 15, APITokens: true, BulkImport: true, CustomCSS: true, SLA: "99.5%"},
		PlanEnterprise: {ID: PlanEnterprise, DisplayName: "Enterprise", StationLimit: -1, StorageBytesLimit: -1, MonthlyListenerHrs: -1, CustomDomainLimit: -1, SeatLimit: -1, APITokens: true, BulkImport: true, CustomCSS: true, SSOSAML: true, SLA: "99.9%"},
	}
}

// Subscription is a tenant's current billing state. Populated by the Stripe
// webhook handler when Phase E lands; today its fields are unused at runtime.
type Subscription struct {
	TenantID         string
	Plan             PlanID
	StripeCustomerID string
	CurrentPeriodEnd time.Time
	CancelAtPeriodEnd bool
}

// Meter is one billable usage dimension reported to Stripe nightly.
type Meter string

const (
	MeterListenerHours  Meter = "listener_hours"
	MeterStorageGBMonth Meter = "storage_gb_month"
	MeterIngestMinutes  Meter = "ingest_minutes"
	MeterSeats          Meter = "seats"
	MeterCustomDomains  Meter = "custom_domains"
)
