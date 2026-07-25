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
	"search": `vur search (s) <term>

Search the AUR by package name and description.

Example:
  vur search pipes
`,
	"info": `vur info <pkg>

Show full AUR package details for an exact package name.

Example:
  vur info pipes.sh
`,
	"install": `vur install (i) <pkg> [pkg...]

Build and install package(s). Each argument can be:
  - an exact AUR package name
  - a search term with no exact match (opens a numbered picker)
  - a path or URL to a .xbps/.deb/.rpm/.pkg.tar.zst file
  - a name already available in Void's own repos (installs directly)

Relevant flags: --edit, --parallel=<N>, --noconfirm/-y

Examples:
  vur install pipes.sh
  vur i -y --edit somefuzzyterm
  vur i ./something.deb
  vur i ./something.xbps
`,
	"remove": `vur remove (rm) <pkg> [pkg...]

Remove installed package(s), via xbps-remove.

Example:
  vur remove pipes.sh
`,
	"update": `vur update (up, upgrade, su)

Update everything:
  1. vur itself, if a newer version is available (silent if current)
  2. a full system upgrade (sudo xbps-install -Su)
  3. any vur-tracked AUR package with a newer version

Relevant flags: --devel (also rebuild -git/-svn/-hg packages if
upstream moved past pkgver even though the version string didn't
change), --noconfirm/-y

Examples:
  vur update
  vur update --devel
  vur -Syu
`,
	"clean": `vur clean (cl)

Wipe the build cache, and optionally the local package repo (you'll
be asked before that part happens).
`,
	"list": `vur list (ls)

List packages vur has installed, with version, and whether it's a
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
