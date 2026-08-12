package auth

import "time"

// Role enumerates the RBAC roles defined in the SRS §3.1.1 matrix.
// Higher tiers strictly include the permissions of lower tiers.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleDJ     Role = "dj"
	RoleViewer Role = "viewer"
)

// Permission is a capability gate. Handlers ask `s.Can(ctx, PermManageBilling)`
// instead of doing role string-matching themselves.
type Permission string

const (
	PermManageBilling Permission = "billing.manage"
	PermManageUsers   Permission = "users.manage"
	PermManageStation Permission = "station.manage"
	PermEditQueue     Permission = "queue.edit"
	PermUploadTrack   Permission = "track.upload"
	PermDeleteTrack   Permission = "track.delete"
	PermViewAnalytics Permission = "analytics.view"
)

// rolePermissions maps the RBAC matrix from docs/srs.md §3.1.1.
var rolePermissions = map[Role]map[Permission]bool{
	RoleOwner: {
		PermManageBilling: true, PermManageUsers: true, PermManageStation: true,
		PermEditQueue: true, PermUploadTrack: true, PermDeleteTrack: true,
		PermViewAnalytics: true,
	},
	RoleAdmin: {
		PermManageUsers: true, PermManageStation: true,
		PermEditQueue: true, PermUploadTrack: true, PermDeleteTrack: true,
		PermViewAnalytics: true,
	},
	RoleDJ: {
		PermEditQueue: true, PermUploadTrack: true, PermDeleteTrack: true,
		PermViewAnalytics: true,
	},
	RoleViewer: {
		PermViewAnalytics: true,
	},
}

// Allowed reports whether r grants p.
func (r Role) Allowed(p Permission) bool {
	return rolePermissions[r][p]
}

// Tenant is one customer workspace (a company, a community broadcaster, etc).
type Tenant struct {
	ID        string
	Name      string
	Slug      string
	CreatedAt time.Time
}

// User is an operator authenticated into the system. Listeners are not users.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	EmailVerified bool
	CreatedAt    time.Time
}

// Membership joins a User to a Tenant with a Role.
type Membership struct {
	UserID   string
	TenantID string
	Role     Role
}

// Session is the authenticated context carried on the request via the JWT
// cookie. Set by the auth middleware; read by RBAC and audit code.
type Session struct {
	UserID   string
	TenantID string
	Role     Role
}
