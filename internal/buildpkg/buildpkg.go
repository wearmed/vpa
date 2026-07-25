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
	"strings"
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

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// FetchSources populates a fresh srcdir from pb.Source.
func FetchSources(pkgbase string, pb *pkgbuild.PKGBUILD, d Dirs) error {
	os.RemoveAll(d.Src)
	if err := os.MkdirAll(d.Src, 0o755); err != nil {
		return err
	}

	for i, entry := range pb.Source {
		if entry == "" {
			continue
		}
		fname, url := pkgbuild.SplitSourceEntry(entry)

		switch {
		case strings.HasPrefix(url, "git+"):
			if err := fetchGitSource(url, fname, d.Src); err != nil {
				return err
			}
		case strings.Contains(url, "://"):
			ui.Info("downloading %s", fname)
			if err := download(url, filepath.Join(d.Src, fname)); err != nil {
				return fmt.Errorf("failed to download %s: %w", fname, err)
			}
			if err := verifyChecksum(pb, i, fname, d.Src); err != nil {
				return err
			}
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
	}
	return nil
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

func download(url, dest string) error {
	sysutil.RequireBin("curl", "curl")
	return sysutil.RunQuiet("curl", "-fL", "--retry", "3", "--connect-timeout", "15", "-o", dest, url)
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

func baseEnv(extra ...string) []string {
	env := []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"TERM=" + firstNonEmpty(os.Getenv("TERM"), "dumb"),
		"LANG=" + firstNonEmpty(os.Getenv("LANG"), "C.UTF-8"),
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
