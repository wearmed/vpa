package main

import "fmt"

// canonicalCommand resolves any recognized command name or alias to its
// canonical form, or "" if unrecognized. The single source of truth for
// aliases, used by both dispatch and help lookup so they can never drift
// apart.
func canonicalCommand(name string) string {
	switch name {
	case "search", "s":
		return "search"
	case "info":
		return "info"
	case "install", "i":
		return "install"
	case "remove", "rm":
		return "remove"
	case "update", "up", "upgrade", "su":
		return "update"
	case "clean", "cl":
		return "clean"
	case "list", "ls":
		return "list"
	case "files", "fl":
		return "files"
	case "owns", "wp", "whatprovides":
		return "owns"
	case "deps":
		return "deps"
	case "revdeps", "rv":
		return "revdeps"
	case "orphans":
		return "orphans"
	case "autoremove", "ar":
		return "autoremove"
	case "reconfigure", "rc":
		return "reconfigure"
	case "repos", "lr", "listrepos":
		return "repos"
	case "hold":
		return "hold"
	case "unhold":
		return "unhold"
	case "help", "h", "?":
		return "help"
	case "helppager", "hp":
		return "helppager"
	default:
		return ""
	}
}

var commandHelp = map[string]string{
	"search": `vpa search (s) <term>        [pacman: -Ss]

Search both Void's repositories and the AUR by name and description.
Void results are listed first (a native package is nearly always the
better choice when one exists); anything already installed is marked.

Example:
  vpa search pipes
`,
	"info": `vpa info <pkg>               [pacman: -Si]

Show full details for a package, from wherever it exists -- Void's
repositories, the AUR, or both.

Example:
  vpa info firefox
  vpa info pipes.sh
`,
	"install": `vpa install (i) <pkg> [pkg...]   [pacman: -S]

Install package(s) from anywhere. Each argument can be:
  - a package in Void's repos (installed directly, nothing is built)
  - an AUR package name (PKGBUILD reviewed, built, packaged as .xbps)
  - a search term with no exact match (opens a numbered picker)
  - a .xbps file or URL (installed directly)
  - a .deb/.rpm/.pkg.tar.zst file or URL (extracted and repackaged
    as a real .xbps; packaging only, it can't fix cross-distro ABI
    differences)

Relevant flags: --edit, --parallel=<N>, --noconfirm/-y

Examples:
  vpa install firefox
  vpa install pipes.sh
  vpa i -y --edit somefuzzyterm
  vpa i ./something.deb
  vpa i ./something.xbps
`,
	"remove": `vpa remove (rm) <pkg> [pkg...]   [pacman: -R, -Rs]

Remove installed package(s). Use pacman's -Rs form to also remove
dependencies that nothing else needs anymore.

Examples:
  vpa remove firefox
  vpa -Rs pipes.sh
`,
	"update": `vpa update (up, upgrade, su)

Update everything:
  1. vpa itself, if a newer version is available (silent if current)
  2. a full system upgrade (sudo xbps-install -Su)
  3. any vpa-tracked AUR package with a newer version

Relevant flags: --devel (also rebuild -git/-svn/-hg packages if
upstream moved past pkgver even though the version string didn't
change), --noconfirm/-y

Examples:
  vpa update
  vpa update --devel
  vpa -Syu
`,
	"clean": `vpa clean (cl)               [pacman: -Sc, -Scc]

Wipe vpa's build cache, and optionally the local package repo (you'll
be asked before that part happens). The pacman forms also clean xbps's
own package cache: -Sc drops outdated packages, -Scc drops all of them.
`,
	"list": `vpa list (ls) [--aur]        [pacman: -Q]

List every installed package on the system, tagging the ones vpa built
from the AUR. Pass --aur (or -a) to list only those.

Related: vpa -Qe (explicitly installed), vpa orphans
`,
	"files": `vpa files (fl) <pkg>         [pacman: -Ql]

List the files a package owns.
`,
	"owns": `vpa owns (wp) <file>         [pacman: -Qo]

Show which package owns a file.

Example:
  vpa owns /usr/bin/firefox
`,
	"deps": `vpa deps <pkg>

Show a package's dependencies.
`,
	"revdeps": `vpa revdeps (rv) <pkg>

Show what depends on a package (reverse dependencies).
`,
	"orphans": `vpa orphans                  [pacman: -Qdt]

List packages that were installed as dependencies and are no longer
needed by anything. Use 'vpa autoremove' to remove them.
`,
	"autoremove": `vpa autoremove (ar)

Remove all orphaned packages (see 'vpa orphans').
`,
	"reconfigure": `vpa reconfigure (rc) <pkg|all>

Re-run a package's configuration step. Pass 'all' to reconfigure every
installed package.
`,
	"repos": `vpa repos (lr)

List the configured xbps repositories.
`,
	"hold": `vpa hold [pkg...]

Hold packages back from updates. With no arguments, lists what's
currently held.
`,
	"unhold": `vpa unhold <pkg> [pkg...]

Release packages previously held back by 'vpa hold'.
`,
}

// printCommandHelp prints canonical name's specific help text if known,
// returning whether it found one.
func printCommandHelp(name string) bool {
	h, ok := commandHelp[canonicalCommand(name)]
	if ok {
		fmt.Print(h)
	}
	return ok
}
