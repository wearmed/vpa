# shellcheck shell=bash
# Parses a PKGBUILD by actually sourcing it (the only reliable way, since it's bash)
# and exports its variables into PB_* globals for the rest of vur to use.
#
# This only *sources* the file (defines vars/functions), it never calls
# prepare()/build()/package() here -- that happens later, after the user has
# had a chance to review the PKGBUILD. Sourcing arbitrary AUR bash is still an
# inherent trust decision (the same one every AUR helper makes); vur always
# shows the PKGBUILD and asks for confirmation before this point.

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
    # PKGBUILDs may call build()/package()/prepare() at parse time only if
    # they misbehave; we don't invoke them ourselves here.
    # shellcheck disable=SC1091  # PKGBUILD is fetched at runtime; nothing to statically follow
    source ./PKGBUILD
    for v in "${_pb_vars[@]}"; do
      declare -p "$v" 2>/dev/null || true
    done
  ) || die "failed to parse PKGBUILD in $dir"

  # Clear any PB_* left over from a previous pkgbuild_load call in this shell.
  local v
  for v in "${_pb_vars[@]}"; do
    unset "PB_${v^^}"
  done

  # NOTE: declare (even via eval) inside a function is function-local unless
  # -g is passed, so PB_* would vanish the instant pkgbuild_load returns
  # without it.
  eval "$(sed -E 's/^declare (\S+) ([a-zA-Z_][a-zA-Z0-9_]*)=/declare -g \1 PB_\U\2\E=/' <<<"$dump")"

  # Normalize scalars that might be missing into empty-but-defined values.
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

# depver <depstring> -> the version-constraint suffix (e.g. ">=4.0.0"), or
# empty if the depstring had no operator. xbps uses the same >=/<=/=/</>
# pkgpattern syntax as Arch's PKGBUILDs, so this suffix carries over verbatim.
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

# pkgbuild_is_devel -- true if PB_SOURCE has a git+ entry (a VCS/-git-style
# package), the case --devel exists to handle: pkgver may not change between
# builds even though upstream has new commits.
pkgbuild_is_devel() {
  local s
  for s in "${PB_SOURCE[@]}"; do
    [[ "$s" == *"git+"* ]] && return 0
  done
  return 1
}

# pkgbuild_devel_latest_commit -- HEAD commit of the first git+ source's
# remote, via a cheap `git ls-remote` (no clone needed). Empty if not devel.
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
