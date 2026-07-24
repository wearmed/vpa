# shellcheck shell=bash
# Sources a PKGBUILD (the only reliable way to parse it) into PB_* globals.
# Only defines vars/functions here; build()/package() run later, after review.

_pb_vars=(pkgname pkgbase pkgver pkgrel pkgdesc url install
          arch license depends makedepends checkdepends optdepends
          provides conflicts replaces source
          sha256sums sha512sums b2sums md5sums noextract)

pkgbuild_load() {
  local dir=$1
  [[ -f "$dir/PKGBUILD" ]] || die "no PKGBUILD found in $dir"

  local dump
  dump=$(
    cd "$dir" || exit 1
    # shellcheck disable=SC1091  # PKGBUILD is fetched at runtime
    source ./PKGBUILD
    for v in "${_pb_vars[@]}"; do
      declare -p "$v" 2>/dev/null || true
    done
  ) || die "failed to parse PKGBUILD in $dir"

  local v
  for v in "${_pb_vars[@]}"; do
    unset "PB_${v^^}"
  done

  # -g needed: declare via eval inside a function is otherwise function-local
  eval "$(sed -E 's/^declare (\S+) ([a-zA-Z_][a-zA-Z0-9_]*)=/declare -g \1 PB_\U\2\E=/' <<<"$dump")"

  PB_PKGVER=${PB_PKGVER:-}
  PB_PKGREL=${PB_PKGREL:-1}
  PB_PKGDESC=${PB_PKGDESC:-}
  PB_URL=${PB_URL:-}
  PB_INSTALL=${PB_INSTALL:-}
  [[ -v PB_PKGNAME ]] || die "PKGBUILD in $dir defines no pkgname"
  [[ -v PB_PKGBASE ]] || PB_PKGBASE=$PB_PKGNAME
  for v in PB_ARCH PB_LICENSE PB_DEPENDS PB_MAKEDEPENDS PB_CHECKDEPENDS \
           PB_OPTDEPENDS PB_PROVIDES PB_CONFLICTS PB_REPLACES PB_SOURCE \
           PB_SHA256SUMS PB_SHA512SUMS PB_B2SUMS PB_MD5SUMS PB_NOEXTRACT; do
    [[ -v $v ]] || eval "$v=()"
  done
}

# pkgbuild_names -> the list of pkgnames this PKGBUILD produces (usually one)
pkgbuild_names() {
  if [[ "$(declare -p PB_PKGNAME)" == "declare -a"* ]]; then
    printf '%s\n' "${PB_PKGNAME[@]}"
  else
    printf '%s\n' "$PB_PKGNAME"
  fi
}

# strdep <depstring> -> pkgname only, stripping version constraints (>=, <=, =)
strdep() {
  local d=$1
  d=${d%%[<>=]*}
  printf '%s' "$d"
}

# depver <depstring> -> version-constraint suffix (e.g. ">=4.0.0"), or empty
depver() {
  local d=$1 bare
  bare=$(strdep "$d")
  printf '%s' "${d#"$bare"}"
}

# optdep_name <opt> -> the pkgname part of an optdepends entry like "foo: needed for bar"
optdep_name() {
  local d=$1
  printf '%s' "${d%%:*}"
}

# pkgbuild_is_devel -- true if PB_SOURCE has a git+ entry (VCS package)
pkgbuild_is_devel() {
  local s
  for s in "${PB_SOURCE[@]}"; do
    [[ "$s" == *"git+"* ]] && return 0
  done
  return 1
}

# pkgbuild_devel_latest_commit -- HEAD of the first git+ source's remote, via ls-remote
pkgbuild_devel_latest_commit() {
  local s url real base
  for s in "${PB_SOURCE[@]}"; do
    case "$s" in
      *"::"*) url=${s#*::} ;;
      *) url=$s ;;
    esac
    [[ "$url" == git+* ]] || continue
    real=${url#git+}
    base=${real%%#*}
    git ls-remote "$base" HEAD 2>/dev/null | awk '{print $1; exit}'
    return 0
  done
}
