package local

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cheatsnake/airstation/internal/filestore"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestPutGetRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	want := []byte("hello radio")

	if err := s.Put(ctx, "a/b/c.txt", bytes.NewReader(want)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	r, err := s.Get(ctx, "a/b/c.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer r.Close()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPutOverwrites(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Put(ctx, "x", bytes.NewReader([]byte("first"))); err != nil {
		t.Fatalf("Put 1: %v", err)
	}
	if err := s.Put(ctx, "x", bytes.NewReader([]byte("second"))); err != nil {
		t.Fatalf("Put 2: %v", err)
	}
	r, _ := s.Get(ctx, "x")
	defer r.Close()
	got, _ := io.ReadAll(r)
	if string(got) != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, filestore.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestExists(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ok, err := s.Exists(ctx, "nope")
	if err != nil || ok {
		t.Errorf("Exists(missing) = (%v, %v), want (false, nil)", ok, err)
	}

	_ = s.Put(ctx, "yep", bytes.NewReader([]byte("x")))
	ok, err = s.Exists(ctx, "yep")
	if err != nil || !ok {
		t.Errorf("Exists(present) = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestDeleteMissingIsNoError(t *testing.T) {
	s := newTestStore(t)
	if err := s.Delete(context.Background(), "never-existed"); err != nil {
		t.Errorf("Delete missing: %v", err)
	}
}

func TestDeleteRemoves(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_ = s.Put(ctx, "kill-me", bytes.NewReader([]byte("x")))
	if err := s.Delete(ctx, "kill-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ok, _ := s.Exists(ctx, "kill-me")
	if ok {
		t.Errorf("Exists after Delete = true")
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, p := range []string{"music/a.mp3", "music/b.mp3", "music/sub/c.mp3", "other/d.mp3"} {
		_ = s.Put(ctx, p, bytes.NewReader([]byte("x")))
	}
	got, err := s.List(ctx, "music/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("List len = %d, want 3 (got %v)", len(got), got)
	}
	for _, g := range got {
		if !startsWith(g, "music/") {
			t.Errorf("List returned non-prefix entry: %q", g)
		}
	}
}

func startsWith(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}

func TestSignedURLWithoutBase(t *testing.T) {
	s := newTestStore(t)
	_, err := s.SignedURL(context.Background(), "x", time.Minute)
	if !errors.Is(err, filestore.ErrSigningNotSupported) {
		t.Errorf("got %v, want ErrSigningNotSupported", err)
	}
}

func TestSignedURLWithBase(t *testing.T) {
	s, _ := New(t.TempDir(), "/static/tracks")
	got, err := s.SignedURL(context.Background(), "song.mp3", time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	want := "/static/tracks/song.mp3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
