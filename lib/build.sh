# shellcheck shell=bash
# Fetches sources, verifies checksums, runs build()/package(). Requires PB_*
# already loaded via pkgbuild_load.
#
# Layout: $BUILD_DIR/<pkgbase>/{git,src,pkg/<name>}

_is_noextract() {
  local fname=$1 n
  for n in "${PB_NOEXTRACT[@]}"; do [[ "$n" == "$fname" ]] && return 0; done
  return 1
}

_maybe_extract() {
  local fname=$1 dir=$2
  _is_noextract "$fname" && return 0
  case "$fname" in
    *.tar|*.tar.gz|*.tgz|*.tar.bz2|*.tbz2|*.tar.xz|*.txz|*.tar.zst|*.tar.lz)
      info "extracting $fname"
      tar -xf "$dir/$fname" -C "$dir" || die "failed to extract $fname"
      ;;
    *.zip)
      require_bin unzip unzip
      info "extracting $fname"
      unzip -qo "$dir/$fname" -d "$dir" || die "failed to extract $fname"
      ;;
    *) : ;;
  esac
}

_check_sum() {
  local tool=$1 expect=$2 file=$3 got
  got=$("$tool" "$file" | awk '{print $1}')
  [[ "$got" == "$expect" ]] || die "$file: $tool mismatch (expected $expect, got $got)"
}

# _verify_checksum <index> <fname> <dir> -- strongest non-SKIP sum wins
_verify_checksum() {
  local idx=$1 fname=$2 dir=$3 val

  val=${PB_SHA512SUMS[$idx]:-}
  if [[ -n "$val" && "$val" != "SKIP" ]]; then
    require_bin sha512sum coreutils
    _check_sum sha512sum "$val" "$dir/$fname"
    return 0
  fi
  val=${PB_SHA256SUMS[$idx]:-}
  if [[ -n "$val" && "$val" != "SKIP" ]]; then
    require_bin sha256sum coreutils
    _check_sum sha256sum "$val" "$dir/$fname"
    return 0
  fi
  val=${PB_B2SUMS[$idx]:-}
  if [[ -n "$val" && "$val" != "SKIP" ]]; then
    require_bin b2sum coreutils
    _check_sum b2sum "$val" "$dir/$fname"
    return 0
  fi
  val=${PB_MD5SUMS[$idx]:-}
  if [[ -n "$val" && "$val" != "SKIP" ]]; then
    require_bin md5sum coreutils
    _check_sum md5sum "$val" "$dir/$fname"
    return 0
  fi
  warn "$fname: no checksum available to verify (all SKIP/missing) -- trusting download as-is"
}

# _fetch_git_source <git+url> <destname> <srcdir>
_fetch_git_source() {
  local url=$1 fname=$2 srcdir=$3 real base frag ref
  real=${url#git+}
  base=${real%%#*}
  frag=""
  [[ "$real" == *#* ]] && frag=${real#*#}
  require_bin git git
  info "cloning $fname"
  rm -rf "${srcdir:?}/${fname:?}"
  git clone --quiet "$base" "$srcdir/$fname" || die "git clone failed for $fname ($base)"
  if [[ -n "$frag" ]]; then
    ref=${frag#*=}
    git -C "$srcdir/$fname" checkout --quiet "$ref" || die "git checkout '$ref' failed for $fname"
  fi
}

# fetch_sources <pkgbase> -- populates a fresh srcdir from PB_SOURCE.
fetch_sources() {
  local pkgbase=$1
  local gitdir="$BUILD_DIR/$pkgbase/git" srcdir="$BUILD_DIR/$pkgbase/src"
  rm -rf "$srcdir"
  mkdir -p "$srcdir"

  local i entry fname url
  for i in "${!PB_SOURCE[@]}"; do
    entry=${PB_SOURCE[$i]}
    [[ -z "$entry" ]] && continue
    if [[ "$entry" == *"::"* ]]; then
      fname=${entry%%::*}
      url=${entry#*::}
    else
      url=$entry
      case "$url" in
        git+*|*://*) fname=$(basename "${url%%#*}") ;;
        *) fname=$url ;;
      esac
    fi

    case "$url" in
      git+*)
        _fetch_git_source "$url" "$fname" "$srcdir"
        ;;
      *://*)
        info "downloading $fname"
        curl -fL --retry 3 --connect-timeout 15 -o "$srcdir/$fname" "$url" \
          || die "failed to download $fname"
        _verify_checksum "$i" "$fname" "$srcdir"
        _maybe_extract "$fname" "$srcdir"
        ;;
      *)
        [[ -f "$gitdir/$url" ]] || die "$pkgbase: local source '$url' not found in checkout"
        cp -- "$gitdir/$url" "$srcdir/$fname"
        _verify_checksum "$i" "$fname" "$srcdir"
        _maybe_extract "$fname" "$srcdir"
        ;;
    esac
  done
}

# run_build <pkgbase> -- runs prepare()/build() as the normal user.
# PKGBUILD values pass through env, never interpolated shell text.
run_build() {
  local pkgbase=$1
  local gitdir="$BUILD_DIR/$pkgbase/git" srcdir="$BUILD_DIR/$pkgbase/src"
  [[ -d "$srcdir" ]] || die "run_build: no srcdir for $pkgbase (run fetch_sources first)"
  info "building $pkgbase"
  env -i HOME="$HOME" PATH="$PATH" TERM="${TERM:-dumb}" LANG="${LANG:-C.UTF-8}" \
      startdir="$gitdir" srcdir="$srcdir" pkgbase="$PB_PKGBASE" \
      pkgver="$PB_PKGVER" pkgrel="$PB_PKGREL" CARCH="$(void_arch)" \
      bash "$ROOT/lib/driver-build.sh" \
    || die "build() failed for $pkgbase"
}

# run_package <pkgbase> -- runs package()/package_<name>() under fakeroot, per pkgname
run_package() {
  local pkgbase=$1
  local gitdir="$BUILD_DIR/$pkgbase/git" srcdir="$BUILD_DIR/$pkgbase/src"
  require_bin fakeroot fakeroot
  local name pkgdir
  while IFS= read -r name; do
    pkgdir="$BUILD_DIR/$pkgbase/pkg/$name"
    rm -rf "$pkgdir"
    mkdir -p "$pkgdir"
    info "packaging $name"
    fakeroot -- env -i HOME="$HOME" PATH="$PATH" TERM="${TERM:-dumb}" LANG="${LANG:-C.UTF-8}" \
        startdir="$gitdir" srcdir="$srcdir" pkgdir="$pkgdir" \
        pkgbase="$PB_PKGBASE" pkgname="$name" pkgver="$PB_PKGVER" pkgrel="$PB_PKGREL" \
        CARCH="$(void_arch)" bash "$ROOT/lib/driver-package.sh" \
      || die "package() failed for $name"
  done < <(pkgbuild_names)
}

# built_vcs_commit <pkgbase> -- HEAD of the built git+ source, for --devel. Empty if not VCS.
built_vcs_commit() {
  local pkgbase=$1
  local srcdir="$BUILD_DIR/$pkgbase/src" s url fname
  for s in "${PB_SOURCE[@]}"; do
    case "$s" in
      *"::"*) fname=${s%%::*}; url=${s#*::} ;;
      *) url=$s; fname=$(basename "${url%%#*}") ;;
    esac
    [[ "$url" == git+* ]] || continue
    if [[ -d "$srcdir/$fname/.git" ]]; then
      git -C "$srcdir/$fname" rev-parse HEAD 2>/dev/null && return 0
    fi
  done
}
