package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"vpa/internal/ui"
	"vpa/internal/xbpsutil"
)

// cmdDowngrade rolls a package back to an earlier version.
//
// A repository index only ever describes the current version of a package,
// so there is nothing to "install an older version" from -- except the
// files xbps already downloaded, which stay in /var/cache/xbps after
// installation. That cache is the rollback story: if you have run the
// version before, you can go back to it, offline and immediately.
func (a *App) cmdDowngrade(args []string) error {
	name := args[0]

	// A version can be named outright to skip the picker, which is what
	// makes this usable from a script.
	var want string
	if len(args) > 1 {
		want = args[1]
	}

	versions, err := availableVersions(name)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		ui.Warn("no older versions of '%s' available to go back to", name)
		ui.Info("vpa looks in %s for versions you've installed before, and in your repositories for older builds they still publish.", xbpsutil.CacheDir)
		ui.Info("That cache is emptied by 'sudo xbps-remove -O', not by 'vpa cleanup' -- anything it removes is gone as a rollback target.")
		return fmt.Errorf("nothing to downgrade '%s' to", name)
	}

	current := xbpsutil.InstalledVersion(name)
	if want != "" {
		return a.installVersion(name, want, current, versions)
	}

	older := make([]xbpsutil.CachedPackage, 0, len(versions))
	for _, v := range versions {
		if current == "" || xbpsutil.CompareVersions(v.Version, current) < 0 {
			older = append(older, v)
		}
	}
	if len(older) == 0 {
		ui.Ok("%s %s is the oldest version you have cached -- nothing older to go back to", name, current)
		return nil
	}

	if current != "" {
		ui.Info("%s is currently at %s", name, current)
	}
	fmt.Println()
	for i, v := range older {
		where := "cached"
		if isRemote(v) {
			where = "download"
		}
		fmt.Printf("  %d) %s %s  (%s)\n", i+1, ui.Bold(name), v.Version, where)
	}
	fmt.Println()

	fmt.Printf("Which version? [1-%d, or blank to cancel] ", len(older))
	line := strings.TrimSpace(readLine())
	if line == "" {
		return fmt.Errorf("cancelled -- '%s' was not changed", name)
	}
	idx, err := strconv.Atoi(line)
	if err != nil || idx < 1 || idx > len(older) {
		return fmt.Errorf("'%s' isn't one of the listed numbers", line)
	}
	return a.doDowngrade(name, older[idx-1], current)
}

// availableVersions collects everything a package could be rolled back to:
// the files xbps already downloaded, plus older builds still published by a
// repository. The cache wins on duplicates, since a local file needs no
// download and no signature round trip.
func availableVersions(name string) ([]xbpsutil.CachedPackage, error) {
	cached, err := xbpsutil.CachedVersions(name)
	if err != nil {
		return nil, err
	}
	have := make(map[string]bool, len(cached))
	for _, v := range cached {
		have[v.Version] = true
	}

	for _, v := range xbpsutil.RemoteVersions(name, fetchText) {
		if !have[v.Version] {
			have[v.Version] = true
			cached = append(cached, v)
		}
	}
	sort.Slice(cached, func(i, j int) bool {
		return xbpsutil.CompareVersions(cached[i].Version, cached[j].Version) > 0
	})
	return cached, nil
}

// fetchText retrieves a URL as text, for reading a repository's directory
// listing. Short timeout: a repository that doesn't answer quickly just
// doesn't contribute versions.
func fetchText(url string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(body), err
}

// downloadPackage fetches a package and its detached signature into a
// temporary directory, returning the local path and a cleanup function.
//
// The signature is fetched alongside deliberately: without it the staged
// repository would be unsigned, which is a quieter outcome than it sounds
// -- xbps does not enforce signatures on local repositories, so a missing
// .sig2 would downgrade the check to nothing rather than fail loudly.
func downloadPackage(p xbpsutil.CachedPackage) (string, func(), error) {
	dir, err := os.MkdirTemp("", "vpa-dl-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(dir) }

	local := filepath.Join(dir, filepath.Base(p.Path))
	if err := downloadTo(p.Path, local); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("couldn't download %s: %w", p.Path, err)
	}
	if err := downloadTo(p.Path+".sig2", local+".sig2"); err != nil {
		ui.Warn("no signature published for this package -- installing it unverified")
	}
	return local, cleanup, nil
}

func downloadTo(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return f.Close()
}

// isRemote reports whether a candidate has to be downloaded first.
func isRemote(p xbpsutil.CachedPackage) bool {
	return strings.HasPrefix(p.Path, "http://") || strings.HasPrefix(p.Path, "https://")
}

// installVersion handles an explicitly named version.
func (a *App) installVersion(name, want, current string, versions []xbpsutil.CachedPackage) error {
	for _, v := range versions {
		if v.Version == want {
			return a.doDowngrade(name, v, current)
		}
	}
	ui.Warn("version '%s' of %s isn't available", want, name)
	ui.Info("what you can go back to:")
	for _, v := range versions {
		fmt.Printf("  %s\n", v.Version)
	}
	return fmt.Errorf("'%s' is not an available version of '%s'", want, name)
}

func (a *App) doDowngrade(name string, target xbpsutil.CachedPackage, current string) error {
	if current != "" && xbpsutil.CompareVersions(target.Version, current) > 0 {
		ui.Warn("%s is newer than the installed %s -- that's an upgrade, not a downgrade", target.Version, current)
	}

	// Downgrading is not routine: the older build may have the bug you
	// updated away from, and nothing stops the next system upgrade putting
	// the new version straight back.
	ui.Info("about to replace %s %s with %s", name, current, target.Version)
	if isRemote(target) {
		ui.Info("downloading it from %s", target.Path)
	}
	if !ui.Confirm("Downgrade %s to %s?", name, target.Version) {
		return fmt.Errorf("cancelled -- '%s' was not changed", name)
	}

	path := target.Path
	if isRemote(target) {
		local, cleanup, err := downloadPackage(target)
		if err != nil {
			return err
		}
		defer cleanup()
		path = local
	}

	pkgver := fmt.Sprintf("%s-%s", name, target.Version)
	if err := xbpsutil.InstallPkgFile(path, pkgver); err != nil {
		return fmt.Errorf("downgrade failed: %w", err)
	}

	ui.Ok("%s is now at %s", name, target.Version)
	ui.Info("'vpa update' will upgrade it again -- run 'vpa hold %s' to stop that.", name)
	return nil
}
