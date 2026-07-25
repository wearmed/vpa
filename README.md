# vur

**V**oid **U**ser **R**epository: an AUR helper for Void Linux.

Fetches PKGBUILDs from the [AUR](https://aur.archlinux.org), builds them, and
packages the result as a real `.xbps` via `xbps-create` — tracked and cleanly
removable through `xbps`, not just dumped onto the filesystem. Written in Go,
ships as a single static binary (no `jq`, no shell library to keep in sync).
PKGBUILDs are themselves bash, so building still shells out to `bash`/`git`/
`fakeroot`/`xbps-*` — Go just handles orchestration, JSON, and packaging.

## Install

```sh
curl -fsSL https://git.wearmed.xyz/suraj/vur/raw/branch/main/install.sh | bash
```

Checks you're on Void, installs `go`/`git`/`curl`/`fakeroot` if missing,
builds a static binary, and symlinks it into `~/.local/bin` (`--system` for
`/usr/local/bin`). Safe to re-run.

## Usage

```
vur search  (s)  <term>            search the AUR
vur info         <pkg>             show AUR package details
vur install (i)  <pkg> [pkg..]     build and install package(s)
vur remove  (rm) <pkg> [pkg..]     remove package(s)
vur upgrade (up)                   system upgrade + rebuild outdated AUR packages
vur clean   (cl)                   wipe the build cache and local package repo
vur list    (ls)                   list packages vur has installed
```

Flags (anywhere on the command line): `--color=<yes|no|auto>`, `--noconfirm`/`-y`,
`--edit` (open PKGBUILD in `$EDITOR` first), `--devel` (also rebuild `-git`
packages on `upgrade` when upstream moved but `pkgver` didn't). All persist
in `~/.config/vur/vur.conf`.

`vur install <term>` with no exact match opens a numbered picker over the
search results (e.g. `1 3 5-7`), most popular result gets the highest number.

```sh
vur i pipes.sh
vur i -y --edit somefuzzyterm
vur up --devel
vur rm pipes.sh
```

## How it works

1. Resolves the package's `PackageBase` via the AUR RPC and clones its git repo.
2. Shows the full `PKGBUILD` (diffed against the last reviewed copy if
   unchanged) and asks for confirmation before ever sourcing it — same trust
   model as yay/paru/pikaur. `--edit` lets you change it first.
3. Resolves `depends`/`makedepends` against Void's xbps repos, using
   `depmap.conf` to bridge Arch/Void naming drift (e.g. `gtk2` → `gtk+`).
   Anything AUR-only gets built recursively, with cycle detection.
4. Downloads sources, verifies checksums, extracts archives.
5. Runs `build()` as your user, `package()` under `fakeroot`.
6. Packages with `xbps-create`, indexes into `~/.cache/vur/repo`, installs
   via `xbps-install --repository=...` — no system config touched.

## Known limitations

- `options=()` flags (`!strip`, `docs`, etc.) are ignored.
- `.install` scriptlets are shown for review but not converted to xbps hooks.
- No systemd-only dependency substitutes — Void has none, so `depmap.conf`
  marks them unresolvable rather than guessing.
- `-git` packages: `upgrade --devel` catches upstream commits moving, but a
  PKGBUILD's dynamic `pkgver()` is never invoked, so the version string can lag.
- Builds run serially.

## Credits

The whole project was inspired by [yay](https://github.com/Jguer/yay).
vur is basically trying to replicate it for Void Linux, rather than Arch.

## License

GPLv3, see [LICENSE](LICENSE).
