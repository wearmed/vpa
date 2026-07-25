// vur -- Void User Repository: an AUR helper for Void Linux.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"vur/internal/config"
	"vur/internal/sysutil"
	"vur/internal/ui"
)

func usageText() string {
	xbpsVer, _ := sysutil.Output("xbps-query", "-V")
	xbpsVer = strings.TrimSpace(xbpsVer)
	if xbpsVer == "" {
		xbpsVer = "unknown"
	}
	return fmt.Sprintf(`vur - AUR helper for Void Linux
XBPS version: %s

USAGE:
vur [OPTIONS] [SUBCOMMANDS] [<ARGS>]

OPTIONS:
--color=<yes|no|auto>        - Enable/disable colorized output (default: auto)
--noconfirm, -y               - Never prompt for confirmation
--edit                         - Open each PKGBUILD in $EDITOR before building
--devel                        - Also rebuild -git/-svn/-hg packages on
                                  upgrade if upstream moved past pkgver
--parallel=<N>                - Concurrent source downloads per package
                                  (default: 4)
--help                         - (same as: help)

SUBCOMMANDS:
search (s) <term>             - Search the AUR
info <pkg>                     - Show information about <package>
install (i) <pkg(s)>           - Build and install <package(s)> from the AUR
                                  (no exact match opens an interactive
                                  numbered picker over the search results)
remove (rm) <pkg(s)>            - Remove <package(s)> (via xbps-remove)
upgrade (up)                   - Offer a full system upgrade, then rebuild
                                  any vur-tracked package with a newer
                                  AUR version
clean (cl)                     - Wipe the build cache and local package repo
list (ls)                      - List packages vur has installed
help                            - Show usage information
helppager (hp)                 - Show usage information (piped to $PAGER)

CONFIG FILE:
~/.config/vur/vur.conf (NOCONFIRM=1, EDITOR=..., CLEAN_AFTER=1)
`, xbpsVer)
}

func usage() {
	fmt.Print(usageText())
}

func helpPager() {
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less"
	}
	cmd := exec.Command(pager)
	cmd.Stdin = strings.NewReader(usageText())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		usage()
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		ui.Die("%v", err)
	}
	ui.NoConfirm = cfg.NoConfirm
	ui.SetColors("auto")

	var rest []string
	for _, a := range os.Args[1:] {
		switch {
		case a == "--noconfirm" || a == "-y":
			cfg.NoConfirm = true
			ui.NoConfirm = true
		case a == "--edit":
			cfg.EditPKGBUILD = true
		case a == "--devel":
			cfg.Devel = true
		case strings.HasPrefix(a, "--color="):
			ui.SetColors(strings.TrimPrefix(a, "--color="))
		case strings.HasPrefix(a, "--parallel="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--parallel=")); err == nil && n > 0 {
				cfg.Parallel = n
			}
		case a == "--help":
			usage()
			os.Exit(0)
		default:
			rest = append(rest, a)
		}
	}

	var cmd string
	if len(rest) > 0 {
		cmd = rest[0]
		rest = rest[1:]
	}

	app := &App{Cfg: cfg}

	var runErr error
	switch cmd {
	case "search", "s":
		runErr = app.cmdSearch(rest)
	case "info":
		runErr = app.cmdInfo(rest)
	case "install", "i":
		runErr = app.cmdInstall(rest)
	case "remove", "rm":
		runErr = app.cmdRemove(rest)
	case "upgrade", "up":
		runErr = app.cmdUpgrade()
	case "clean", "cl":
		runErr = app.cmdClean()
	case "list", "ls":
		runErr = app.cmdList()
	case "helppager", "hp":
		helpPager()
	case "", "-h", "help":
		usage()
	default:
		ui.Die("unknown command '%s' (try: vur help)", cmd)
	}

	if runErr != nil {
		ui.Die("%v", runErr)
	}
}
