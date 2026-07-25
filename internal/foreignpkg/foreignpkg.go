// Package foreignpkg imports a prebuilt Arch (.pkg.tar.zst/.xz/.gz), Debian
// (.deb), or RPM (.rpm) package: extracts its payload via bsdtar (libarchive
// reads all three natively), pulls out what metadata is available, and hands
// back a pkgdir ready for xbps-create -- same shape as an AUR build's pkgdir.
//
// This only repackages files; it can't fix ABI/library mismatches between
// distros, so the result may or may not actually run on Void.
package foreignpkg

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"vur/internal/sysutil"
)

type Format int

const (
	Unknown Format = iota
	Arch
	Debian
	RPM
)

type Meta struct {
	Name    string
	Version string
	Release string
	Desc    string
	URL     string
	License string
	Depends []string
}

// Detect returns the foreign package format implied by a filename, if any.
func Detect(nameOrURL string) Format {
	base := strings.ToLower(nameOrURL)
	switch {
	case strings.HasSuffix(base, ".deb"):
		return Debian
	case strings.HasSuffix(base, ".rpm"):
		return RPM
	case strings.HasSuffix(base, ".pkg.tar.zst"), strings.HasSuffix(base, ".pkg.tar.xz"), strings.HasSuffix(base, ".pkg.tar.gz"):
		return Arch
	default:
		return Unknown
	}
}

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// Fetch resolves pathOrURL to a local file, downloading it to a temp file
// first if it's an http(s) URL. The returned cleanup always removes any
// temp file it created (a no-op for an already-local path).
func Fetch(pathOrURL string) (path string, cleanup func(), err error) {
	if !strings.HasPrefix(pathOrURL, "http://") && !strings.HasPrefix(pathOrURL, "https://") {
		if _, err := os.Stat(pathOrURL); err != nil {
			return "", func() {}, fmt.Errorf("no such file: %s", pathOrURL)
		}
		return pathOrURL, func() {}, nil
	}

	tmp, err := os.CreateTemp("", "vur-foreign-*"+filepath.Ext(pathOrURL))
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { os.Remove(tmp.Name()) }

	resp, err := httpClient.Get(pathOrURL)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cleanup()
		return "", func() {}, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		cleanup()
		return "", func() {}, err
	}
	tmp.Close()
	return tmp.Name(), cleanup, nil
}

// Extract unpacks file (of the given Format) into pkgdir and returns
// whatever metadata it could recover. originalName is the package's real
// filename (e.g. from the source URL) -- file itself may be a randomly
// named temp download, which would break RPM's filename-convention
// metadata fallback if used instead.
func Extract(format Format, file, originalName, pkgdir string) (Meta, error) {
	sysutil.RequireBin("bsdtar", "bsdtar")
	// A stale pkgdir from a previous (failed, or different-version) import
	// of the same package would otherwise get overlaid rather than replaced,
	// silently shipping leftover files the new package doesn't actually have.
	os.RemoveAll(pkgdir)
	if err := os.MkdirAll(pkgdir, 0o755); err != nil {
		return Meta{}, err
	}

	switch format {
	case Arch:
		return extractArch(file, pkgdir)
	case Debian:
		return extractDebian(file, pkgdir)
	case RPM:
		return extractRPM(file, originalName, pkgdir)
	default:
		return Meta{}, fmt.Errorf("unrecognized foreign package format for %s", file)
	}
}

