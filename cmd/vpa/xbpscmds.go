package main

import (
	"fmt"

	"vpa/internal/ui"
	"vpa/internal/xbpsutil"
)

// The commands in this file cover the rest of xbps's day-to-day surface, so
// managing a Void system never needs to drop back to raw xbps-* invocations.
// Names and aliases follow vpm's, since that's the xbps front-end people
// already know.

func (a *App) cmdSync() error {
	if !ui.Confirm("Refresh repository data?") {
		return nil
	}
	return xbpsutil.SyncRepos(ui.NoConfirm)
}

func (a *App) cmdFileList(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa filelist <pkg>")
	}
	return xbpsutil.Files(args[0], a.Cfg.RepoDir)
}

func (a *App) cmdWhatProvides(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa whatprovides <file>")
	}
	out, _ := xbpsutil.Owns(args[0])
	if out == "" {
		ui.Info("no installed package owns %s", args[0])
		return nil
	}
	fmt.Println(out)
	return nil
}

func (a *App) cmdSearchFile(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa searchfile <file>")
	}
	out, _ := xbpsutil.SearchFile(args[0])
	if out == "" {
		ui.Info("no installed package has a file matching %q", args[0])
		return nil
	}
	fmt.Println(out)
	return nil
}

func (a *App) cmdDeps(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa deps <pkg>")
	}
	return xbpsutil.Deps(args[0], a.Cfg.RepoDir)
}

func (a *App) cmdReverse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa reverse <pkg>")
	}
	return xbpsutil.RevDeps(args[0], a.Cfg.RepoDir)
}

func (a *App) cmdOrphans() error {
	out, _ := xbpsutil.Orphans()
	if out == "" {
		ui.Ok("nothing to clean up -- no orphaned packages")
		return nil
	}
	fmt.Println(out)
	ui.Info("remove these with: vpa autoremove")
	return nil
}

func (a *App) cmdAutoremove() error {
	out, _ := xbpsutil.Orphans()
	if out == "" {
		ui.Ok("nothing to remove -- no orphaned packages")
		return nil
	}
	fmt.Println(out)
	if !ui.Confirm("Remove the above packages?") {
		return nil
	}
	return xbpsutil.Autoremove(ui.NoConfirm)
}

func (a *App) cmdReconfigure(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa reconfigure <pkg|all>")
	}
	return xbpsutil.Reconfigure(args[0])
}

func (a *App) cmdListRepos() error { return xbpsutil.Repos() }

func (a *App) cmdAddRepo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa addrepo <url>")
	}
	url := args[0]
	ui.Warn("This adds %s as a package source for your whole system. Only add repositories you trust -- packages from it can install anything, as root.", url)
	if !ui.Confirm("Add this repository?") {
		return nil
	}
	if err := xbpsutil.AddRepo(url); err != nil {
		return err
	}
	ui.Ok("added %s", url)
	return xbpsutil.SyncRepos(ui.NoConfirm)
}

func (a *App) cmdForceInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa forceinstall <pkg> [pkg...]")
	}
	ui.Warn("Force-installing overwrites files already on disk. This is for repairing a broken package, not normal installs -- use 'vpa install' for those.")
	if !ui.Confirm("Force-install %v?", args) {
		return nil
	}
	return xbpsutil.ForceInstall(a.Cfg.RepoDir, args...)
}

func (a *App) cmdRemoveRecursive(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa removerecursive <pkg> [pkg...]")
	}
	if !ui.Confirm("Remove %v, plus any dependencies nothing else needs?", args) {
		return nil
	}
	if err := xbpsutil.RemoveRecursive(args...); err != nil {
		return err
	}
	return a.forgetRemoved(args)
}

func (a *App) cmdDevInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa devinstall <pkg> [pkg...]")
	}
	// A -devel package only exists for some packages; include the ones that
	// really do rather than failing the whole install on the ones that don't.
	var want []string
	for _, p := range args {
		want = append(want, p)
		devel := p + "-devel"
		if xbpsutil.IsAvailable(devel, a.Cfg.RepoDir) {
			want = append(want, devel)
		} else {
			ui.Info("no %s package exists; installing %s on its own", devel, p)
		}
	}
	return a.cmdInstall(want)
}

func (a *App) cmdListAlternatives() error { return xbpsutil.ListAlternatives() }

func (a *App) cmdSetAlternative(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa setalternative <pkg>")
	}
	return xbpsutil.SetAlternative(args[0])
}

func (a *App) cmdHold(args []string) error {
	if len(args) == 0 {
		return xbpsutil.ListHeld()
	}
	if err := xbpsutil.Hold(args); err != nil {
		return err
	}
	ui.Ok("held back from updates: %v", args)
	return nil
}

func (a *App) cmdUnhold(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa unhold <pkg> [pkg...]")
	}
	if err := xbpsutil.Unhold(args); err != nil {
		return err
	}
	ui.Ok("no longer held: %v", args)
	return nil
}
