# vpa

**V**oid **P**ackage **A**ssistant — the only Void package manager you'll need.

One tool for everything you install on Void: your normal `xbps` packages, the
[AUR](https://aur.archlinux.org), standalone `.xbps` files, and even prebuilt
Debian/RPM/Arch packages. Anything it builds or imports becomes a real `.xbps`
package, so it's tracked and cleanly removable through `xbps` like everything
else on your system — never just files dumped onto your filesystem.

Written in Go, ships as a single static binary (no `jq`, no shell library to
keep in sync). PKGBUILDs are themselves bash, so building them still shells
out to `bash`/`git`/`fakeroot`/`xbps-*` — Go handles orchestration, JSON,
concurrency, and packaging.

## Install

```sh
curl -fsSL https://git.wearmed.xyz/suraj/vpa/raw/branch/main/install.sh | bash
```

Checks you're on Void, installs `go`/`git`/`curl`/`fakeroot` if missing,
builds a static binary, and symlinks it into `~/.local/bin` (`--system` for
`/usr/local/bin`). Safe to re-run.

## Usage

```
INSTALLING AND REMOVING
vpa install (i)  <pkg>...        install from anywhere
vpa devinstall (di) <pkg>...     install packages plus their -devel parts
vpa remove  (rm) <pkg>...        remove
vpa removerecursive (rr) <pkg>   remove, plus unneeded dependencies
vpa update  (up)                 update vpa, your system, and AUR packages
vpa sync    (sy)                 refresh repository data only

FINDING THINGS
vpa search  (s)  <term>          search Void's repos + the AUR
vpa info         <pkg>           details from Void's repos and/or AUR
vpa list    (ls) [--aur]         list installed packages
vpa filelist (fl) <pkg>          files a package installs
vpa whatprovides (wp) <file>     which package a file came from
vpa searchfile (sf) <file>       find installed packages containing a file
vpa deps         <pkg>           what a package needs
vpa reverse (rv) <pkg>           what depends on a package

LOOKING AFTER YOUR SYSTEM
vpa orphans                      no-longer-needed dependencies
vpa autoremove (ar)              remove those
vpa cleanup (cl)                 free up disk space
vpa reconfigure (rc) <pkg|all>   re-run a package's setup step
vpa hold / unhold <pkg>...       stop packages being updated

REPOSITORIES AND ALTERNATIVES
vpa listrepos (lr)               show configured repositories
vpa addrepo <url>                add another repository
vpa listalternatives (la)        show configurable defaults
vpa setalternative (sa) <pkg>    choose which package provides one
```

Command names and aliases follow [`vpm`](https://github.com/netzverweigerer/vpm),
the xbps front-end many Void users already know.

`vpa` on its own gives you a short overview of the handful of commands you
actually need day to day; `vpa help --all` lists everything. Each command
explains itself too — `vpa help install`, `vpa install --help`, or just
`vpa install` with nothing after it.

### What `vpa install` accepts

- a package already in Void's repos → installs it directly, no AUR involved
- an AUR package name → reviews the PKGBUILD, builds it, installs it
- a search term with no exact match → numbered picker (`1 3 5-7`)
- a `.xbps` file or URL → installs it directly
- a `.deb`/`.rpm`/`.pkg.tar.zst` file or URL → extracts and repackages it as
  a real `.xbps` (packaging only — it can't fix ABI differences between
  distros, so whether a foreign binary actually runs depends on how
  compatible it happens to be with Void's glibc)

### Searching and listing

`vpa search` covers Void's repositories and the AUR in one pass, listing
Void results first (a native package is nearly always the better choice when
one exists) and marking anything already installed. `vpa info` works the
same way, showing a package from whichever side it exists on — or both.

`vpa list` shows every installed package on the system, tagging the ones vpa
built from the AUR; `vpa list --aur` narrows it to just those.

### `vpa update`

Updates everything in one go: vpa itself (if a newer version exists), a full
system upgrade, then any AUR package vpa tracks.

### Asking before it does things

vpa assumes yes and gets on with it. `vpa --assumeno` makes it ask first,
`vpa --assumeyes` switches back — the setting is saved, so you only do it
once. `--confirm` and `--noconfirm`/`-y` override it for a single run.

One thing always asks regardless: an AUR build script vpa hasn't shown you
before. That's a stranger's code about to run on your machine, so it isn't
treated as a routine confirmation. Passing `--noconfirm` explicitly skips it
for scripting, or set `TRUST_AUR=1` in the config if you never want to be
asked.

### Flags

Anywhere on the command line: `--color=<yes|no|auto>`, `--noconfirm`/`-y`,
`--edit` (open PKGBUILD in `$EDITOR` first), `--devel` (also rebuild `-git`
packages when upstream moved but `pkgver` didn't), `--parallel=<N>`
(concurrent source downloads, default 4). All persist in
`~/.config/vpa/vpa.conf` (`PARALLEL_DOWNLOADS=N` for the last one).

```sh
vpa i firefox              # straight from Void's repos
vpa i pipes.sh             # from the AUR
vpa i ./something.deb      # repackaged as .xbps
vpa update                 # update everything
vpa wp /usr/bin/brave-origin
```

## How it works

For a Void repo package or a `.xbps` file, vpa just drives `xbps` directly.
For an AUR package:

1. Resolves the `PackageBase` via the AUR RPC and clones its git repo.
2. Shows a plain summary of what it is, where the code is downloaded from
   and what it pulls in, then asks before running any of it — the full
   build script is one keypress away, and re-installing something whose
   script changed shows you a diff. Same trust model as yay/paru/pikaur:
   an AUR package is a stranger's script, and building it runs their code.
3. Resolves `depends`/`makedepends` against Void's repos, using `depmap.conf`
   to bridge Arch/Void naming drift (e.g. `gtk2` → `gtk+`). If a dependency
   has no Void package by that name, before giving up vpa checks what shared
   libraries the real Arch package ships against what Void packages actually
   provide (e.g. Arch's `libxss` → Void's `libXScrnSaver`, both shipping
   `libXss.so.1`) — matching on real ABI instead of guessing from spelling.
   Anything still unresolved and AUR-only gets built recursively, with cycle
   detection.
4. Downloads sources (concurrently, cached by checksum so rebuilds skip
   unchanged ones), verifies checksums, extracts archives.
5. Runs `build()` as your user (with `MAKEFLAGS`/`CARGO_BUILD_JOBS` set to
   your core count), `package()` under `fakeroot`.
6. Packages with `xbps-create`, indexes into `~/.cache/vpa/repo`, installs
   via `xbps-install --repository=...` — no system config touched. Multiple
   packages build tier-by-tier: independent packages in a tier build in
   parallel, and each tier is installed before the next tier's builds start,
   so a package needing an earlier AUR dependency at build time finds it.

## Known limitations

- `options=()` flags (`!strip`, `docs`, etc.) are ignored.
- `.install` scriptlets are shown for review but not converted to xbps hooks.
- No systemd-only dependency substitutes — Void has none, so `depmap.conf`
  marks them unresolvable rather than guessing.
- `-git` packages: `update --devel` catches upstream commits moving, but a
  PKGBUILD's dynamic `pkgver()` is never invoked, so the version string can lag.
- Imported `.deb`/RPM/Arch packages carry no usable dependency information —
  their declared deps are printed as a warning, not installed.
- Shared-library dependency matching only works for actual libraries —
  virtual/meta names with no `.so` file (e.g. `ttf-font`) stay unresolved.

## Credits

The AUR-facing parts of vpa were inspired by [yay](https://github.com/Jguer/yay)
— the numbered install picker, reviewing a PKGBUILD before building it,
`--devel` rebuilds. Inspiration only: no yay code was copied or adapted, and
none of vpa's other functionality derives from it. The command structure
takes cues from [vpm](https://github.com/netzverweigerer/vpm).

## License

GPLv3 — see [LICENSE](LICENSE).

Free software: use it, modify it, share it. If you distribute a modified
version, it has to stay GPLv3 and ship its source too, so vpa can never be
turned into someone else's closed-source product.

---

used claude to make this readme, i aint typing allat