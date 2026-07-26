package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"vpa/internal/aurapi"
	"vpa/internal/buildpkg"
	"vpa/internal/config"
	"vpa/internal/deps"
	"vpa/internal/flatpak"
	"vpa/internal/gitutil"
	"vpa/internal/manifest"
	"vpa/internal/pkgbuild"
	"vpa/internal/sysutil"
	"vpa/internal/ui"
	"vpa/internal/xbpsutil"
)

type App struct {
	Cfg *config.Config
	// ExplicitYes records that --noconfirm/-y was actually passed on the
	// command line, as opposed to yes simply being the configured default.
	// Reviewing an unseen AUR build script is a security decision rather
	// than a routine confirmation, so it keeps asking under the default
	// and only auto-approves when you've explicitly said to.
	ExplicitYes bool
}

func (a *App) gitDir(pkgbase string) string {
	return buildpkg.NewDirs(a.Cfg.BuildDir, pkgbase).Git
}

// cmdSearch searches Void's own repositories and the AUR together, since
// from a user's point of view "what can I install called X" spans both.
// Void results come first: a native package is nearly always the better
// choice when one exists.
func (a *App) cmdSearch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa search <term>")
	}
	// `vpa search cat browser` browses a category instead of matching text.
	if isCategoryWord(args[0]) {
		return a.cmdSearchCategory(args[1:])
	}
	term := args[0]

	// Both lookups are independent; run them together so the AUR round trip
	// doesn't serialize behind the local one.
	var (
		voidPkgs []xbpsutil.RepoPackage
		aurPkgs  []aurapi.Package
		flatApps []flatpak.App
		voidErr  error
		aurErr   error
		wg       sync.WaitGroup
	)
	haveFlatpak := flatpak.Available()
	wg.Add(2)
	go func() { defer wg.Done(); voidPkgs, voidErr = xbpsutil.SearchRepos(term) }()
	go func() { defer wg.Done(); aurPkgs, aurErr = aurapi.Search(term) }()
	if haveFlatpak {
		wg.Add(1)
		go func() { defer wg.Done(); flatApps, _ = flatpak.Search(term) }()
	}
	wg.Wait()

	if voidErr != nil {
		ui.Warn("Void repository search failed: %v", voidErr)
	}
	if aurErr != nil {
		ui.Warn("AUR search failed: %v", aurErr)
	}

	sort.Slice(voidPkgs, func(i, j int) bool { return voidPkgs[i].Name < voidPkgs[j].Name })
	for _, p := range voidPkgs {
		mark := ""
		if p.Installed {
			mark = " [installed]"
		}
		fmt.Printf("%s %s%s\n", ui.Bold("void/"+p.Name), p.Version, mark)
		if p.Desc != "" {
			fmt.Printf("    %s\n", p.Desc)
		}
	}

	sort.Slice(aurPkgs, func(i, j int) bool { return aurPkgs[i].Name < aurPkgs[j].Name })
	for _, p := range aurPkgs {
		mark := ""
		if xbpsutil.IsInstalled(p.Name) {
			mark = " [installed]"
		}
		fmt.Printf("%s %s%s%s\n", ui.Bold("aur/"+p.Name), p.Version, mark, aurHealthNote(p, a.Cfg.StaleDays))
		if p.Description != "" {
			fmt.Printf("    %s\n", p.Description)
		}
	}

	for _, a := range flatApps {
		mark := ""
		if flatpak.IsInstalled(a.ID) {
			mark = " [installed]"
		}
		fmt.Printf("%s %s%s\n", ui.Bold("flatpak/"+a.ID), a.Version, mark)
		if a.Desc != "" {
			fmt.Printf("    %s\n", a.Desc)
		}
	}

	if len(voidPkgs) == 0 && len(aurPkgs) == 0 && len(flatApps) == 0 {
		where := "Void's repos or the AUR"
		if haveFlatpak {
			where = "Void's repos, the AUR or Flathub"
		}
		ui.Info("no results for '%s' in %s", term, where)
		return nil
	}
	if len(flatApps) > 0 {
		ui.Info("install a Flatpak with its full ID, e.g. vpa install %s", flatApps[0].ID)
	}
	return nil
}

