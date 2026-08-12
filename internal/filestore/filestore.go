// Package filestore abstracts byte storage so the rest of the codebase can
// stay agnostic about whether tracks and HLS segments live on a local
// filesystem (self-host, local dev) or in object storage (Cloud Storage,
// production).
//
// The interface is intentionally small: Put / Get / Delete / Exists / List /
// SignedURL. Anything richer (multipart, range, ACL, presigned uploads) is
// out of scope here and should land alongside the feature that needs it.
package filestore

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned by Get / Exists / Delete when the path has no
// object. Callers should treat it as a non-fatal condition where appropriate.
var ErrNotFound = errors.New("filestore: not found")

// ErrSigningNotSupported is returned by SignedURL when the backend does not
// expose objects via a directly-fetchable URL (typical for local-FS in
// production, where the application server proxies instead). Callers should
// fall back to a server-side stream.
var ErrSigningNotSupported = errors.New("filestore: signing not supported by backend")

// FileStore stores opaque byte sequences keyed by string paths. Paths use
// forward slashes regardless of OS, are case-sensitive, and must not start
// with a slash.
type FileStore interface {
	// Put writes the contents of r to the named path. Existing data at the
	// path is overwritten. The implementation is responsible for any
	// directory creation, ACL configuration, and content-type detection.
	Put(ctx context.Context, path string, r io.Reader) error

	// Get returns a reader for the named path. The caller must Close the
	// returned reader. Returns ErrNotFound if the path has no object.
	Get(ctx context.Context, path string) (io.ReadCloser, error)

	// Delete removes the named path. A missing path is not an error.
	Delete(ctx context.Context, path string) error

	// Exists reports whether the named path has an object.
	Exists(ctx context.Context, path string) (bool, error)

	// List returns the names of objects with the given prefix.
	List(ctx context.Context, prefix string) ([]string, error)

	// SignedURL returns a URL the caller can hand to an end-user to fetch
	// the object directly. ttl bounds the URL validity. Implementations
	// that do not support signing (e.g. plain local FS) should return
	// ErrSigningNotSupported so the caller can fall back to proxying.
	SignedURL(ctx context.Context, path string, ttl time.Duration) (string, error)
}
