package http

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeUploadNameStripsPath(t *testing.T) {
	cases := map[string]string{
		"song.mp3":                  "song.mp3",
		"../../etc/passwd":          "passwd",
		"foo/bar/baz.aac":           "baz.aac",
		"C:\\Users\\evil\\ok.flac":  "C:\\Users\\evil\\ok.flac", // filepath.Base is POSIX on linux — Windows backslashes are treated as name chars, which is fine because filepath.Join then puts them inside TracksDir.
		"":                          "",
		".":                         "",
		"/":                         "",
		"only-name.mp3":             "only-name.mp3",
	}
	for input, want := range cases {
		if got := safeUploadName(input); got != want {
			t.Errorf("safeUploadName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUniquePathReturnsOriginalWhenFree(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mp3")
	if got := uniquePath(p); got != p {
		t.Errorf("uniquePath on free path returned %q, want %q", got, p)
	}
}

func TestUniquePathAvoidsCollision(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "clash.mp3")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := uniquePath(p)
	if got == p {
		t.Errorf("uniquePath returned original despite collision")
	}
	if filepath.Ext(got) != ".mp3" {
		t.Errorf("uniquePath dropped the extension: %q", got)
	}
	if filepath.Dir(got) != dir {
		t.Errorf("uniquePath escaped the target dir: %q", got)
	}
}

func TestShortRandIsAlphanumericAndSixChars(t *testing.T) {
	s := shortRand()
	if len(s) != 6 {
		t.Errorf("len = %d, want 6 (got %q)", len(s), s)
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			t.Errorf("non-alphanumeric char %q in %q", r, s)
		}
	}
}

func TestSplitAndTrim(t *testing.T) {
	got := splitAndTrim(" a, b ,c,,  d  ", ",")
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMaxInt(t *testing.T) {
	cases := []struct{ a, b, want int }{
		{3, 5, 5},
		{-1, -2, 1},
		{0, 4, 4},
		{7, 7, 7},
	}
	for _, c := range cases {
		if got := maxInt(c.a, c.b); got != c.want {
			t.Errorf("maxInt(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