// cmdInfo shows details for a package from wherever it exists -- Void's
// repos, the AUR, or both (a name can legitimately exist in each, and
// seeing both is the point).
func (a *App) cmdInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa info <pkg>")
	}
	name := args[0]

	// A reverse-DNS app ID only ever means a Flatpak.
	if flatpak.LooksLikeAppID(name) && flatpak.Available() {
		fmt.Printf("%s\n", ui.Bold("flatpak/"+name))
		return flatpak.Info(name)
	}

	inVoid := xbpsutil.IsInVoidRepos(name)
	if inVoid {
		fmt.Printf("%s\n", ui.Bold("void/"+name))
		if err := xbpsutil.ShowRepo(name); err != nil {
			ui.Warn("couldn't read Void package info: %v", err)
		}
	}

	pkgs, err := aurapi.Info(name)
	if err != nil {
		if inVoid {
			ui.Warn("AUR lookup failed: %v", err)
			return nil
		}
		return fmt.Errorf("AUR info request failed: %w", err)
	}
	p, ok := aurapi.ByName(pkgs, name)
	if !ok {
		if inVoid {
			return nil // found in Void's repos, just not on the AUR
		}
		return fmt.Errorf("package '%s' not found in Void's repos or the AUR", name)
	}

	if inVoid {
		fmt.Println()
	}
	fmt.Printf("%s\n", ui.Bold("aur/"+name))
	lastMod := time.Unix(p.LastModified, 0).UTC().Format("2006-01-02T15:04:05Z")
	fmt.Printf("Name           : %s\n", p.Name)
	fmt.Printf("PackageBase    : %s\n", p.PackageBase)
	fmt.Printf("Version        : %s\n", p.Version)
	fmt.Printf("Description    : %s\n", orDash(p.Description))
	fmt.Printf("URL            : %s\n", orDash(p.URL))
	fmt.Printf("License        : %s\n", strings.Join(p.License, ", "))
	fmt.Printf("Depends        : %s\n", strings.Join(p.Depends, ", "))
	fmt.Printf("MakeDepends    : %s\n", strings.Join(p.MakeDepends, ", "))
	fmt.Printf("OptDepends     : %s\n", strings.Join(p.OptDepends, ", "))
	fmt.Printf("Maintainer     : %s\n", orDash(p.Maintainer))
	fmt.Printf("Votes          : %d\n", p.NumVotes)
	fmt.Printf("Popularity     : %v\n", p.Popularity)
	fmt.Printf("Last Modified  : %s\n", lastMod)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (a *App) cmdInstall(pkgs []string) error {
	if len(pkgs) == 0 {
		return fmt.Errorf("usage: vpa install <pkg> [pkg...]")
	}

	var xbpsFileArgs, foreignArgs, voidArgs, aurArgs, flatArgs []string
	for _, p := range pkgs {
		switch {
		case isXbpsFileArg(p):
			xbpsFileArgs = append(xbpsFileArgs, p)
		case isForeignPkgArg(p):
			foreignArgs = append(foreignArgs, p)
		case a.Cfg.PreferFlatpak || flatpak.LooksLikeAppID(p):
			// A reverse-DNS app ID is unambiguous (nothing in Void's repos
			// or the AUR is named that way), so it needs no flag. Otherwise
			// Flatpak is only used when --flatpak asks for it: silently
			// installing a sandboxed bundle when a native package exists
			// would be a surprising thing to do behind someone's back.
			flatArgs = append(flatArgs, p)
		case xbpsutil.IsInVoidRepos(p):
			// A real Void package -- no PKGBUILD, no AUR involved, so go
			// straight to xbps-install. Note this deliberately checks Void's
			// own repos only, not vpa's local build repo: anything vpa built
			// earlier also lives there, and counting that would make this
			// reinstall the stale cached build instead of checking the AUR.
			voidArgs = append(voidArgs, p)
		default:
			aurArgs = append(aurArgs, p)
		}
	}
	if len(flatArgs) > 0 {
		if !flatpak.Available() {
			return fmt.Errorf("flatpak isn't installed -- run 'vpa install flatpak' first")
		}
		ids, err := resolveFlatpakNames(flatArgs)
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			ui.Info("installing from Flathub: %s", strings.Join(ids, " "))
			if err := flatpak.Install(ui.NoConfirm, ids...); err != nil {
				return fmt.Errorf("flatpak install failed: %w", err)
			}
		}
	}
	for _, f := range xbpsFileArgs {
		if err := a.installXbpsFile(f); err != nil {
			return err
		}
	}
	for _, f := range foreignArgs {
		if err := a.installForeign(f); err != nil {
			return err
		}
	}
	if len(voidArgs) > 0 {
		// xbps prints a red "ERROR: Package X already installed." and still
		// exits 0 for these, which reads like a failure when nothing is
		// wrong. Filter them out and say so plainly instead.
		var todo []string
		for _, p := range voidArgs {
			if xbpsutil.IsInstalled(p) {
				ui.Ok("%s is already installed", p)
				continue
			}
			todo = append(todo, p)
		}
		if len(todo) > 0 {
			if err := a.installVoidRepo(todo); err != nil {
				return err
			}
		}
	}
	if len(aurArgs) == 0 {
		return nil
	}

	sysutil.RequireBin("git", "git")
	sysutil.RequireBin("fakeroot", "fakeroot")
	sysutil.RequireBin("xbps-create", "xbps")
	// PKGBUILD build()/package() bodies routinely shell out to tar with
	// zstd/xz/bzip2 compression -- makepkg's environment assumes these
	// exist (they're part of base-devel on Arch), and without them a build
	// fails deep inside package() with a bare "Cannot exec" from tar.
	sysutil.RequireBin("bsdtar", "bsdtar")
	sysutil.RequireBin("zstd", "zstd")
	sysutil.RequireBin("xz", "xz")
	sysutil.RequireBin("bzip2", "bzip2")

	infos, err := aurapi.Info(aurArgs...)
	if err != nil {
		return fmt.Errorf("AUR lookup failed: %w", err)
	}

	var bases []string
	var badBases []string
	seenBase := make(map[string]bool)
	addBase := func(base string) {
		// A pkgbase becomes a directory under the build cache, so reject
		// anything that isn't usable as a single path component rather
		// than letting it escape.
		if !aurapi.ValidPackageName(base) {
			badBases = append(badBases, base)
			return
		}
		if !seenBase[base] {
			seenBase[base] = true
			bases = append(bases, base)
		}
	}
	tracked, err := manifest.Load(a.Cfg.ManifestFile)
	if err != nil {
		return err
	}
	for _, name := range aurArgs {
		if p, ok := aurapi.ByName(infos, name); ok {
			// Rebuilding something already installed at the version the AUR
			// currently offers can cost minutes for a large package and
			// achieves nothing. `vpa update` is what refreshes these.
			if e, known := tracked.Get(name); known && e.Version == p.Version && xbpsutil.IsInstalled(name) {
				ui.Ok("%s %s is already installed and up to date", name, e.Version)
				continue
			}
			if err := a.confirmAURHealth(p); err != nil {
				return err
			}
			addBase(p.PackageBase)
			continue
		}
		picked, err := interactiveSelectPkg(name)
		if err != nil {
			return fmt.Errorf("'%s' not found in AUR", name)
		}
		if len(picked) == 0 {
			return fmt.Errorf("no package selected for '%s'", name)
		}
		for _, pname := range picked {
			base, err := aurapi.PackageBase(pname)
			if err != nil || base == "" {
				return fmt.Errorf("'%s' not found in AUR", pname)
			}
			addBase(base)
		}
	}

	if len(badBases) > 0 {
		return fmt.Errorf("refusing package name(s) with unexpected characters: %s", strings.Join(badBases, ", "))
	}
	if len(bases) == 0 {
		return nil // everything requested was already installed and current
	}

	// Clone every top-level requested base concurrently before the (necessarily
	// sequential, since it prompts for confirmation) resolution walk -- network
	// latency for N packages becomes ~1 round trip instead of N.
	cloneErrs := make([]error, len(bases))
	{
		sem := make(chan struct{}, a.Cfg.Parallel)
		var wg sync.WaitGroup
		for i, base := range bases {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, base string) {
				defer wg.Done()
				defer func() { <-sem }()
				cloneErrs[i] = gitutil.CloneAUR(base, a.gitDir(base))
			}(i, base)
		}
		wg.Wait()
	}

	resolver := deps.NewResolver(a.Cfg.UserDepmap, a.Cfg.RepoDir)
	for i, base := range bases {
		if cloneErrs[i] != nil {
			return cloneErrs[i]
		}
		if err := resolver.Resolve(base, a.gitDir(base), a.Cfg.BuildDir, gitutil.CloneAUR, a.reviewAndLoad); err != nil {
			return err
		}
	}

	if len(resolver.Unresolved) > 0 {
		ui.Warn("unresolved dependencies:")
		for _, u := range resolver.Unresolved {
			fmt.Fprintf(os.Stderr, "  - %s\n", u)
		}
		if !ui.Confirm("Continue anyway?") {
			return fmt.Errorf("aborted due to unresolved dependencies")
		}
	}
	ui.Info("build order: %s", strings.Join(resolver.PlanOrder, " "))

	arch, err := xbpsutil.Arch()
	if err != nil {
		return err
	}

	// Build tier-by-tier: packages within a tier have no dependency relation
	// to each other (source fetching runs concurrently for the whole tier;
	// the actual build()/package() steps stay sequential within a tier to
	// avoid oversubscribing CPU cores with concurrent compiles), and each
	// tier is installed before the next tier's builds start, since a later
	// tier's build() may need an earlier tier's AUR-only package actually
	// present on disk as a real compile-time dependency.
	var builtNames []string
	for _, tier := range resolver.Tiers() {
		pkgs := make([]*pkgbuild.PKGBUILD, len(tier))
		dirsList := make([]buildpkg.Dirs, len(tier))
		fetchErrs := make([]error, len(tier))

		sem := make(chan struct{}, a.Cfg.Parallel)
		var wg sync.WaitGroup
		for i, pb := range tier {
			dirsList[i] = buildpkg.NewDirs(a.Cfg.BuildDir, pb)
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, pb string) {
				defer wg.Done()
				defer func() { <-sem }()
				pkg, err := pkgbuild.Load(dirsList[i].Git)
				if err != nil {
					fetchErrs[i] = err
					return
				}
				pkgs[i] = pkg
				fetchErrs[i] = buildpkg.FetchSources(pb, pkg, dirsList[i], a.Cfg.Parallel, a.Cfg.CacheDir)
			}(i, pb)
		}
		wg.Wait()

		var tierBuilt []string
		for i, pb := range tier {
			if fetchErrs[i] != nil {
				return fmt.Errorf("%s: %w", pb, fetchErrs[i])
			}
			pkg, dirs := pkgs[i], dirsList[i]
			if err := buildpkg.RunBuild(pb, pkg, dirs, arch); err != nil {
				return err
			}
			if err := buildpkg.RunPackage(pb, pkg, dirs, arch); err != nil {
				return err
			}
			depsStr := resolver.RuntimeDepsString(pkg)
			for _, name := range pkg.Names {
				pkgdir := filepath.Join(dirs.Pkg, name)
				if err := xbpsutil.Create(name, pkg.Ver, pkg.Rel, pkgdir, depsStr, pkg.Desc, pkg.URL, strings.Join(pkg.License, ", "), a.Cfg.RepoDir); err != nil {
					return err
				}
				tierBuilt = append(tierBuilt, name)
			}
		}

		if err := xbpsutil.Rindex(a.Cfg.RepoDir); err != nil {
			return err
		}
		if err := xbpsutil.Install(a.Cfg.RepoDir, a.Cfg.CacheDir, tierBuilt...); err != nil {
			return err
		}
		builtNames = append(builtNames, tierBuilt...)
	}

	m, err := manifest.Load(a.Cfg.ManifestFile)
	if err != nil {
		return err
	}
	for _, pb := range resolver.PlanOrder {
		dirs := buildpkg.NewDirs(a.Cfg.BuildDir, pb)
		pkg, err := pkgbuild.Load(dirs.Git)
		if err != nil {
			return err
		}
		commit := buildpkg.BuiltVCSCommit(pkg, dirs)
		for _, name := range pkg.Names {
			m.Set(name, pkg.FullVersion(), commit)
		}
		if a.Cfg.CleanAfter {
			os.RemoveAll(dirs.Src)
			os.RemoveAll(dirs.Pkg)
		}
	}
	if err := m.Save(); err != nil {
		return err
	}

	ui.Ok("installed: %s", strings.Join(builtNames, " "))
	return nil
}

