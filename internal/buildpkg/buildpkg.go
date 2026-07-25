// Package buildpkg fetches PKGBUILD sources, verifies checksums, and runs
// prepare()/build() and package()/package_<name>() via small bash drivers
// (build()/package() are themselves bash, so this unavoidably shells out).
package buildpkg

import (
	_ "embed"
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"vur/internal/gitutil"
	"vur/internal/pkgbuild"
	"vur/internal/sysutil"
	"vur/internal/ui"
)

//go:embed driver-build.sh
var driverBuild []byte

//go:embed driver-package.sh
var driverPackage []byte

// Dirs is the per-pkgbase directory layout: $BuildDir/<pkgbase>/{git,src,pkg/<name>}.
type Dirs struct {
	Git string
	Src string
	Pkg string // parent of per-pkgname pkgdirs
}

func NewDirs(buildDir, pkgbase string) Dirs {
	root := filepath.Join(buildDir, pkgbase)
	return Dirs{Git: filepath.Join(root, "git"), Src: filepath.Join(root, "src"), Pkg: filepath.Join(root, "pkg")}
}

// Shared transport with generous connection reuse/idle limits: multiple
// concurrent source downloads (possibly from the same host, e.g. GitHub
// release assets) benefit from keep-alive instead of each opening a fresh
// TCP+TLS handshake, which is what happens with the old one-`curl`-process-
// per-download approach this replaces.
var httpClient = &http.Client{
	Timeout: 30 * time.Minute,
	Transport: &http.Transport{
		MaxIdleConns:        64,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	},
}

// FetchSources populates a fresh srcdir from pb.Source, downloading/cloning
// up to `parallel` entries at once (each source entry writes to its own
// distinct filename, so concurrent entries can't collide). parallel < 1 is
// treated as 1 (fully serial). Verified downloads are cached in cacheDir
// keyed by checksum, so rebuilds (a failed build retried, `upgrade`
// rebuilding the same version, etc.) skip re-downloading unchanged sources.
func FetchSources(pkgbase string, pb *pkgbuild.PKGBUILD, d Dirs, parallel int, cacheDir string) error {
	os.RemoveAll(d.Src)
	if err := os.MkdirAll(d.Src, 0o755); err != nil {
		return err
	}
	if parallel < 1 {
		parallel = 1
	}
	sourceCache := filepath.Join(cacheDir, "sources")
	os.MkdirAll(sourceCache, 0o755)

	// Check build-time deps once upfront rather than racing RequireBin's
	// confirm-and-install prompt across goroutines.
	for _, entry := range pb.Source {
		_, url := pkgbuild.SplitSourceEntry(entry)
		if strings.HasPrefix(url, "git+") {
			sysutil.RequireBin("git", "git")
		}
	}

	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	errCh := make(chan error, len(pb.Source))

	for i, entry := range pb.Source {
		if entry == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, entry string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fetchOneSource(pkgbase, pb, d, i, entry, sourceCache); err != nil {
				errCh <- err
			}
		}(i, entry)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		return err
	}
	return nil
}

func fetchOneSource(pkgbase string, pb *pkgbuild.PKGBUILD, d Dirs, i int, entry, sourceCache string) error {
	fname, url := pkgbuild.SplitSourceEntry(entry)

	switch {
	case strings.HasPrefix(url, "git+"):
		return fetchGitSource(url, fname, d.Src)
	case strings.Contains(url, "://"):
		dest := filepath.Join(d.Src, fname)
		if cached, ok := cachedSource(pb, i, sourceCache); ok {
			if err := copyFile(cached, dest); err == nil && verifyChecksum(pb, i, fname, d.Src) == nil {
				maybeExtract(pb, fname, d.Src)
				return nil
			}
			os.Remove(dest) // cache entry didn't verify; fall through to a real download
		}
		ui.Info("downloading %s", fname)
		if err := download(url, dest); err != nil {
			return fmt.Errorf("failed to download %s: %w", fname, err)
		}
		if err := verifyChecksum(pb, i, fname, d.Src); err != nil {
			return err
		}
		saveToCache(pb, i, dest, sourceCache)
		maybeExtract(pb, fname, d.Src)
	default:
		srcPath := filepath.Join(d.Git, url)
		if _, err := os.Stat(srcPath); err != nil {
			return fmt.Errorf("%s: local source '%s' not found in checkout", pkgbase, url)
		}
		if err := copyFile(srcPath, filepath.Join(d.Src, fname)); err != nil {
			return err
		}
		if err := verifyChecksum(pb, i, fname, d.Src); err != nil {
			return err
		}
		maybeExtract(pb, fname, d.Src)
	}
	return nil
}

