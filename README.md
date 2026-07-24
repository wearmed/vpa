# vur

**V**oid **U**ser **R**epository — an AUR helper for Void Linux.

`vur` fetches PKGBUILDs from the [AUR](https://aur.archlinux.org), builds them
with a minimal makepkg-like driver, and packages the result as a real
`.xbps` package via `xbps-create` — so it's tracked and cleanly removable
through `xbps` like any native Void package, not just dumped onto the
filesystem with `make install`.

## Install

```sh
git clone git@git.wearmed.xyz:suraj/vur.git
ln -s "$(pwd)/vur/vur" ~/.local/bin/vur   # make sure ~/.local/bin is on PATH
```

`vur` will offer to install its own runtime dependencies (`jq`, `fakeroot`,
`git`, `curl`, `xbps-create`) via `sudo xbps-install` the first time it needs
one that's missing.

## Usage

```
vur search <term>          search the AUR
vur info <pkg>             show AUR package details
vur install <pkg> [pkg..]  build and install package(s) from the AUR
vur remove <pkg> [pkg..]   remove installed package(s) (via xbps-remove)
vur upgrade                rebuild any vur-tracked package with a newer AUR version
vur list                   list packages vur has installed
```

Example:

```sh
vur search pipes
vur info pipes.sh
vur install pipes.sh
vur list
vur remove pipes.sh
```

## How it works

1. Resolves the requested package's `PackageBase` via the AUR RPC and clones
   its git repo (`aur.archlinux.org/<pkgbase>.git`).
2. Shows you the full `PKGBUILD` (and any `.install` scriptlet) and asks for
   confirmation **before ever sourcing it** — the same trust model every AUR
   helper (yay, paru, pikaur) uses, since a PKGBUILD is arbitrary bash.
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
- `-git` packages fetch via `git+` sources, but a PKGBUILD's dynamic
  `pkgver()` (re-derived from `git describe` at build time) is never
  invoked, so version numbers may be stale.
- Dependency version constraints are not deeply validated beyond
  presence/absence when resolving against Void repos.
- Builds run strictly serially.

## License

GPLv3, see [LICENSE](LICENSE).
