package auth

import (
	"strings"
	"testing"
)

func TestHashAndCompareRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Errorf("hash prefix = %q, want $argon2id$", h[:20])
	}
	if err := ComparePasswordAndHash("correct horse battery staple", h); err != nil {
		t.Errorf("match: %v", err)
	}
	if err := ComparePasswordAndHash("wrong password", h); err == nil {
		t.Errorf("expected mismatch error")
	}
}

func TestCompareRejectsMalformedHash(t *testing.T) {
	tests := []string{
		"",
		"$argon2id$",
		"$bcrypt$v=19$m=65536,t=2,p=1$abc$def",
		"not-a-hash",
	}
	for _, h := range tests {
		if err := ComparePasswordAndHash("x", h); err == nil {
			t.Errorf("ComparePasswordAndHash(%q) returned nil, want error", h)
		}
	}
}

func TestHashIsSalted(t *testing.T) {
	a, _ := HashPassword("same password")
	b, _ := HashPassword("same password")
	if a == b {
		t.Errorf("hashes are identical — salt is not being randomised")
	}
}

func TestRoleAllowed(t *testing.T) {
	cases := []struct {
		r    Role
		p    Permission
		want bool
	}{
		{RoleOwner, PermManageBilling, true},
		{RoleAdmin, PermManageBilling, false},
		{RoleAdmin, PermManageStation, true},
		{RoleDJ, PermManageStation, false},
		{RoleDJ, PermEditQueue, true},
		{RoleViewer, PermEditQueue, false},
		{RoleViewer, PermViewAnalytics, true},
	}
	for _, c := range cases {
		if got := c.r.Allowed(c.p); got != c.want {
			t.Errorf("Role(%s).Allowed(%s) = %v, want %v", c.r, c.p, got, c.want)
		}
	}
}
