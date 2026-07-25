# vur

**V**oid **U**ser **R**epository — an AUR helper for Void Linux.

Void doesn't have anything like the AUR, but a lot of software only ships as
a PKGBUILD. `vur` grabs those from the AUR, builds them, and turns the result
into a real `.xbps` package via `xbps-create`, so it ends up tracked and
removable through `xbps` like anything else — not just files thrown onto
your filesystem by a `make install`.

It's written in Go and ships as one static binary, so there's no `jq` to
install and no pile of shell scripts to keep in sync. That said, a PKGBUILD
is just bash, so building one still means shelling out to `bash`, `git`,
`fakeroot`, and `xbps-*` under the hood — Go is mostly there to make the
orchestration, JSON handling, and distribution less painful.

## Install

```sh
git clone git@git.wearmed.xyz:suraj/vur.git
cd vur
./install.sh
```

It'll check you're actually on Void, grab `go`/`git`/`curl`/`fakeroot` if
you're missing any of them, build the binary, and symlink it into
`~/.local/bin` (pass `--system` for `/usr/local/bin` instead). Fine to
re-run whenever.

## Usage

Aliases are short, `vpm`-style: `vur [OPTIONS] [SUBCOMMAND] [<ARGS>]`

```
vur search  (s)  <term>            search the AUR
vur info         <pkg>             show AUR package details
vur install (i)  <pkg> [pkg..]     build and install package(s)
vur remove  (rm) <pkg> [pkg..]     remove package(s)
vur upgrade (up)                   system upgrade + rebuild outdated AUR packages
vur clean   (cl)                   wipe the build cache and local package repo
vur list    (ls)                   list packages vur has installed
```

Flags work anywhere on the line: `--color=<yes|no|auto>`, `--noconfirm`/`-y`,
`--edit` (pop the PKGBUILD open in `$EDITOR` before building), `--devel`
(also rebuild `-git` packages on `upgrade` if upstream moved even though
`pkgver` didn't). If you don't want to type these every time, they've all
got a home in `~/.config/vur/vur.conf` too.

If you type something that isn't an exact package name, `vur install`
just opens a numbered picker over the search results instead of giving up
(`1 3 5-7` works for picking a few at once) — the most popular match gets
the number that's easiest to reach.

```sh
vur i pipes.sh
vur i -y --edit somefuzzyterm
vur up --devel
vur rm pipes.sh
```

## How it works

1. Looks up the package's `PackageBase` on the AUR and clones its git repo.
2. Shows you the PKGBUILD (just a diff against what you last saw, if nothing
   changed) and won't do anything until you confirm — same trust model as
   yay/paru/pikaur, since a PKGBUILD is arbitrary bash and there's no way
   around that. `--edit` if you want to change something first.
3. Works out `depends`/`makedepends` against what's actually in Void's repos,
   using `depmap.conf` to paper over the naming differences from Arch (`gtk2`
   → `gtk+`, that sort of thing). Anything only available on the AUR gets
   built first, recursively, and it'll notice if you've got a dependency cycle.
4. Grabs the sources, checks the hashes, unpacks whatever needs unpacking.
5. Runs `build()` as you, `package()` under `fakeroot`.
6. Hands the result to `xbps-create`, drops it in a little local repo at
   `~/.cache/vur/repo`, and installs it from there. Doesn't touch any of
   your actual system config to do this.

## Known limitations

- Doesn't look at `options=()` (`!strip`, `docs`, etc.) — whatever
  `package()` produces is what you get.
- `.install` scriptlets get shown to you but aren't wired up as xbps hooks.
- Won't guess at systemd-only dependencies, because Void just doesn't have
  them — `depmap.conf` marks those as unresolvable instead of pretending.
- For `-git` packages, `upgrade --devel` will notice upstream moved even if
  `pkgver` stayed the same, but it never runs a PKGBUILD's `pkgver()`, so the
  version string you see can be a bit behind what's actually installed.
- Everything builds one at a time, nothing in parallel yet.

## License

GPLv3, see [LICENSE](LICENSE).
