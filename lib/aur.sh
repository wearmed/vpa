# shellcheck shell=bash
# shellcheck disable=SC2154  # c_bold/c_reset etc. come from lib/common.sh, sourced alongside this file by `vur`
# Talks to the AUR RPC (https://aur.archlinux.org/rpc/v5) and clones AUR git repos.

aur_rpc_search() {
  local term=$1
  curl -fsSL --get \
    --data-urlencode "arg=$term" \
    --data-urlencode "by=name-desc" \
    "$AUR_RPC/search"
}

# aur_rpc_info <name> [name...]  -- batched multiinfo lookup
aur_rpc_info() {
  local args=() n
  for n in "$@"; do args+=(--data-urlencode "arg[]=$n"); done
  curl -fsSL --get "${args[@]}" "$AUR_RPC/info"
}

# aur_pkgbase <name> -> prints PackageBase for a given AUR package name, or empty if not found.
aur_pkgbase() {
  local name=$1 json
  json=$(aur_rpc_info "$name") || return 1
  jq -r --arg n "$name" '.results[] | select(.Name == $n) | .PackageBase' <<<"$json"
}

# aur_clone <pkgbase> <destdir>
aur_clone() {
  local pkgbase=$1 dest=$2
  if [[ -d "$dest/.git" ]]; then
    info "updating existing checkout of $pkgbase"
    git -C "$dest" fetch --quiet origin
    git -C "$dest" reset --hard --quiet origin/HEAD
  else
    rm -rf "$dest"
    git clone --quiet "$AUR_GIT/$pkgbase.git" "$dest"
  fi
}

cmd_search() {
  local term=${1:-}
  [[ -n "$term" ]] || die "usage: vur search <term>"
  require_bin jq jq
  require_bin curl curl

  local json count
  json=$(aur_rpc_search "$term") || die "AUR search request failed"
  count=$(jq -r '.resultcount // 0' <<<"$json")
  if [[ "$count" -eq 0 ]]; then
    info "no AUR results for '$term'"
    return 0
  fi

  jq -r '.results | sort_by(.Name)[] | [.Name, .Version, (.Description // "")] | @tsv' <<<"$json" |
  while IFS=$'\t' read -r name ver desc; do
    printf '%saur/%s %s%s\n    %s\n' "$c_bold" "$name" "$ver" "$c_reset" "$desc"
  done
}

cmd_info() {
  local pkg=${1:-}
  [[ -n "$pkg" ]] || die "usage: vur info <pkg>"
  require_bin jq jq
  require_bin curl curl

  local json n
  json=$(aur_rpc_info "$pkg") || die "AUR info request failed"
  n=$(jq -r '.resultcount // 0' <<<"$json")
  [[ "$n" -gt 0 ]] || die "package '$pkg' not found in AUR"

  jq -r '.results[0] |
    "Name           : \(.Name)",
    "PackageBase    : \(.PackageBase)",
    "Version        : \(.Version)",
    "Description    : \(.Description // "-")",
    "URL            : \(.URL // "-")",
    "License        : \((.License // []) | join(", "))",
    "Depends        : \((.Depends // []) | join(", "))",
    "MakeDepends    : \((.MakeDepends // []) | join(", "))",
    "OptDepends     : \((.OptDepends // []) | join(", "))",
    "Maintainer     : \(.Maintainer // "-")",
    "Votes          : \(.NumVotes)",
    "Popularity     : \(.Popularity)",
    "Last Modified  : \(.LastModified | todate)"
  ' <<<"$json"
}
