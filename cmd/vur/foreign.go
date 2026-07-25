package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"vur/internal/foreignpkg"
	"vur/internal/manifest"
	"vur/internal/sysutil"
	"vur/internal/ui"
	"vur/internal/xbpsutil"
)

// isForeignPkgArg reports whether an install argument names a foreign
// binary package (Arch/Debian/RPM) rather than an AUR package name.
func isForeignPkgArg(arg string) bool {
	return foreignpkg.Detect(arg) != foreignpkg.Unknown
}

// installForeign imports a prebuilt Arch/.deb/.rpm package: extracts its
// payload, recovers whatever metadata is available, and repackages it as a
// real .xbps via xbps-create -- same destination shape as an AUR build,
// just without any build step. This can't fix ABI mismatches between
// distros (different glibc versions, library layouts, etc.), only
// packaging; whether the binary actually runs on Void is not guaranteed.
func (a *App) installForeign(pathOrURL string) error {
	format := foreignpkg.Detect(pathOrURL)
	sysutil.RequireBin("bsdtar", "bsdtar")

	ui.Info("fetching %s", pathOrURL)
	file, cleanup, err := foreignpkg.Fetch(pathOrURL)
	defer cleanup()
	if err != nil {
		return err
	}

	pkgdir := filepath.Join(a.Cfg.BuildDir, "foreign", sanitizeName(filepath.Base(pathOrURL)), "pkg")
	ui.Info("extracting %s", filepath.Base(pathOrURL))
	meta, err := foreignpkg.Extract(format, file, filepath.Base(pathOrURL), pkgdir)
	if err != nil {
		return err
	}
	if meta.Name == "" {
		return fmt.Errorf("couldn't determine a package name for %s -- it may not really be a %s package", pathOrURL, formatName(format))
	}

	ui.Info("imported: %s %s-%s (%s)", meta.Name, meta.Version, meta.Release, formatName(format))
	if meta.Desc != "" {
		ui.Info("  %s", meta.Desc)
	}
	if len(meta.Depends) > 0 {
		ui.Warn("this %s package declares dependencies vur can't map to Void package names -- you may need to install these yourself if things don't work: %s", formatName(format), strings.Join(meta.Depends, ", "))
	}
	ui.Warn("this is a foreign binary repackaged as-is -- it was built for %s, not Void, so it may not run correctly even though both are glibc-based", formatName(format))

	if !ui.Confirm("Repackage '%s' as a Void .xbps and install it?", meta.Name) {
		return fmt.Errorf("aborted by user for %s", meta.Name)
	}

	xver, xrel := sanitizeVersRel(meta.Version, meta.Release)
	if err := xbpsutil.Create(meta.Name, xver, xrel, pkgdir, "", meta.Desc, meta.URL, meta.License, a.Cfg.RepoDir); err != nil {
		return err
	}
	if err := xbpsutil.Rindex(a.Cfg.RepoDir); err != nil {
		return err
	}
	if err := xbpsutil.Install(a.Cfg.RepoDir, a.Cfg.CacheDir, meta.Name); err != nil {
		return err
	}

	m, err := manifest.Load(a.Cfg.ManifestFile)
	if err != nil {
		return err
	}
	m.Set(meta.Name, xver+"-"+xrel, "")
	if err := m.Save(); err != nil {
		return err
	}

	ui.Ok("installed: %s", meta.Name)
	return nil
}

func formatName(f foreignpkg.Format) string {
	switch f {
	case foreignpkg.Arch:
		return "Arch"
	case foreignpkg.Debian:
		return "Debian"
	case foreignpkg.RPM:
		return "RPM"
	default:
		return "unknown"
	}
}

// sanitizeName turns an arbitrary filename into something safe to use as a
// directory component (strips the package-format extensions off too).
func sanitizeName(name string) string {
	for _, ext := range []string{".pkg.tar.zst", ".pkg.tar.xz", ".pkg.tar.gz", ".deb", ".rpm"} {
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

var (
	numericRe    = regexp.MustCompile(`^[0-9]+$`)
	invalidVerRe = regexp.MustCompile(`[^A-Za-z0-9._]+`)
)

// sanitizeVersRel adapts a foreign package's version/release into xbps-create's
// strict pkgver format (name-version_revision, where revision must be a
// plain integer) -- RPM/deb releases like "1.fc44" or "1ubuntu2" fail that
// check outright, so anything non-numeric gets folded into the version
// string instead and the revision becomes a synthetic "1".
func sanitizeVersRel(version, release string) (string, string) {
	if numericRe.MatchString(release) {
		return invalidVerRe.ReplaceAllString(version, "."), release
	}
	combined := version
	if release != "" {
		combined += "." + release
	}
	return invalidVerRe.ReplaceAllString(combined, "."), "1"
}
