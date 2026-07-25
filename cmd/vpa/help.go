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
	case "help", "h", "?":
		return "help"
	case "helppager", "hp":
		return "helppager"
	default:
		return ""
	}
}

var commandHelp = map[string]string{
	"search": `vpa search (s) <term>

Search the AUR by package name and description.

Example:
  vpa search pipes
`,
	"info": `vpa info <pkg>

Show full AUR package details for an exact package name.

Example:
  vpa info pipes.sh
`,
	"install": `vpa install (i) <pkg> [pkg...]

Build and install package(s). Each argument can be:
  - an exact AUR package name
  - a search term with no exact match (opens a numbered picker)
  - a path or URL to a .xbps/.deb/.rpm/.pkg.tar.zst file
  - a name already available in Void's own repos (installs directly)

Relevant flags: --edit, --parallel=<N>, --noconfirm/-y

Examples:
  vpa install pipes.sh
  vpa i -y --edit somefuzzyterm
  vpa i ./something.deb
  vpa i ./something.xbps
`,
	"remove": `vpa remove (rm) <pkg> [pkg...]

Remove installed package(s), via xbps-remove.

Example:
  vpa remove pipes.sh
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
	"clean": `vpa clean (cl)

Wipe the build cache, and optionally the local package repo (you'll
be asked before that part happens).
`,
	"list": `vpa list (ls)

List packages vpa has installed, with version, and whether it's a
tracked -git/-svn/-hg devel package.
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
