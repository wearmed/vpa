// vpa by wearmed -- the universal void linux package manager.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"vpa/internal/config"
	"vpa/internal/sysutil"
	"vpa/internal/ui"
)

// Version: a feature release bumps the tenths and clears the hundredths
// (1.12 -> 1.2), a bugfix adds a hundredth (1.2 -> 1.21).
const Version = "1.22"

func usageText() string {
	return `vpa by wearmed
the universal void linux package manager

COMMANDS:
  vpa install <name>      install something
  vpa remove <name>       uninstall something
  vpa search <term>       find something to install
  vpa search cat <what>   browse a category
  vpa search cat          list every category
  vpa info <name>         show details for a package
  vpa list                list what you have installed
  vpa update              update everything

FLAGS:
  --assumeno              ask before making changes
  --assumeyes             stop asking (the default)
  --flatpak               install from Flathub
  --edit                  edit an AUR build script first
  --version               show which version this is

HELP:
  vpa help <command>      how one command works
  vpa help --all          every command and flag
`
}

func usageFullText() string {
	xbpsVer, _ := sysutil.Output("xbps-query", "-V")
	xbpsVer = strings.TrimSpace(xbpsVer)
	if xbpsVer == "" {
		xbpsVer = "unknown"
	}
	return fmt.Sprintf(`vpa by wearmed
the universal void linux package manager
vpa version: %s   XBPS version: %s

USAGE:
vpa [OPTIONS] <COMMAND> [ARGS]

INSTALLING AND REMOVING:
install (i) <pkg(s)>          - Install from Void's repos, the AUR,
                                 Flathub, or a package file
devinstall (di) <pkg(s)>      - Install packages plus their -devel parts
forceinstall (fi) <pkg(s)>    - Reinstall, overwriting existing files
remove (rm) <pkg(s)>          - Remove packages
removerecursive (rr) <pkg(s)> - Remove packages and unneeded dependencies
update (up, upgrade)          - Update vpa, Flatpaks, your system, and
                                 AUR packages
sync (sy)                     - Refresh repository data only

FINDING THINGS:
search (s) <term>             - Search Void's repos, the AUR and Flathub
search cat <category>         - Browse a category (no name lists them all)
info <pkg>                    - Show details for a package
list (ls) [--aur]             - List installed packages (incl. Flatpaks)
filelist (fl) <pkg>           - List the files a package installs
whatprovides (wp) <file>      - Show which package a file came from
searchfile (sf) <file>        - Find installed packages containing a file
deps <pkg>                    - Show what a package needs
reverse (rv) <pkg>            - Show what depends on a package

LOOKING AFTER YOUR SYSTEM:
orphans                       - List no-longer-needed dependencies
autoremove (ar)               - Remove those orphaned packages
cleanup (cl)                  - Free up disk space
reconfigure (rc) <pkg|all>    - Re-run a package's setup step
hold [pkg(s)]                 - Stop packages being updated (or list held)
unhold <pkg(s)>               - Allow held packages to update again

REPOSITORIES AND ALTERNATIVES:
listrepos (lr)                - Show configured repositories
addrepo <url>                 - Add another repository
listalternatives (la)         - Show configurable defaults
setalternative (sa) <pkg>     - Choose which package provides one

HELP:
help (h, ?) [command]         - Show help, or help for one command
helppager (hp)                - Show this, piped through $PAGER

OPTIONS:
--assumeyes / --assumeno      - Set whether vpa asks before making changes
                                 (saved; assumeyes is the default)
--noconfirm, -y               - Don't ask anything this run, including an
                                 unseen AUR build script
--confirm                     - Ask before making changes, just this run
--edit                        - Edit an AUR build script before building
--devel                       - Also rebuild -git packages whose upstream
                                 code changed
--flatpak                     - Install the named packages from Flathub
--parallel=<N>                - Concurrent downloads (default: 4)
--color=<yes|no|auto>         - Colored output (default: auto)
--version, -V                 - Show which version of vpa this is
--help                        - Show help, or a command's own help

CONFIG FILE:
~/.config/vpa/vpa.conf (NOCONFIRM=1, EDITOR=..., CLEAN_AFTER=1,
                        TRUST_AUR=1, PREFER_FLATPAK=1, PARALLEL_DOWNLOADS=N,
                        STALE_DAYS=30, CATEGORY_URL=...)
~/.config/vpa/categories.conf  - your own search categories
`, Version, xbpsVer)
}

func usage() {
	fmt.Print(usageText())
}

func usageFull() {
	fmt.Print(usageFullText())
}

