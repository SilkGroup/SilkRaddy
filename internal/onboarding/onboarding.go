// Package onboarding is the scaffold for Phase D (docs/saas-roadmap.md):
// sign-up, custom domain, white-label branding. The handler stubs here
// return 501 Not Implemented and are not registered in the HTTP server.
// They exist so Phase D PRs can be reviewed as deltas rather than as
// "everything new."
package onboarding

import (
	"errors"
	"net/http"
)

// ErrNotImplemented is returned by every scaffold endpoint until Phase D
// supplies a real handler.
var ErrNotImplemented = errors.New("onboarding: not implemented yet — Phase D")

// SignupRequest is the shape the public sign-up form posts.
type SignupRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	StationName  string `json:"stationName"`
	StationSlug  string `json:"stationSlug"`
	TermsAccepted bool  `json:"termsAccepted"`
}

// CustomDomainRequest is the shape the studio posts when an operator adds a
// custom hostname.
type CustomDomainRequest struct {
	Hostname string `json:"hostname"`
}

// HandleSignup is the scaffold. Wired in Phase D.
func HandleSignup(w http.ResponseWriter, r *http.Request) {
	http.Error(w, ErrNotImplemented.Error(), http.StatusNotImplemented)
}

// HandleAddCustomDomain is the scaffold. Wired in Phase D.
func HandleAddCustomDomain(w http.ResponseWriter, r *http.Request) {
	http.Error(w, ErrNotImplemented.Error(), http.StatusNotImplemented)
}
