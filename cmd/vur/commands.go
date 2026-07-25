package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"vur/internal/aurapi"
	"vur/internal/buildpkg"
	"vur/internal/config"
	"vur/internal/deps"
	"vur/internal/gitutil"
	"vur/internal/manifest"
	"vur/internal/pkgbuild"
	"vur/internal/sysutil"
	"vur/internal/ui"
	"vur/internal/xbpsutil"
)

type App struct {
	Cfg *config.Config
}

func (a *App) gitDir(pkgbase string) string {
	return buildpkg.NewDirs(a.Cfg.BuildDir, pkgbase).Git
}

func (a *App) cmdSearch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vur search <term>")
	}
	pkgs, err := aurapi.Search(args[0])
	if err != nil {
		return fmt.Errorf("AUR search request failed: %w", err)
	}
	if len(pkgs) == 0 {
		ui.Info("no AUR results for '%s'", args[0])
		return nil
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })
	for _, p := range pkgs {
		fmt.Printf("%s %s\n", ui.Bold("aur/"+p.Name), p.Version)
		fmt.Printf("    %s\n", p.Description)
	}
	return nil
}

func (a *App) cmdInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vur info <pkg>")
	}
	pkgs, err := aurapi.Info(args[0])
	if err != nil {
		return fmt.Errorf("AUR info request failed: %w", err)
	}
	p, ok := aurapi.ByName(pkgs, args[0])
	if !ok {
		return fmt.Errorf("package '%s' not found in AUR", args[0])
	}
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
		return fmt.Errorf("usage: vur install <pkg> [pkg...]")
	}

	var foreignArgs, aurArgs []string
	for _, p := range pkgs {
		if isForeignPkgArg(p) {
			foreignArgs = append(foreignArgs, p)
		} else {
			aurArgs = append(aurArgs, p)
		}
	}
	for _, f := range foreignArgs {
		if err := a.installForeign(f); err != nil {
			return err
		}
	}
	if len(aurArgs) == 0 {
		return nil
	}

	sysutil.RequireBin("git", "git")
	sysutil.RequireBin("fakeroot", "fakeroot")
	sysutil.RequireBin("xbps-create", "xbps")

	infos, err := aurapi.Info(aurArgs...)
	if err != nil {
		return fmt.Errorf("AUR lookup failed: %w", err)
	}

	var bases []string
	for _, name := range aurArgs {
		if p, ok := aurapi.ByName(infos, name); ok {
			bases = append(bases, p.PackageBase)
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
			bases = append(bases, base)
		}
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
			m.Set(name, pkg.Ver+"-"+pkg.Rel, commit)
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
		return fmt.Errorf("usage: vur remove <pkg> [pkg...]")
	}
	if err := xbpsutil.Remove(pkgs...); err != nil {
		return err
	}
	m, err := manifest.Load(a.Cfg.ManifestFile)
	if err != nil {
		return err
	}
	for _, p := range pkgs {
		m.Remove(p)
	}
	if err := m.Save(); err != nil {
		return err
	}
	ui.Ok("removed: %s", strings.Join(pkgs, " "))
	return nil
}

func (a *App) cmdUpgrade() error {
	if ui.Confirm("Run a full system upgrade first (sudo xbps-install -Su)?") {
		if err := sysutil.RunInteractive("sudo", "xbps-install", "-Su"); err != nil {
			ui.Warn("system upgrade failed or was declined by xbps; continuing to AUR packages")
		}
	}

	m, err := manifest.Load(a.Cfg.ManifestFile)
	if err != nil {
		return err
	}
	if m.Empty() {
		ui.Info("nothing tracked by vur yet")
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
	ui.Ok("cleaned")
	return nil
}

func (a *App) cmdList() error {
	m, err := manifest.Load(a.Cfg.ManifestFile)
	if err != nil {
		return err
	}
	if m.Empty() {
		ui.Info("nothing tracked by vur")
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