func helpPager() {
	// $PAGER commonly carries arguments (e.g. "less -R"); treating the whole
	// string as one literal binary name would fail to even start.
	fields := strings.Fields(os.Getenv("PAGER"))
	if len(fields) == 0 {
		fields = []string{"less"}
	}
	cmd := exec.Command(fields[0], fields[1:]...)
	cmd.Stdin = strings.NewReader(usageFullText())
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
	wantAll := false
	explicitYes := false
	var setDefault string
	for _, a := range os.Args[1:] {
		switch {
		case a == "--noconfirm" || a == "-y":
			cfg.NoConfirm = true
			ui.NoConfirm = true
			explicitYes = true
		case a == "--confirm":
			cfg.NoConfirm = false
			ui.NoConfirm = false
		case a == "--assumeyes":
			setDefault = "1"
		case a == "--assumeno":
			setDefault = "0"
		case a == "--edit":
			cfg.EditPKGBUILD = true
		case a == "--devel":
			cfg.Devel = true
		case a == "--flatpak":
			cfg.PreferFlatpak = true
		case strings.HasPrefix(a, "--color="):
			ui.SetColors(strings.TrimPrefix(a, "--color="))
		case strings.HasPrefix(a, "--parallel="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--parallel=")); err == nil && n > 0 {
				cfg.Parallel = n
			}
		case a == "--version" || a == "-V":
			fmt.Printf("vpa %s\n", Version)
			return
		case a == "--help" || a == "-h":
			wantHelp = true
		case a == "--all":
			wantAll = true
		default:
			rest = append(rest, a)
		}
	}

	if setDefault != "" {
		if err := config.SetOption(cfg.ConfigFile, "NOCONFIRM", setDefault); err != nil {
			ui.Die("couldn't save that setting: %v", err)
		}
		if setDefault == "1" {
			ui.Ok("vpa will assume yes from now on. Run 'vpa --assumeno' to go back to asking.")
			ui.Info("The one thing it still asks about is an AUR build script it hasn't seen before, since that runs a stranger's code on your machine. Set TRUST_AUR=1 in %s to skip that too.", cfg.ConfigFile)
		} else {
			ui.Ok("vpa will ask before making changes from now on. Run 'vpa --assumeyes' to stop asking.")
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
	// one was given (e.g. `vpa install --help`), otherwise the general one.
	if wantHelp {
		if wantAll {
			usageFull()
		} else if cmd == "" || !printCommandHelp(cmd) {
			usage()
		}
		return
	}

	// `vpa help <cmd>` shows that command's own usage, `vpa help --all`
	// shows every command, and bare `vpa help`/`h`/`?` (or no command at
	// all) shows the short overview.
	if cmd == "help" || rawCmd == "" {
		if wantAll {
			usageFull()
			return
		}
		if len(rest) > 0 {
			if rest[0] == "--all" || rest[0] == "-a" || rest[0] == "all" {
				usageFull()
				return
			}
			if printCommandHelp(rest[0]) {
				return
			}
			ui.Warn("no command called '%s' -- here's what vpa can do:", rest[0])
		}
		usage()
		return
	}

	app := &App{Cfg: cfg, ExplicitYes: explicitYes}

	var runErr error
	switch cmd {
	case "search":
		runErr = requireArgs(cmd, rest, app.cmdSearch)
	case "info":
		runErr = requireArgs(cmd, rest, app.cmdInfo)
	case "install":
		runErr = requireArgs(cmd, rest, app.cmdInstall)
	case "devinstall":
		runErr = requireArgs(cmd, rest, app.cmdDevInstall)
	case "forceinstall":
		runErr = requireArgs(cmd, rest, app.cmdForceInstall)
	case "remove":
		runErr = requireArgs(cmd, rest, app.cmdRemove)
	case "removerecursive":
		runErr = requireArgs(cmd, rest, app.cmdRemoveRecursive)
	case "update":
		runErr = app.cmdUpdate()
	case "sync":
		runErr = app.cmdSync()
	case "list":
		runErr = app.cmdList(rest)
	case "filelist":
		runErr = requireArgs(cmd, rest, app.cmdFileList)
	case "whatprovides":
		runErr = requireArgs(cmd, rest, app.cmdWhatProvides)
	case "searchfile":
		runErr = requireArgs(cmd, rest, app.cmdSearchFile)
	case "deps":
		runErr = requireArgs(cmd, rest, app.cmdDeps)
	case "reverse":
		runErr = requireArgs(cmd, rest, app.cmdReverse)
	case "orphans":
		runErr = app.cmdOrphans()
	case "autoremove":
		runErr = app.cmdAutoremove()
	case "reconfigure":
		runErr = requireArgs(cmd, rest, app.cmdReconfigure)
	case "listrepos":
		runErr = app.cmdListRepos()
	case "addrepo":
		runErr = requireArgs(cmd, rest, app.cmdAddRepo)
	case "listalternatives":
		runErr = app.cmdListAlternatives()
	case "setalternative":
		runErr = requireArgs(cmd, rest, app.cmdSetAlternative)
	case "cleanup":
		runErr = app.cmdClean()
	case "hold":
		runErr = app.cmdHold(rest)
	case "unhold":
		runErr = requireArgs(cmd, rest, app.cmdUnhold)
	case "version":
		fmt.Printf("vpa %s\n", Version)
	case "helppager":
		helpPager()
	default:
		ui.Die("I don't know the command '%s'. Run 'vpa help' to see what vpa can do.", rawCmd)
	}

	if runErr != nil {
		ui.Die("%v", runErr)
	}
}

// requireArgs runs fn, but shows the command's own usage instead if the
// user gave it nothing to work on -- more useful to a newcomer than an
// error string.
func requireArgs(cmd string, args []string, fn func([]string) error) error {
	if len(args) == 0 {
		printCommandHelp(cmd)
		os.Exit(1)
	}
	return fn(args)
}
