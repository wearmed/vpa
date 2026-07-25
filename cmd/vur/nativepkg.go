package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"vur/internal/foreignpkg"
	"vur/internal/sysutil"
	"vur/internal/ui"
	"vur/internal/xbpsutil"
)

// isXbpsFileArg reports whether an install argument names a raw .xbps
// binary package (already Void-native, no format translation needed).
func isXbpsFileArg(arg string) bool {
	return strings.HasSuffix(strings.ToLower(arg), ".xbps")
}

// installXbpsFile installs a standalone .xbps file (local path or URL)
// directly: xbps-install only takes package names resolved against a repo,
// not a bare file path, so this indexes the file into vur's own local repo
// (the same mechanism used for its own AUR builds) and installs it by the
// name recovered from the file itself.
func (a *App) installXbpsFile(pathOrURL string) error {
	ui.Info("fetching %s", pathOrURL)
	file, cleanup, err := foreignpkg.Fetch(pathOrURL)
	defer cleanup()
	if err != nil {
		return err
	}

	pkgname, err := xbpsNameFromFilename(filepath.Base(pathOrURL))
	if err != nil {
		return err
	}

	if !ui.Confirm("Install '%s' (a .xbps package) from %s?", pkgname, pathOrURL) {
		return fmt.Errorf("aborted by user for %s", pkgname)
	}

	dest := filepath.Join(a.Cfg.RepoDir, filepath.Base(pathOrURL))
	if err := copyToRepo(file, dest); err != nil {
		return err
	}
	if err := xbpsutil.Rindex(a.Cfg.RepoDir); err != nil {
		return err
	}
	if err := xbpsutil.Install(a.Cfg.RepoDir, a.Cfg.CacheDir, pkgname); err != nil {
		return err
	}

	ui.Ok("installed: %s", pkgname)
	return nil
}

// xbpsNameFromFilename recovers the bare package name from a real .xbps
// filename (name-pkgver_pkgrel.arch.xbps, xbps-create's own naming
// convention -- verified as exactly what our own Create() produces).
// Delegates the name/version split to xbps-uhelper getpkgname rather than
// guessing with a regex, since pkgnames can themselves contain dashes and
// digits that make the split ambiguous without xbps's own parsing rules.
func xbpsNameFromFilename(name string) (string, error) {
	name = strings.TrimSuffix(name, ".xbps")
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return "", fmt.Errorf("doesn't look like a valid .xbps filename: %s", name)
	}
	pkgver := name[:i] // strips the trailing ".<arch>" component
	out, err := sysutil.Output("xbps-uhelper", "getpkgname", pkgver)
	if err != nil {
		return "", fmt.Errorf("couldn't determine the package name for %s: %w", name, err)
	}
	pkgname := strings.TrimSpace(out)
	if pkgname == "" {
		return "", fmt.Errorf("couldn't determine the package name for %s", name)
	}
	return pkgname, nil
}

func copyToRepo(src, dst string) error {
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

// installVoidRepo installs package names that are already resolvable from
// Void's own configured repos (or already installed) -- no PKGBUILD, no
// AUR involved at all, so none of vur's review/confirm ceremony applies;
// xbps-install's own native prompt (unless --noconfirm) is the only
// confirmation needed, same trust level as running it directly yourself.
func (a *App) installVoidRepo(names []string) error {
	ui.Info("already in Void's repos, installing directly: %s", strings.Join(names, " "))
	return xbpsutil.Install(a.Cfg.RepoDir, a.Cfg.CacheDir, names...)
}
