// Package local provides a FileStore backed by a local filesystem root.
// It is the implementation used by self-hosted deployments and by local
// development.
package local

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cheatsnake/airstation/internal/filestore"
)

// Store implements filestore.FileStore against a local directory root.
//
// Paths handed to the interface are joined onto Root with filepath.Join so
// callers see the same forward-slash-separated keys regardless of host OS.
// PublicBaseURL, when set, lets SignedURL return a relative URL the existing
// static-file handler can serve (e.g. "/static/tracks/<path>"); when empty,
// SignedURL returns ErrSigningNotSupported so callers proxy through the app.
type Store struct {
	Root          string
	PublicBaseURL string
}

// New returns a Store rooted at root. The directory is created if missing.
func New(root, publicBaseURL string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{Root: root, PublicBaseURL: strings.TrimRight(publicBaseURL, "/")}, nil
}

func (s *Store) full(path string) string {
	return filepath.Join(s.Root, filepath.FromSlash(path))
}

func (s *Store) Put(ctx context.Context, path string, r io.Reader) error {
	full := s.full(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (s *Store) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	f, err := os.Open(s.full(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, filestore.ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

func (s *Store) Delete(ctx context.Context, path string) error {
	err := os.Remove(s.full(path))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) Exists(ctx context.Context, path string) (bool, error) {
	_, err := os.Stat(s.full(path))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s *Store) List(ctx context.Context, prefix string) ([]string, error) {
	fullPrefix := s.full(prefix)
	root := fullPrefix
	if info, err := os.Stat(fullPrefix); err != nil || !info.IsDir() {
		// Prefix isn't a directory — walk its parent and filter.
		root = filepath.Dir(fullPrefix)
	}
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.Root, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) {
			out = append(out, key)
		}
		return nil
	})
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return out, err
}

func (s *Store) SignedURL(ctx context.Context, path string, ttl time.Duration) (string, error) {
	if s.PublicBaseURL == "" {
		return "", filestore.ErrSigningNotSupported
	}
	return s.PublicBaseURL + "/" + strings.TrimLeft(path, "/"), nil
}