func (a *App) cmdRemove(pkgs []string) error {
	if len(pkgs) == 0 {
		return fmt.Errorf("usage: vpa remove <pkg> [pkg...]")
	}

	// An installed Flatpak lives in Flatpak's database, not xbps's, so it
	// has to be removed with flatpak or xbps-remove would just say it isn't
	// installed.
	if flatpak.Available() {
		var flat, rest []string
		for _, p := range pkgs {
			// Route on "is this a Flatpak's name" rather than "is it
			// installed": otherwise removing an app ID that isn't installed
			// falls through to xbps, which reports the confusing
			// "Package org.gnome.Calculator is not currently installed".
			isAppID := flatpak.LooksLikeAppID(p) && !xbpsutil.IsInstalled(p)
			if (flatpak.IsInstalled(p) && !xbpsutil.IsInstalled(p)) || isAppID {
				flat = append(flat, p)
			} else {
				rest = append(rest, p)
			}
		}
		for _, p := range flat {
			if !flatpak.IsInstalled(p) {
				return fmt.Errorf("no Flatpak called %q is installed", p)
			}
		}
		if len(flat) > 0 {
			ui.Info("removing Flatpak: %s", strings.Join(flat, " "))
			if err := flatpak.Remove(ui.NoConfirm, flat...); err != nil {
				return fmt.Errorf("flatpak uninstall failed: %w", err)
			}
		}
		if len(rest) == 0 {
			return nil
		}
		pkgs = rest
	}
	if err := xbpsutil.Remove(pkgs...); err != nil {
		return err
	}
	if err := a.forgetRemoved(pkgs); err != nil {
		return err
	}
	ui.Ok("removed: %s", strings.Join(pkgs, " "))
	return nil
}