func extractArch(file, pkgdir string) (Meta, error) {
	if err := sysutil.RunQuiet("bsdtar", "-xf", file, "-C", pkgdir); err != nil {
		return Meta{}, fmt.Errorf("bsdtar failed to extract %s: %w", file, err)
	}

	var m Meta
	data, err := os.ReadFile(filepath.Join(pkgdir, ".PKGINFO"))
	if err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(data)))
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "#") {
				continue
			}
			key, val, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key, val = strings.TrimSpace(key), strings.TrimSpace(val)
			switch key {
			case "pkgname":
				m.Name = val
			case "pkgver":
				m.Version, m.Release = splitVerRel(val)
			case "pkgdesc":
				m.Desc = val
			case "url":
				m.URL = val
			case "license":
				m.License = val
			case "depend":
				m.Depends = append(m.Depends, val)
			}
		}
	}

	// Metadata files live alongside the payload at the archive root; strip
	// them so they don't end up shipped as part of the package itself.
	for _, f := range []string{".PKGINFO", ".BUILDINFO", ".MTREE", ".INSTALL"} {
		os.Remove(filepath.Join(pkgdir, f))
	}
	return m, nil
}

func extractDebian(file, pkgdir string) (Meta, error) {
	staging, err := os.MkdirTemp("", "vur-deb-*")
	if err != nil {
		return Meta{}, err
	}
	defer os.RemoveAll(staging)

	if err := sysutil.RunQuiet("bsdtar", "-xf", file, "-C", staging); err != nil {
		return Meta{}, fmt.Errorf("bsdtar failed to extract %s: %w", file, err)
	}

	dataTar, err := globOne(staging, "data.tar.*")
	if err != nil {
		return Meta{}, err
	}
	if err := sysutil.RunQuiet("bsdtar", "-xf", dataTar, "-C", pkgdir); err != nil {
		return Meta{}, fmt.Errorf("bsdtar failed to extract %s: %w", dataTar, err)
	}

	var m Meta
	if controlTar, err := globOne(staging, "control.tar.*"); err == nil {
		controlDir := filepath.Join(staging, "control")
		os.MkdirAll(controlDir, 0o755)
		if sysutil.RunQuiet("bsdtar", "-xf", controlTar, "-C", controlDir) == nil {
			if data, err := os.ReadFile(filepath.Join(controlDir, "control")); err == nil {
				m = parseDebControl(string(data))
			}
		}
	}
	return m, nil
}

func parseDebControl(data string) Meta {
	var m Meta
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue // continuation line, e.g. multi-line Description
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "Package":
			m.Name = val
		case "Version":
			m.Version, m.Release = splitVerRel(val)
		case "Description":
			m.Desc = val
		case "Homepage":
			m.URL = val
		case "Depends":
			for _, d := range strings.Split(val, ",") {
				m.Depends = append(m.Depends, strings.TrimSpace(d))
			}
		}
	}
	return m
}

var rpmFilenameRe = regexp.MustCompile(`^(.+)-([^-]+)-([^-]+)\.([^.]+)\.rpm$`)

func extractRPM(file, originalName, pkgdir string) (Meta, error) {
	if err := sysutil.RunQuiet("bsdtar", "-xf", file, "-C", pkgdir); err != nil {
		return Meta{}, fmt.Errorf("bsdtar failed to extract %s: %w", file, err)
	}

	// bsdtar hands back the cpio payload only, with no separate metadata
	// file -- RPM's real header would need a hand-rolled tag parser, which
	// is disproportionate effort here. RPM filenames reliably follow
	// name-version-release.arch.rpm, so fall back to that convention.
	var m Meta
	if match := rpmFilenameRe.FindStringSubmatch(filepath.Base(originalName)); match != nil {
		m.Name, m.Version, m.Release = match[1], match[2], match[3]
	}
	return m, nil
}

// splitVerRel splits a combined "version-release" string (the convention
// both Arch's .PKGINFO pkgver and Debian's control Version field use) into
// its parts. Falls back to release "1" if there's no dash to split on.
func splitVerRel(combined string) (version, release string) {
	i := strings.LastIndex(combined, "-")
	if i < 0 {
		return combined, "1"
	}
	return combined[:i], combined[i+1:]
}

func globOne(dir, pattern string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("no file matching %s in %s", pattern, dir)
	}
	return matches[0], nil
}
