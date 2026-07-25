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
                                  update if upstream moved past pkgver
--parallel=<N>                - Concurrent source downloads per package
                                  (default: 4)
--help                         - Show this message, or a subcommand's own
                                  usage if one was given (e.g. vur install --help)

SUBCOMMANDS:
search (s) <term>             - Search the AUR
info <pkg>                     - Show information about <package>
install (i) <pkg(s)>           - Build and install <package(s)> from the AUR
                                  (no exact match opens an interactive
                                  numbered picker over the search results)
remove (rm) <pkg(s)>            - Remove <package(s)> (via xbps-remove)
update (up, upgrade, su)       - Update everything: vur itself (if a newer
                                  version exists), a full system upgrade,
                                  then any vur-tracked AUR package
clean (cl)                     - Wipe the build cache and local package repo
list (ls)                      - List packages vur has installed
help (h, ?)                    - Show this message, or run 'vur help <cmd>'
                                  for a subcommand's own usage
helppager (hp)                 - Show usage information (piped to $PAGER)

PACMAN-STYLE FLAGS:
Also accepts pacman's own syntax, with pacman's semantics:
-S <pkg>      install          -Ss <term>    search
-Sy           refresh repos    -Si <pkg>     package info
-Su           upgrade          -Sc / -Scc    clean package cache
-Syu / -Syyu  refresh+upgrade  -U <file>     install a local package file
-R <pkg>      remove           -Rs <pkg>     remove + unneeded deps
-Q            list installed   -Qi <pkg>     installed package info
-Qs <term>    search installed -Ql <pkg>     list a package's files
-Qe           explicitly installed          -Qo <file>  which package owns file
-Qdt          orphans

CONFIG FILE:
~/.config/vur/vur.conf (NOCONFIRM=1, EDITOR=..., CLEAN_AFTER=1)
`, xbpsVer)
}

func usage() {
	fmt.Print(usageText())
}

func helpPager() {
	// $PAGER commonly carries arguments (e.g. "less -R"); treating the whole
	// string as one literal binary name would fail to even start.
	fields := strings.Fields(os.Getenv("PAGER"))
	if len(fields) == 0 {
		fields = []string{"less"}
	}
	cmd := exec.Command(fields[0], fields[1:]...)
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
	wantHelp := false
	var pacOp pacmanOp
	havePacOp := false
	for _, a := range os.Args[1:] {
		if p, ok := parsePacmanOp(a); ok {
			pacOp, havePacOp = p, true
			continue
		}
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
		case a == "--help" || a == "-h":
			wantHelp = true
		default:
			rest = append(rest, a)
		}
	}

	// pacman-style invocation (-S, -Syu, -Rns, -Qi, ...) takes over
	// entirely; everything left in rest is its operands.
	if havePacOp {
		app := &App{Cfg: cfg}
		if err := app.runPacmanOp(pacOp, rest); err != nil {
			ui.Die("%v", err)
		}
		return
	}

	var rawCmd string
	if len(rest) > 0 {
		rawCmd = rest[0]
		rest = rest[1:]
	}
	cmd := canonicalCommand(rawCmd)

	// --help/-h anywhere on the line shows that subcommand's own usage if
	// one was given (e.g. `vur install --help`), otherwise the general one.
	if wantHelp {
		if cmd == "" || !printCommandHelp(cmd) {
			usage()
		}
		return
	}

	// `vur help <cmd>` shows <cmd>'s own usage; bare `vur help`/`h`/`?` (or
	// no command at all) shows the general one.
	if cmd == "help" || rawCmd == "" {
		if len(rest) > 0 && printCommandHelp(rest[0]) {
			return
		}
		usage()
		return
	}

	app := &App{Cfg: cfg}

	var runErr error
	switch cmd {
	case "search":
		if len(rest) == 0 {
			printCommandHelp(cmd)
			os.Exit(1)
		}
		runErr = app.cmdSearch(rest)
	case "info":
		if len(rest) == 0 {
			printCommandHelp(cmd)
			os.Exit(1)
		}
		runErr = app.cmdInfo(rest)
	case "install":
		if len(rest) == 0 {
			printCommandHelp(cmd)
			os.Exit(1)
		}
		runErr = app.cmdInstall(rest)
	case "remove":
		if len(rest) == 0 {
			printCommandHelp(cmd)
			os.Exit(1)
		}
		runErr = app.cmdRemove(rest)
	case "update":
		runErr = app.cmdUpdate()
	case "clean":
		runErr = app.cmdClean()
	case "list":
		runErr = app.cmdList()
	case "helppager":
		helpPager()
	default:
		ui.Die("unknown command '%s' (try: vur help)", rawCmd)
	}

	if runErr != nil {
		ui.Die("%v", runErr)
	}
}
