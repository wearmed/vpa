package main

import (
	"fmt"
	"os"
	"strings"

	"vpa/internal/sysutil"
	"vpa/internal/ui"
)

// pacmanOp is a parsed pacman-style invocation (e.g. "-Syu", "-Rns").
type pacmanOp struct {
	op   byte           // primary operation: S, R, Q, U
	mods map[byte]int   // modifier letter -> how many times it appeared (yy, cc, ...)
}

func (p pacmanOp) has(m byte) bool  { return p.mods[m] > 0 }
func (p pacmanOp) count(m byte) int { return p.mods[m] }

// parsePacmanOp recognizes a pacman-style bundled short-flag argument
// ("-Syu", "-Rns", "-Qi", ...). Returns ok=false for anything else,
// including vpa's own long flags and bare "-y"/"-h" (which stay
// --noconfirm and --help respectively rather than being reinterpreted).
func parsePacmanOp(arg string) (pacmanOp, bool) {
	if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
		return pacmanOp{}, false
	}
	letters := arg[1:]
	op := letters[0]
	if op != 'S' && op != 'R' && op != 'Q' && op != 'U' {
		return pacmanOp{}, false
	}
	p := pacmanOp{op: op, mods: map[byte]int{}}
	for i := 1; i < len(letters); i++ {
		p.mods[letters[i]]++
	}
	return p, true
}

// runPacmanOp executes a parsed pacman-style operation, mapping it onto
// the equivalent xbps/vpa behavior. Semantics follow pacman's: the
// modifiers decompose the same way (-Sy refreshes only, -Su upgrades
// only, -Syu does both), and the -Q family queries installed packages
// rather than remote ones.
func (a *App) runPacmanOp(p pacmanOp, args []string) error {
	switch p.op {
	case 'S':
		return a.pacmanSync(p, args)
	case 'R':
		return a.pacmanRemove(p, args)
	case 'Q':
		return a.pacmanQuery(p, args)
	case 'U':
		if len(args) == 0 {
			return fmt.Errorf("-U needs one or more package files (e.g. vpa -U ./foo.xbps)")
		}
		return a.cmdInstall(args)
	}
	return fmt.Errorf("unsupported operation -%c", p.op)
}

func (a *App) pacmanSync(p pacmanOp, args []string) error {
	switch {
	case p.has('s'):
		if len(args) == 0 {
			return fmt.Errorf("-Ss needs a search term")
		}
		return a.cmdSearch(args)
	case p.has('i'):
		if len(args) == 0 {
			return fmt.Errorf("-Si needs a package name")
		}
		return a.cmdInfo(args)
	case p.has('c'):
		// -Sc removes outdated packages from the cache, -Scc also removes
		// uninstalled ones -- xbps-remove -O/-OO map exactly.
		flag := "-O"
		if p.count('c') > 1 {
			flag = "-OO"
		}
		xargs := []string{"xbps-remove", flag}
		if ui.NoConfirm {
			xargs = append(xargs, "-y")
		}
		if err := sysutil.RunInteractive("sudo", xargs...); err != nil {
			return fmt.Errorf("cache clean failed: %w", err)
		}
		return a.cmdClean()
	}

	refresh := p.has('y')
	upgrade := p.has('u')

	if len(args) > 0 {
		// -S <pkg> (optionally with y/u alongside, as pacman allows)
		if refresh {
			if err := a.syncRepos(p.count('y') > 1); err != nil {
				return err
			}
		}
		return a.cmdInstall(args)
	}

	switch {
	case refresh && upgrade:
		if err := a.syncRepos(p.count('y') > 1); err != nil {
			return err
		}
		return a.cmdUpdate()
	case refresh:
		return a.syncRepos(p.count('y') > 1)
	case upgrade:
		return a.cmdUpdate()
	}
	return fmt.Errorf("-S needs a package name (or use -Sy/-Su/-Syu)")
}

// syncRepos refreshes repository indexes. force (-Syy) additionally drops
// vpa's own sync-freshness marker so the next install re-syncs too, which
// is the closest equivalent to pacman's "force refresh even if it looks
// current" -- xbps-install -S always re-fetches repodata regardless, so
// there's no separate "extra forceful" fetch to ask it for.
func (a *App) syncRepos(force bool) error {
	if force {
		os.Remove(a.Cfg.CacheDir + "/last-repo-sync")
	}
	args := []string{"xbps-install", "-S"}
	if ui.NoConfirm {
		args = append(args, "-y")
	}
	if err := sysutil.RunInteractive("sudo", args...); err != nil {
		return fmt.Errorf("repository sync failed: %w", err)
	}
	ui.Ok("repositories synced")
	return nil
}

func (a *App) pacmanRemove(p pacmanOp, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("-R needs one or more package names")
	}
	// -Rs/-Rns recursively removes now-unneeded dependencies too; xbps-remove
	// -R is the direct equivalent. pacman's 'n' (purge config files) has no
	// xbps counterpart -- xbps always removes non-modified config files and
	// deliberately preserves modified ones -- so it's accepted and ignored
	// rather than silently implying something it can't do.
	if p.has('n') {
		ui.Warn("-n (purge config files) has no xbps equivalent -- xbps keeps modified config files by design; removing normally")
	}
	xargs := []string{"xbps-remove"}
	if p.has('s') {
		xargs = append(xargs, "-R")
	}
	xargs = append(xargs, "-y")
	xargs = append(xargs, args...)
	if err := sysutil.RunInteractive("sudo", xargs...); err != nil {
		return fmt.Errorf("xbps-remove failed")
	}
	return a.forgetRemoved(args)
}

func (a *App) pacmanQuery(p pacmanOp, args []string) error {
	switch {
	case p.has('o'):
		if len(args) == 0 {
			return fmt.Errorf("-Qo needs a file path")
		}
		return sysutil.RunInteractive("xbps-query", "-o", args[0])
	case p.has('l'):
		if len(args) == 0 {
			return fmt.Errorf("-Ql needs a package name")
		}
		return sysutil.RunInteractive("xbps-query", "-f", args[0])
	case p.has('i'):
		if len(args) == 0 {
			return fmt.Errorf("-Qi needs a package name")
		}
		return sysutil.RunInteractive("xbps-query", "-S", args[0])
	case p.has('s'):
		if len(args) == 0 {
			return fmt.Errorf("-Qs needs a search term")
		}
		// Installed-only search: xbps-query -s searches repos too, so filter
		// to the [*] (installed) rows the way pacman -Qs would.
		out, _ := sysutil.Output("xbps-query", "-s", args[0])
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "[*]") {
				fmt.Println(line)
			}
		}
		return nil
	case p.has('d') && p.has('t'):
		return sysutil.RunInteractive("xbps-query", "-O")
	case p.has('e'):
		return sysutil.RunInteractive("xbps-query", "-m")
	}
	return sysutil.RunInteractive("xbps-query", "-l")
}
