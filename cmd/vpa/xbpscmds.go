package main

import (
	"fmt"

	"vpa/internal/ui"
	"vpa/internal/xbpsutil"
)

// The commands in this file expose the rest of xbps's day-to-day surface as
// first-class vpa subcommands, so managing a Void system never needs to drop
// back to raw xbps-* invocations.

func (a *App) cmdFiles(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa files <pkg>")
	}
	return xbpsutil.Files(args[0], a.Cfg.RepoDir)
}

func (a *App) cmdOwns(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa owns <file>")
	}
	return xbpsutil.Owns(args[0])
}

func (a *App) cmdDeps(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa deps <pkg>")
	}
	return xbpsutil.Deps(args[0], a.Cfg.RepoDir)
}

func (a *App) cmdRevDeps(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa revdeps <pkg>")
	}
	return xbpsutil.RevDeps(args[0], a.Cfg.RepoDir)
}

func (a *App) cmdOrphans() error { return xbpsutil.Orphans() }

func (a *App) cmdAutoremove() error {
	if !ui.Confirm("Remove all orphaned packages?") {
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

func (a *App) cmdRepos() error { return xbpsutil.Repos() }

func (a *App) cmdHold(args []string) error {
	if len(args) == 0 {
		return xbpsutil.ListHeld()
	}
	if err := xbpsutil.Hold(args); err != nil {
		return err
	}
	ui.Ok("held: %v", args)
	return nil
}

func (a *App) cmdUnhold(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vpa unhold <pkg> [pkg...]")
	}
	if err := xbpsutil.Unhold(args); err != nil {
		return err
	}
	ui.Ok("unheld: %v", args)
	return nil
}
