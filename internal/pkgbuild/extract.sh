#!/usr/bin/env bash
# Sources a PKGBUILD (the only reliable way to parse it) and emits its
# variables as NUL-delimited records vpa can parse without touching bash's
# declare -p quoting rules: "S\tname\tvalue\0" for scalars, "A\tname\0"
# followed by "I\tvalue\0" per element for arrays, "X\tname\0" + "I\t...\0"
# for an arch-specific override array (e.g. source_x86_64) whose items get
# appended to the base array of the same name rather than replacing it --
# matching makepkg's own arch-override semantics.
set -e
cd "$1"
arch=${2:-}
# shellcheck disable=SC1091
source ./PKGBUILD

emit_scalar() {
  printf 'S\t%s\t%s\0' "$1" "${!1-}"
}

emit_array() {
  local name=$1
  declare -p "$name" &>/dev/null || return 0
  printf 'A\t%s\0' "$name"
  local -n ref="$name"
  local v
  for v in "${ref[@]}"; do
    printf 'I\t%s\0' "$v"
  done
}

emit_arch_override() {
  local base=$1 archvar="${1}_${arch}"
  [[ -n "$arch" ]] || return 0
  declare -p "$archvar" &>/dev/null || return 0
  printf 'X\t%s\0' "$base"
  local -n ref="$archvar"
  local v
  for v in "${ref[@]}"; do
    printf 'I\t%s\0' "$v"
  done
}

# shellcheck disable=SC2154  # pkgname comes from the sourced PKGBUILD above
if declare -p pkgname 2>/dev/null | grep -q '^declare -a'; then
  emit_array pkgname
else
  emit_scalar pkgname
fi

for v in pkgbase pkgver pkgrel pkgdesc url install; do
  emit_scalar "$v"
done

for v in arch license depends makedepends checkdepends optdepends provides conflicts replaces source sha256sums sha512sums b2sums md5sums noextract; do
  emit_array "$v"
  emit_arch_override "$v"
done