// forgetRemoved drops packages from vpa's manifest after they've been
// removed from the system, so `vpa list`/`vpa update` stop tracking them.
func (a *App) forgetRemoved(pkgs []string) error {
	m, err := manifest.Load(a.Cfg.ManifestFile)
	if err != nil {
		return err
	}
	for _, p := range pkgs {
		m.Remove(p)
	}
	return m.Save()
}

func (a *App) cmdUpdate() error {
	// vpa updates itself as part of a normal update run -- it's just another
	// thing on the system that gets out of date, and a stale vpa silently
	// missing fixes is exactly the failure mode worth avoiding. Non-fatal:
	// a self-update problem shouldn't block updating actual packages.
	if err := selfUpdateIfAvailable(); err != nil {
		ui.Warn("couldn't update vpa itself: %v", err)
	}

	if ui.Confirm("Run a full system upgrade first (sudo xbps-install -Su)?") {
		// -y here too: with --noconfirm, xbps-install's own nested confirmation
		// prompt would otherwise still block waiting for interactive input
		// (or silently abort on a non-tty stdin), defeating the point of
		// --noconfirm entirely.
		args := []string{"xbps-install", "-Su"}
		if ui.NoConfirm {
			args = append(args, "-y")
		}
		if err := sysutil.RunInteractive("sudo", args...); err != nil {
			ui.Warn("system upgrade failed or was declined by xbps; continuing to AUR packages")
		}
	}

	if flatpak.Available() {
		ui.Info("updating Flatpak applications...")
		if err := flatpak.Update(ui.NoConfirm); err != nil {
			ui.Warn("flatpak update failed: %v", err)
		}
	}

	m, err := manifest.Load(a.Cfg.ManifestFile)
	if err != nil {
		return err
	}
	if m.Empty() {
		ui.Info("no AUR packages tracked by vpa yet")
		return nil
	}

	names := make([]string, 0, len(m.Entries))
	for name := range m.Entries {
		names = append(names, name)
	}
	sort.Strings(names)

	infos, err := aurapi.Info(names...)
	if err != nil {
		return fmt.Errorf("AUR lookup failed: %w", err)
	}

	var outdated []string
	for _, name := range names {
		entry := m.Entries[name]
		p, ok := aurapi.ByName(infos, name)
		if !ok {
			ui.Warn("'%s' no longer found on AUR", name)
			continue
		}
		if p.Version != entry.Version {
			ui.Info("%s: %s -> %s", name, entry.Version, p.Version)
			outdated = append(outdated, name)
			continue
		}
		if a.Cfg.Devel && entry.Commit != "" {
			dir := a.gitDir(p.PackageBase)
			if err := gitutil.CloneAUR(p.PackageBase, dir); err != nil {
				ui.Warn("devel check failed for %s: %v", name, err)
				continue
			}
			pkg, err := pkgbuild.Load(dir)
			if err != nil {
				continue
			}
			latest := pkg.DevelLatestCommit()
			if latest != "" && latest != entry.Commit {
				ui.Info("%s: devel commit changed (%s -> %s)", name, short(entry.Commit), short(latest))
				outdated = append(outdated, name)
			}
		}
	}

	if len(outdated) == 0 {
		ui.Ok("everything up to date")
		return nil
	}
	if !ui.Confirm("Rebuild %d package(s): %s?", len(outdated), strings.Join(outdated, " ")) {
		return fmt.Errorf("aborted")
	}
	return a.cmdInstall(outdated)
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func (a *App) cmdClean() error {
	ui.Info("removing build cache: %s", a.Cfg.BuildDir)
	entries, _ := os.ReadDir(a.Cfg.BuildDir)
	for _, e := range entries {
		os.RemoveAll(filepath.Join(a.Cfg.BuildDir, e.Name()))
	}
	if ui.Confirm("Also clear the local package repo at %s (rebuilt automatically as needed)?", a.Cfg.RepoDir) {
		for _, pat := range []string{"*.xbps", "*-repodata", "*-stagedata"} {
			matches, _ := filepath.Glob(filepath.Join(a.Cfg.RepoDir, pat))
			for _, m := range matches {
				os.Remove(m)
			}
		}
		ui.Ok("cleared %s", a.Cfg.RepoDir)
	}
	// Unused Flatpak runtimes are where the disk space usually is.
	if flatpak.Available() {
		if ui.Confirm("Also remove Flatpak runtimes nothing still needs?") {
			if err := flatpak.RemoveUnused(ui.NoConfirm); err != nil {
				ui.Warn("removing unused Flatpak runtimes failed: %v", err)
			}
		}
	}
	ui.Ok("cleaned")
	return nil
}

// cmdList lists every installed package on the system, tagging the ones
// vpa built from the AUR. `--aur` narrows it to just those.
func (a *App) cmdList(args []string) error {
	m, err := manifest.Load(a.Cfg.ManifestFile)
	if err != nil {
		return err
	}

	aurOnly := false
	for _, arg := range args {
		if arg == "--aur" || arg == "-a" {
			aurOnly = true
		}
	}

	if aurOnly {
		if m.Empty() {
			ui.Info("vpa hasn't built any AUR packages yet")
			return nil
		}
		names := make([]string, 0, len(m.Entries))
		for name := range m.Entries {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			e := m.Entries[name]
			devel := ""
			if e.Commit != "" {
				devel = "(devel)"
			}
			fmt.Printf("%-30s %-15s %s\n", name, e.Version, devel)
		}
		return nil
	}

	installed, err := xbpsutil.ListInstalled()
	if err != nil {
		return fmt.Errorf("couldn't list installed packages: %w", err)
	}
	sort.Slice(installed, func(i, j int) bool { return installed[i].Name < installed[j].Name })
	if flatpak.Available() {
		if apps, err := flatpak.List(); err == nil {
			for _, app := range apps {
				fmt.Printf("%-40s %s (flatpak)\n", app.ID, app.Version)
			}
		}
	}
	for _, p := range installed {
		tag := ""
		if e, ok := m.Get(p.Name); ok {
			tag = " (aur)"
			if e.Commit != "" {
				tag = " (aur, devel)"
			}
		}
		fmt.Printf("%-40s %s%s\n", p.Name, p.Version, tag)
	}
	return nil
}
