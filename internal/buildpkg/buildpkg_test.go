package buildpkg

import (
	"path/filepath"
	"strings"
	"testing"

	"vpa/internal/pkgbuild"
)

// safeJoin guards every PKGBUILD-controlled path. A hostile or malformed
// PKGBUILD must never be able to write outside the build directory.
func TestSafeJoinRejectsEscapes(t *testing.T) {
	base := "/home/u/.cache/vpa/build/pkg/src"
	bad := []string{
		"../../../../tmp/pwned",
		"../../evil",
		"..",
		"a/../../../etc/passwd",
		"/etc/passwd",
		"/tmp/absolute",
		"",
	}
	for _, name := range bad {
		if got, err := safeJoin(base, name); err == nil {
			t.Errorf("safeJoin(%q) allowed escape -> %q", name, got)
		}
	}
}

func TestSafeJoinAllowsNormalNames(t *testing.T) {
	base := "/home/u/.cache/vpa/build/pkg/src"
	good := map[string]string{
		"v1.2.3.tar.gz":      base + "/v1.2.3.tar.gz",
		"sub/dir/file.patch": base + "/sub/dir/file.patch",
		"repo.git":           base + "/repo.git",
		"./plain.txt":        base + "/plain.txt",
	}
	for name, want := range good {
		got, err := safeJoin(base, name)
		if err != nil {
			t.Errorf("safeJoin(%q) unexpectedly failed: %v", name, err)
			continue
		}
		if got != filepath.Clean(want) {
			t.Errorf("safeJoin(%q) = %q, want %q", name, got, want)
		}
	}
}

// A checksum field is used as a cache filename, so a value containing path
// separators must not be usable to write outside the cache.
func TestCacheRejectsTraversalChecksum(t *testing.T) {
	pb := &pkgbuild.PKGBUILD{Sha256sums: []string{"../../../../tmp/pwned"}}
	if _, ok := cachedSource(pb, 0, "/home/u/.cache/vpa/sources"); ok {
		t.Error("a checksum containing '..' must not resolve to a cache entry")
	}
	// Must not panic or write anywhere; nothing to assert beyond it returning.
	saveToCache(pb, 0, "/dev/null", "/home/u/.cache/vpa/sources")
}

func TestSourceCacheSumPrefersStrongest(t *testing.T) {
	pb := &pkgbuild.PKGBUILD{
		Sha512sums: []string{"SKIP"},
		Sha256sums: []string{"abc256"},
		B2sums:     []string{"b2"},
		Md5sums:    []string{"md5"},
	}
	if got := sourceCacheSum(pb, 0); got != "abc256" {
		t.Errorf("got %q, want the strongest non-SKIP sum (abc256)", got)
	}
	// All SKIP/missing: unverifiable sources must never be cached.
	pb2 := &pkgbuild.PKGBUILD{Sha256sums: []string{"SKIP"}}
	if got := sourceCacheSum(pb2, 0); got != "" {
		t.Errorf("got %q, want empty for an unverifiable source", got)
	}
	// Index beyond the array must not panic.
	if got := sourceCacheSum(pb, 5); got != "" {
		t.Errorf("out-of-range index gave %q", got)
	}
}

func TestNewDirsLayout(t *testing.T) {
	d := NewDirs("/build", "mypkg")
	if !strings.HasSuffix(d.Git, "/mypkg/git") ||
		!strings.HasSuffix(d.Src, "/mypkg/src") ||
		!strings.HasSuffix(d.Pkg, "/mypkg/pkg") {
		t.Errorf("unexpected layout: %+v", d)
	}
}

func TestAnyNonSkip(t *testing.T) {
	if anyNonSkip(nil) || anyNonSkip([]string{"SKIP"}) || anyNonSkip([]string{""}) {
		t.Error("expected false for empty/SKIP-only sums")
	}
	if !anyNonSkip([]string{"SKIP", "abc"}) {
		t.Error("expected true when a real checksum is present")
	}
}

func TestHasTarSuffix(t *testing.T) {
	for _, f := range []string{"a.tar", "a.tar.gz", "a.tgz", "a.tar.xz", "a.tar.zst", "a.tar.bz2"} {
		if !hasTarSuffix(f) {
			t.Errorf("hasTarSuffix(%q) = false", f)
		}
	}
	for _, f := range []string{"a.zip", "a.txt", "a.tar.gz.sig", ""} {
		if hasTarSuffix(f) {
			t.Errorf("hasTarSuffix(%q) = true", f)
		}
	}
}
