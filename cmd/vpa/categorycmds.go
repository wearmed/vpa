package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"vpa/internal/aurapi"
	"vpa/internal/categories"
	"vpa/internal/flatpak"
	"vpa/internal/sysutil"
	"vpa/internal/ui"
	"vpa/internal/xbpsutil"
)

// isCategoryWord reports whether the user asked for a category search
// rather than an ordinary one. "cat" is the short form the user types.
func isCategoryWord(s string) bool {
	switch strings.ToLower(s) {
	case "cat", "category", "categories":
		return true
	}
	return false
}

// loadCategories builds the category map, refreshing the copy cached from
// the server if it's old. A failed refresh is not worth interrupting a
// search over -- there's always the built-in list underneath it.
func (a *App) loadCategories() *categories.Set {
	if categories.NeedsRefresh(a.Cfg.CategoryCache) {
		// Best effort: not reaching the server just means searching the
		// list built into this binary, which is not worth a warning.
		_ = categories.Refresh(a.Cfg.CategoryURL, a.Cfg.CategoryCache)
	}
	return categories.Load(a.Cfg.CategoryCache, a.Cfg.UserCategories)
}

// cmdSearchCategory lists what's actually installable in a category.
//
// The category names a set of candidate packages; this checks all three
// sources for them and shows only what really exists, so a list can name
// something Void dropped or the AUR renamed without the user ever seeing a
// dead entry.
func (a *App) cmdSearchCategory(args []string) error {
	set := a.loadCategories()
	if len(args) == 0 {
		printCategoryList(set)
		return nil
	}

	name := args[0]
	cat, ok := set.Lookup(name)
	if !ok {
		ui.Warn("no category called '%s'", name)
		printCategoryList(set)
		return nil
	}

	var (
		voidPkgs []xbpsutil.RepoPackage
		aurPkgs  []aurapi.Package
		flatApps []flatpak.App
		wg       sync.WaitGroup
	)
	voidNames, flatIDs := splitCandidates(cat.Packages)
	haveFlatpak := flatpak.Available()

	wg.Add(2)
	go func() {
		defer wg.Done()
		var err error
		if voidPkgs, err = xbpsutil.RepoPackagesByName(voidNames); err != nil {
			ui.Warn("Void repository lookup failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		// Names already found in Void's repos still get looked up: the same
		// name existing in both places is worth seeing, same as a normal
		// search shows both.
		if pkgs, err := aurapi.Info(voidNames...); err == nil {
			aurPkgs = pkgs
		} else {
			ui.Warn("AUR lookup failed: %v", err)
		}
	}()
	if haveFlatpak {
		wg.Add(1)
		go func() {
			defer wg.Done()
			flatApps = flatpakCandidates(cat, flatIDs)
		}()
	}
	wg.Wait()

	title := cat.Name
	if len(cat.Aliases) > 0 {
		title += " (" + strings.Join(cat.Aliases, ", ") + ")"
	}
	fmt.Printf("%s\n\n", ui.Bold(title))

	sort.Slice(voidPkgs, func(i, j int) bool { return voidPkgs[i].Name < voidPkgs[j].Name })
	for _, p := range voidPkgs {
		printResult("void/"+p.Name, p.Version, p.Desc, p.Installed, "")
	}

	inVoid := make(map[string]bool, len(voidPkgs))
	for _, p := range voidPkgs {
		inVoid[p.Name] = true
	}
	sort.Slice(aurPkgs, func(i, j int) bool { return aurPkgs[i].Name < aurPkgs[j].Name })
	for _, p := range aurPkgs {
		printResult("aur/"+p.Name, p.Version, p.Description,
			xbpsutil.IsInstalled(p.Name), aurHealthNote(p, a.Cfg.StaleDays))
	}

	sort.Slice(flatApps, func(i, j int) bool { return flatApps[i].ID < flatApps[j].ID })
	for _, f := range flatApps {
		printResult("flatpak/"+f.ID, f.Version, f.Desc, flatpak.IsInstalled(f.ID), "")
	}

	total := len(voidPkgs) + len(aurPkgs) + len(flatApps)
	if total == 0 {
		ui.Info("nothing in '%s' is available on this system right now", cat.Name)
		return nil
	}
	ui.Info("%d in '%s' -- install any of them by name, e.g. vpa install %s",
		total, cat.Name, firstResultName(voidPkgs, aurPkgs, flatApps))
	return nil
}

// splitCandidates separates Flatpak application IDs from ordinary package
// names. An app ID is reverse-DNS, which no xbps or AUR package name is.
func splitCandidates(all []string) (pkgNames, flatpakIDs []string) {
	for _, n := range all {
		if flatpak.LooksLikeAppID(n) {
			flatpakIDs = append(flatpakIDs, n)
		} else {
			pkgNames = append(pkgNames, n)
		}
	}
	return pkgNames, flatpakIDs
}

// flatpakCandidates resolves the Flatpak side of a category. Flathub is the
// one source that actually records categories, via AppStream, so when
// appstreamcli is installed the list comes from there and stays current on
// its own; the IDs in the category file are the fallback for when it isn't.
func flatpakCandidates(cat *categories.Category, ids []string) []flatpak.App {
	seen := map[string]bool{}
	var apps []flatpak.App

	if cat.Freedesktop != "" {
		for _, app := range appstreamCategory(cat.Freedesktop) {
			if !seen[app.ID] {
				seen[app.ID] = true
				apps = append(apps, app)
			}
		}
	}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		// flatpak search matches on more than the ID, so the exact ID has
		// to be picked back out of the results.
		found, err := flatpak.Search(id)
		if err != nil {
			continue
		}
		for _, app := range found {
			if app.ID == id {
				seen[id] = true
				apps = append(apps, app)
				break
			}
		}
	}
	return apps
}

// appstreamCategory asks appstreamcli which applications are in a
// freedesktop category. Absent appstreamcli, or an empty catalog, this just
// returns nothing and the curated IDs carry the category on their own.
func appstreamCategory(name string) []flatpak.App {
	out, err := sysutil.Output("appstreamcli", "list-categories", name)
	if err != nil {
		return nil
	}
	var apps []flatpak.App
	var cur flatpak.App
	flush := func() {
		if cur.ID != "" {
			apps = append(apps, cur)
		}
		cur = flatpak.App{}
	}
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "Identifier":
			flush()
			// "org.mozilla.firefox [desktop-application]"
			cur.ID = strings.Fields(val)[0]
		case "Name":
			cur.Name = val
		case "Summary":
			cur.Desc = val
		}
	}
	flush()
	return apps
}

