// Package gcs provides a FileStore backed by Google Cloud Storage.
//
// Authentication uses Application Default Credentials, which on Cloud Run is
// transparently the runtime service account. SignedURL uses V4 signing via
// the IAM Credentials SignBlob API so no private-key JSON file needs to be
// mounted into the container — the runtime service account just needs:
//
//   - roles/storage.objectAdmin on the bucket (for Put/Get/Delete/List)
//   - roles/iam.serviceAccountTokenCreator on itself (for SignBlob)
//
// For local development with a JSON key file, set GOOGLE_APPLICATION_CREDENTIALS
// and the SDK will use the private key directly without round-tripping IAM.
package gcs

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/cheatsnake/airstation/internal/filestore"
	"google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Store implements filestore.FileStore against a single GCS bucket.
type Store struct {
	client    *storage.Client
	bucket    *storage.BucketHandle
	name      string
	iamClient *iamcredentials.Service

	// serviceAccount is the principal whose key the SignBlob API uses to
	// sign V4 URLs. Empty means SignedURL will error until New is called
	// with one. On Cloud Run this should be the runtime SA email
	// (PROJECT_NUMBER-compute@developer.gserviceaccount.com or a custom SA
	// attached to the service).
	serviceAccount string
}

// New opens a GCS client and verifies the bucket is reachable.
// serviceAccount is the email used as GoogleAccessID for V4 URL signing;
// it must be set if SignedURL will be called. Plain Put / Get / Delete /
// Exists / List work without it.
func New(ctx context.Context, bucket, serviceAccount string, opts ...option.ClientOption) (*Store, error) {
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("storage.NewClient: %w", err)
	}
	bh := client.Bucket(bucket)
	if _, err := bh.Attrs(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("bucket %q not reachable: %w", bucket, err)
	}
	iamSvc, err := iamcredentials.NewService(ctx, opts...)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("iamcredentials.NewService: %w", err)
	}
	return &Store{
		client:         client,
		bucket:         bh,
		name:           bucket,
		serviceAccount: serviceAccount,
		iamClient:      iamSvc,
	}, nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

func (s *Store) Put(ctx context.Context, path string, r io.Reader) error {
	w := s.bucket.Object(path).NewWriter(ctx)
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return fmt.Errorf("gcs put %q: %w", path, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("gcs put %q close: %w", path, err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	r, err := s.bucket.Object(path).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, filestore.ErrNotFound
		}
		return nil, fmt.Errorf("gcs get %q: %w", path, err)
	}
	return r, nil
}

func (s *Store) Delete(ctx context.Context, path string) error {
	err := s.bucket.Object(path).Delete(ctx)
	if err == nil || errors.Is(err, storage.ErrObjectNotExist) {
		return nil
	}
	return fmt.Errorf("gcs delete %q: %w", path, err)
}

func (s *Store) Exists(ctx context.Context, path string) (bool, error) {
	_, err := s.bucket.Object(path).Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("gcs exists %q: %w", path, err)
}

func (s *Store) List(ctx context.Context, prefix string) ([]string, error) {
	it := s.bucket.Objects(ctx, &storage.Query{Prefix: prefix})
	var out []string
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("gcs list %q: %w", prefix, err)
		}
		out = append(out, attrs.Name)
	}
	return out, nil
}

// SignedURL returns a V4-signed GET URL. Signing is performed via the IAM
// SignBlob API so the runtime never needs a private key on disk.
func (s *Store) SignedURL(ctx context.Context, path string, ttl time.Duration) (string, error) {
	if s.serviceAccount == "" {
		return "", fmt.Errorf("gcs signing requires a service account email; pass it to gcs.New")
	}
	opts := &storage.SignedURLOptions{
		Scheme:         storage.SigningSchemeV4,
		Method:         "GET",
		GoogleAccessID: s.serviceAccount,
		Expires:        time.Now().Add(ttl),
		SignBytes: func(b []byte) ([]byte, error) {
			resp, err := s.iamClient.Projects.ServiceAccounts.SignBlob(
				"projects/-/serviceAccounts/"+s.serviceAccount,
				&iamcredentials.SignBlobRequest{Payload: base64.StdEncoding.EncodeToString(b)},
			).Context(ctx).Do()
			if err != nil {
				return nil, err
			}
			return base64.StdEncoding.DecodeString(strings.TrimSpace(resp.SignedBlob))
		},
	}
	return s.bucket.SignedURL(path, opts)
}
