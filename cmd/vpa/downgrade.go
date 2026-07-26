package main

import (
	"fmt"
	"strconv"
	"strings"

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

	versions, err := xbpsutil.CachedVersions(name)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		ui.Warn("no cached versions of '%s' to go back to", name)
		ui.Info("vpa can only downgrade to a version already in %s, which means one you've installed before.", xbpsutil.CacheDir)
		ui.Info("'vpa cleanup' clears that cache, so anything removed by it is gone as a rollback target.")
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
		fmt.Printf("  %d) %s %s\n", i+1, ui.Bold(name), v.Version)
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

// installVersion handles an explicitly named version.
func (a *App) installVersion(name, want, current string, versions []xbpsutil.CachedPackage) error {
	for _, v := range versions {
		if v.Version == want {
			return a.doDowngrade(name, v, current)
		}
	}
	ui.Warn("version '%s' of %s isn't cached", want, name)
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
	if !ui.Confirm("Downgrade %s to %s?", name, target.Version) {
		return fmt.Errorf("cancelled -- '%s' was not changed", name)
	}

	if err := xbpsutil.InstallFiles(target.Path); err != nil {
		return fmt.Errorf("downgrade failed: %w", err)
	}

	ui.Ok("%s is now at %s", name, target.Version)
	ui.Info("'vpa update' will upgrade it again -- run 'vpa hold %s' to stop that.", name)
	return nil
}