// sourceCacheSum returns the strongest available non-SKIP checksum for
// source index i, used as the cache key -- same strength order as
// verifyChecksum. Empty if none is declared (unverifiable sources are never
// cached, since there'd be nothing safe to key them by).
func sourceCacheSum(pb *pkgbuild.PKGBUILD, i int) string {
	for _, sums := range [][]string{pb.Sha512sums, pb.Sha256sums, pb.B2sums, pb.Md5sums} {
		if i < len(sums) && sums[i] != "" && sums[i] != "SKIP" {
			return sums[i]
		}
	}
	return ""
}

func cachedSource(pb *pkgbuild.PKGBUILD, i int, sourceCache string) (string, bool) {
	sum := sourceCacheSum(pb, i)
	if sum == "" {
		return "", false
	}
	path := filepath.Join(sourceCache, sum)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

func saveToCache(pb *pkgbuild.PKGBUILD, i int, file, sourceCache string) {
	sum := sourceCacheSum(pb, i)
	if sum == "" {
		return
	}
	copyFile(file, filepath.Join(sourceCache, sum))
}

func fetchGitSource(url, fname, srcdir string) error {
	real := strings.TrimPrefix(url, "git+")
	base, frag, _ := strings.Cut(real, "#")
	ref := ""
	if frag != "" {
		_, ref, _ = strings.Cut(frag, "=")
	}
	ui.Info("cloning %s", fname)
	return gitutil.CloneWorkingCopy(base, filepath.Join(srcdir, fname), ref)
}

// download fetches url to dest using a shared, connection-reusing HTTP
// client (native Go, no per-download `curl` subprocess) with a few retries
// for transient failures.
func download(url, dest string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		if err := downloadOnce(url, dest); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func downloadOnce(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// verifyChecksum uses whichever of Sha512sums/Sha256sums/B2sums/Md5sums has
// a non-SKIP entry at index i, strongest first, shelling out to the
// matching coreutils *sum tool (avoids adding a blake2b module dependency
// just for b2sum -- keeps `go build` stdlib-only/fully offline-capable).
func verifyChecksum(pb *pkgbuild.PKGBUILD, i int, fname, dir string) error {
	type sumSrc struct {
		tool string
		sums []string
	}
	for _, s := range []sumSrc{
		{"sha512sum", pb.Sha512sums}, {"sha256sum", pb.Sha256sums},
		{"b2sum", pb.B2sums}, {"md5sum", pb.Md5sums},
	} {
		if i >= len(s.sums) {
			continue
		}
		want := s.sums[i]
		if want == "" || want == "SKIP" {
			continue
		}
		sysutil.RequireBin(s.tool, "coreutils")
		out, err := sysutil.Output(s.tool, filepath.Join(dir, fname))
		if err != nil {
			return fmt.Errorf("%s: failed to run %s: %w", fname, s.tool, err)
		}
		got := strings.Fields(out)[0]
		if got != want {
			return fmt.Errorf("%s: %s mismatch (expected %s, got %s)", fname, s.tool, want, got)
		}
		return nil
	}
	ui.Warn("%s: no checksum available to verify (all SKIP/missing) -- trusting download as-is", fname)
	return nil
}

func isNoExtract(pb *pkgbuild.PKGBUILD, fname string) bool {
	for _, n := range pb.NoExtract {
		if n == fname {
			return true
		}
	}
	return false
}

func maybeExtract(pb *pkgbuild.PKGBUILD, fname, dir string) {
	if isNoExtract(pb, fname) {
		return
	}
	full := filepath.Join(dir, fname)
	switch {
	case strings.HasSuffix(fname, ".zip"):
		ui.Info("extracting %s", fname)
		if err := extractZip(full, dir); err != nil {
			ui.Warn("failed to extract %s: %v", fname, err)
		}
	case hasTarSuffix(fname):
		ui.Info("extracting %s", fname)
		if err := sysutil.RunQuiet("tar", "-xf", full, "-C", dir); err != nil {
			ui.Warn("failed to extract %s: %v", fname, err)
		}
	}
}

func hasTarSuffix(fname string) bool {
	for _, suf := range []string{".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz", ".tar.zst", ".tar.lz"} {
		if strings.HasSuffix(fname, suf) {
			return true
		}
	}
	return false
}

func extractZip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		path := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("zip entry escapes destination: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0o755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// runDriver runs an embedded driver script with values passed only through
// the process environment (never interpolated as shell text), matching the
// injection-safety approach of the original bash implementation. prefix, if
// given, wraps the invocation (e.g. []string{"fakeroot", "--"}).
func runDriver(script []byte, env []string, prefix ...string) error {
	tmp, err := os.CreateTemp("", "vur-driver-*.sh")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(script); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	args := append(append([]string{}, prefix...), "bash", tmp.Name())
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// numJobs is runtime.NumCPU(), used for MAKEFLAGS below -- makepkg.conf
// normally sets this for you, but our minimal env starts blank, which
// otherwise silently defaults most Makefiles to a single-threaded build.
var numJobs = strconv.Itoa(runtime.NumCPU())

func baseEnv(extra ...string) []string {
	env := []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"TERM=" + firstNonEmpty(os.Getenv("TERM"), "dumb"),
		"LANG=" + firstNonEmpty(os.Getenv("LANG"), "C.UTF-8"),
		"MAKEFLAGS=-j" + numJobs,
		"CARGO_BUILD_JOBS=" + numJobs,
	}
	return append(env, extra...)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// RunBuild runs prepare()/build() as the normal user.
func RunBuild(pkgbase string, pb *pkgbuild.PKGBUILD, d Dirs, arch string) error {
	if _, err := os.Stat(d.Src); err != nil {
		return fmt.Errorf("run_build: no srcdir for %s (run FetchSources first)", pkgbase)
	}
	ui.Info("building %s", pkgbase)
	env := baseEnv(
		"startdir="+d.Git, "srcdir="+d.Src, "pkgbase="+pb.Base,
		"pkgver="+pb.Ver, "pkgrel="+pb.Rel, "CARCH="+arch,
	)
	if err := runDriver(driverBuild, env); err != nil {
		return fmt.Errorf("build() failed for %s: %w", pkgbase, err)
	}
	return nil
}

// RunPackage runs package()/package_<name>() under fakeroot, once per pkgname.
func RunPackage(pkgbase string, pb *pkgbuild.PKGBUILD, d Dirs, arch string) error {
	sysutil.RequireBin("fakeroot", "fakeroot")
	for _, name := range pb.Names {
		pkgdir := filepath.Join(d.Pkg, name)
		os.RemoveAll(pkgdir)
		if err := os.MkdirAll(pkgdir, 0o755); err != nil {
			return err
		}
		ui.Info("packaging %s", name)
		env := baseEnv(
			"startdir="+d.Git, "srcdir="+d.Src, "pkgdir="+pkgdir,
			"pkgbase="+pb.Base, "pkgname="+name, "pkgver="+pb.Ver,
			"pkgrel="+pb.Rel, "CARCH="+arch,
		)
		err := runDriver(driverPackage, env, "fakeroot", "--")
		if err != nil {
			return fmt.Errorf("package() failed for %s: %w", name, err)
		}
	}
	return nil
}

// BuiltVCSCommit returns the HEAD of the first git+ source actually built
// (if any), for --devel tracking. Empty for ordinary (non-VCS) packages.
func BuiltVCSCommit(pb *pkgbuild.PKGBUILD, d Dirs) string {
	for _, s := range pb.Source {
		fname, url := pkgbuild.SplitSourceEntry(s)
		if !strings.HasPrefix(url, "git+") {
			continue
		}
		dir := filepath.Join(d.Src, fname)
		if commit := gitutil.HeadCommit(dir); commit != "" {
			return commit
		}
	}
	return ""
}
