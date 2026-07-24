# shellcheck shell=bash
# Wraps xbps-create/xbps-rindex/xbps-install so a built pkgdir becomes a real,
# xbps-tracked package. Never touches /etc/xbps.d/ -- the local build repo is
# passed via --repository on every call instead, so vur leaves zero system
# config residue and can be uninstalled by just deleting its directories.

# create_xbps_pkg <pkgname> <pkgver> <pkgrel> <pkgdir> <deps> <desc> <url> <license>
create_xbps_pkg() {
  local pkgname=$1 pkgver=$2 pkgrel=$3 pkgdir=$4 deps=$5 desc=$6 url=$7 license=$8
  require_bin xbps-create xbps

  [[ -n "$pkgname" ]] || die "create_xbps_pkg: empty pkgname (broken PKGBUILD?)"
  [[ -n "$pkgver" ]] || die "$pkgname: empty pkgver -- likely a -git/-svn/-hg PKGBUILD whose pkgver() vur doesn't invoke (see README limitations); set pkgver= manually with --edit"
  [[ -d "$pkgdir" ]] || die "$pkgname: pkgdir $pkgdir doesn't exist -- package() didn't run"
  if [[ -z "$(find "$pkgdir" -mindepth 1 -print -quit 2>/dev/null)" ]]; then
    die "$pkgname: package() produced an empty directory -- nothing to package. Check the PKGBUILD's package() function (--edit to inspect it)"
  fi

  local arch; arch=$(void_arch)
  local args=(-A "$arch" -n "${pkgname}-${pkgver}_${pkgrel}" -s "${desc:-$pkgname}")
  [[ -n "$deps" ]]    && args+=(-D "$deps")
  [[ -n "$url" ]]     && args+=(-H "$url")
  [[ -n "$license" ]] && args+=(-l "$license")
  ( cd "$REPO_DIR" && xbps-create "${args[@]}" "$pkgdir" ) || die "xbps-create failed for $pkgname"

  local outfile="$REPO_DIR/${pkgname}-${pkgver}_${pkgrel}.${arch}.xbps"
  [[ -f "$outfile" ]] || die "xbps-create reported success for $pkgname but $outfile is missing"
  ok "built ${pkgname}-${pkgver}_${pkgrel}.${arch}.xbps"
}

# index_repo -- (re)builds the local repodata index over everything in $REPO_DIR.
index_repo() {
  require_bin xbps-rindex xbps
  shopt -s nullglob
  local pkgs=("$REPO_DIR"/*.xbps)
  shopt -u nullglob
  if [[ ${#pkgs[@]} -eq 0 ]]; then
    warn "no packages in $REPO_DIR to index"
    return 0
  fi
  xbps-rindex -fa "${pkgs[@]}" || die "xbps-rindex failed"
}

# install_from_local <pkgname> [pkgname...]
install_from_local() {
  local pkgs=("$@")
  [[ ${#pkgs[@]} -gt 0 ]] || return 0
  sudo xbps-install --repository="$REPO_DIR" -Sy "${pkgs[@]}" || die "xbps-install failed for: ${pkgs[*]}"
}
