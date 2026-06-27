package auth

import (
	"context"
	"errors"
)

type ctxKey string

const ctxKeySession ctxKey = "auth.session"

// WithSession returns a derived context that carries s.
func WithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, ctxKeySession, s)
}

// FromContext returns the session attached to ctx, or an error if none is.
func FromContext(ctx context.Context) (*Session, error) {
	s, ok := ctx.Value(ctxKeySession).(*Session)
	if !ok || s == nil {
		return nil, errors.New("auth: no session on context")
	}
	return s, nil
}

// TenantID is the convenience accessor used by every store-layer call that
// must scope its query. Returns "" when no session is present (single-tenant
// mode); callers in multi-tenant mode should check for the empty string.
func TenantID(ctx context.Context) string {
	if s, err := FromContext(ctx); err == nil {
		return s.TenantID
	}
	return ""
}
