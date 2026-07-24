# vur

**V**oid **U**ser **R**epository — an AUR helper for Void Linux.

`vur` fetches PKGBUILDs from the [AUR](https://aur.archlinux.org), builds them
with a minimal makepkg-like driver, and packages the result as a real
`.xbps` package via `xbps-create` — so it's tracked and cleanly removable
through `xbps` like any native Void package, not just dumped onto the
filesystem with `make install`.

## Install

Same shape as bringing up `yay` for the first time on Arch
(`git clone ... && cd yay && makepkg -si`):

```sh
git clone git@git.wearmed.xyz:suraj/vur.git
cd vur
./install.sh
```

`install.sh` checks you're on Void, installs `git`/`curl`/`jq`/`fakeroot` if
missing, symlinks `vur` into `~/.local/bin` (or `/usr/local/bin` with
`./install.sh --system`), and writes a starter config to
`~/.config/vur/vur.conf`. Safe to re-run.

## Usage

```
vur [flags] search <term>          search the AUR
vur [flags] info <pkg>             show AUR package details
vur [flags] install <pkg> [pkg..]  build and install package(s) from the AUR
vur [flags] remove <pkg> [pkg..]   remove installed package(s) (via xbps-remove)
vur [flags] upgrade                offer a full system upgrade, then rebuild any
                                    vur-tracked package with a newer AUR version
vur [flags] clean                  wipe the build cache and local package repo
vur [flags] list                   list packages vur has installed
```

Flags (may appear anywhere in the command line):

| Flag                | Effect                                                                 |
|----------------------|------------------------------------------------------------------------|
| `--noconfirm`, `-y`  | never prompt for confirmation (for scripting)                          |
| `--edit`             | open each PKGBUILD in `$EDITOR` before building                        |
| `--devel`            | on `upgrade`, also rebuild `-git`/`-svn`/`-hg` packages whose upstream commit moved even if `pkgver` didn't change |

Persistent versions of the same knobs live in `~/.config/vur/vur.conf`
(`NOCONFIRM=1`, `EDITOR=nvim`, `CLEAN_AFTER=1` to drop each package's build
directory right after a successful install).

`vur install <term>` doesn't require an exact package name: if there's no
exact AUR match, it opens a `yay`-style numbered picker over the search
results (accepts space-separated indices and ranges, e.g. `1 3 5-7`), sorted
so the most popular result gets the easiest-to-reach (highest) number.

Example:

```sh
vur search pipes
vur info pipes.sh
vur install pipes.sh
vur install --noconfirm --edit somefuzzyterm   # interactive picker + PKGBUILD editing
vur upgrade --devel
vur list
vur remove pipes.sh
```

## How it works

1. Resolves the requested package's `PackageBase` via the AUR RPC and clones
   its git repo (`aur.archlinux.org/<pkgbase>.git`).
2. Shows you the full `PKGBUILD` (and any `.install` scriptlet) and asks for
   confirmation **before ever sourcing it** — the same trust model every AUR
   helper (yay, paru, pikaur) uses, since a PKGBUILD is arbitrary bash. On a
   package you've already reviewed before, it shows a diff against the last
   reviewed copy instead of the full text again (or says so plainly if
   nothing changed). `--edit` opens it in `$EDITOR` first if you want to
   change anything.
3. Resolves `depends`/`makedepends` against Void's xbps repos, using
   `config/depmap.conf` to bridge Arch/Void package-name drift (e.g. `gtk2`
   → `gtk+`). Anything not in Void's repos but valid on the AUR is built
   recursively, in dependency order, with cycle detection.
4. Downloads sources (`source=()`), verifying `sha256sums`/`sha512sums`/
   `b2sums`/`md5sums` and auto-extracting recognized archives.
5. Runs `prepare()`/`build()` as your normal user, then
   `package()`/`package_<name>()` under `fakeroot`, in a minimal sandboxed
   environment — untrusted PKGBUILD values are only ever passed across via
   the process environment, never interpolated into shell text.
6. Packages the result with `xbps-create`, indexes it into a local repo at
   `~/.cache/vur/repo`, and installs via `xbps-install --repository=...` —
   no system-wide `/etc/xbps.d/` changes, no root config touched.

## Known limitations

- `options=()` flags (`!strip`, `docs`, `staticlibs`, etc.) are ignored.
- `.install` scriptlets are shown for review but not translated into xbps
  `INSTALL`/`REMOVE` hooks.
- No name mapping guesses for systemd-only dependencies — Void uses runit,
  and `depmap.conf` marks those explicitly unresolvable rather than
  silently substituting something non-equivalent.
- `-git` packages fetch via `git+` sources, and `upgrade --devel` will
  notice and rebuild when the upstream commit has moved even though
  `pkgver` didn't change, but a PKGBUILD's dynamic `pkgver()` (re-derived
  from `git describe` at build time) is never invoked, so the version
  *string* shown by `vur list` may still lag behind what's actually built.
- Dependency version constraints are not deeply validated beyond
  presence/absence when resolving against Void repos.
- Builds run strictly serially.

## License

GPLv3, see [LICENSE](LICENSE).