func printResult(name, version, desc string, installed bool, note string) {
	mark := ""
	if installed {
		mark = " [installed]"
	}
	// AppStream doesn't report a version, so Flatpak results often have
	// none -- don't leave a dangling space where it would have gone.
	if version != "" {
		version = " " + version
	}
	fmt.Printf("%s%s%s%s\n", ui.Bold(name), version, mark, note)
	if desc != "" {
		fmt.Printf("    %s\n", desc)
	}
}

func firstResultName(void []xbpsutil.RepoPackage, aur []aurapi.Package, flat []flatpak.App) string {
	switch {
	case len(void) > 0:
		return void[0].Name
	case len(aur) > 0:
		return aur[0].Name
	case len(flat) > 0:
		return flat[0].ID
	}
	return ""
}

func printCategoryList(set *categories.Set) {
	names := set.Names()
	fmt.Printf("%s\n\n", ui.Bold("Categories you can search"))
	const cols = 4
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	for i, n := range names {
		fmt.Printf("  %-*s", width, n)
		if i%cols == cols-1 {
			fmt.Println()
		}
	}
	if len(names)%cols != 0 {
		fmt.Println()
	}
	fmt.Printf("\n  vpa search cat browser\n\nAdd your own categories in ~/.config/vpa/categories.conf\n")
}
