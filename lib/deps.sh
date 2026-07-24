# shellcheck shell=bash
# Dependency classification/resolution: Void xbps repos first, AUR fallback,
# config/depmap.conf bridges Arch/Void package-name drift.

# Parsed once into memory instead of grepping the depmap files on every
# lookup; DEFAULT_DEPMAP loaded first so USER_DEPMAP entries win, matching
# the previous "check user file first" behavior. Last line for a given name
# wins within either file.
declare -gA _DEPMAP
_DEPMAP_LOADED=0
_depmap_load() {
  [[ "$_DEPMAP_LOADED" == 1 ]] && return 0
  local f line name
  for f in "$DEFAULT_DEPMAP" "$USER_DEPMAP"; do
    [[ -r "$f" ]] || continue
    while IFS= read -r line; do
      [[ -z "$line" || "$line" == \#* ]] && continue
      name=${line%%=*}
      _DEPMAP[$name]=${line#*=}
    done < "$f"
  done
  _DEPMAP_LOADED=1
}

# depmap_lookup <archname> -> mapped name, original if unmapped, "-" if no equivalent
depmap_lookup() {
  local name=$1
  _depmap_load
  printf '%s\n' "${_DEPMAP[$name]:-$name}"
}

# dep_classify <archdepstring> -- sets DEP_CLASS (installed|available|aur|unresolved),
# DEP_RESOLVED_NAME, DEP_REASON, DEP_AUR_BASE. Memoized per raw depstring:
# the same dep is classified twice per pkgbase (resolve_pkgbase, then
# runtime_deps_string), and repeats across pkgbases in the same run.
declare -gA _DEP_CACHE
dep_classify() {
  local raw=$1
  if [[ -n "${_DEP_CACHE[$raw]:-}" ]]; then
    IFS=$'\t' read -r DEP_CLASS DEP_RESOLVED_NAME DEP_REASON DEP_AUR_BASE <<<"${_DEP_CACHE[$raw]}"
    return 0
  fi

  local bare mapped
  bare=$(strdep "$raw")
  mapped=$(depmap_lookup "$bare")
  DEP_AUR_BASE=
  DEP_REASON=

  if [[ "$mapped" == "-" ]]; then
    DEP_CLASS=unresolved DEP_RESOLVED_NAME=$bare
    DEP_REASON="no Void equivalent (per depmap.conf)"
  elif xbps-query "$mapped" >/dev/null 2>&1; then
    DEP_CLASS=installed DEP_RESOLVED_NAME=$mapped
  elif xbps-query -R --repository="$REPO_DIR" "$mapped" >/dev/null 2>&1; then
    DEP_CLASS=available DEP_RESOLVED_NAME=$mapped
  else
    local base
    base=$(aur_pkgbase "$bare" 2>/dev/null) || base=
    if [[ -n "$base" ]]; then
      DEP_CLASS=aur DEP_RESOLVED_NAME=$bare DEP_AUR_BASE=$base
    else
      DEP_CLASS=unresolved DEP_RESOLVED_NAME=$bare
      DEP_REASON="not installed, not in Void repos, not found in AUR"
    fi
  fi

  _DEP_CACHE[$raw]="$DEP_CLASS	$DEP_RESOLVED_NAME	$DEP_REASON	$DEP_AUR_BASE"
}

# resolve_deps_init -- (re)initializes the build-plan globals
resolve_deps_init() {
  PLAN_ORDER=()
  declare -gA PLAN_SEEN=()
  PLAN_STACK=()
  PLAN_UNRESOLVED=()
}

# resolve_pkgbase <pkgbase> <gitdir> -- reviews+loads the PKGBUILD, recurses
# into AUR-only deps, appends to PLAN_ORDER post-order (dependency-first).
# Clobbers PB_* while recursing; reload with pkgbuild_load before using PB_*
# for a specific PLAN_ORDER entry afterward.
resolve_pkgbase() {
  local pkgbase=$1 gitdir=$2 s

  for s in "${PLAN_STACK[@]}"; do
    [[ "$s" == "$pkgbase" ]] && die "dependency cycle detected: ${PLAN_STACK[*]} -> $pkgbase"
  done
  [[ -n "${PLAN_SEEN[$pkgbase]:-}" ]] && return 0
  PLAN_STACK+=("$pkgbase")

  review_and_load "$pkgbase" "$gitdir"

  local raw combined=("${PB_DEPENDS[@]}" "${PB_MAKEDEPENDS[@]}")
  for raw in "${combined[@]}"; do
    dep_classify "$raw"
    case "$DEP_CLASS" in
      installed|available) : ;;
      aur)
        local subdir="$BUILD_DIR/$DEP_AUR_BASE/git"
        aur_clone "$DEP_AUR_BASE" "$subdir"
        resolve_pkgbase "$DEP_AUR_BASE" "$subdir"
        ;;
      unresolved)
        PLAN_UNRESOLVED+=("$pkgbase needs '$raw' ($DEP_REASON)")
        ;;
    esac
  done

  PLAN_ORDER+=("$pkgbase")
  PLAN_SEEN[$pkgbase]=1
  unset 'PLAN_STACK[-1]'
}

# runtime_deps_string -- resolved runtime deps for xbps-create -D, from
# PB_DEPENDS only. xbps pkgpatterns need an operator, so unversioned deps
# get ">=0" appended (void-packages' own convention for "any version").
runtime_deps_string() {
  local raw out=() ver
  for raw in "${PB_DEPENDS[@]}"; do
    dep_classify "$raw"
    if [[ "$DEP_CLASS" != unresolved ]]; then
      ver=$(depver "$raw")
      [[ -z "$ver" ]] && ver=">=0"
      out+=("${DEP_RESOLVED_NAME}${ver}")
    fi
  done
  printf '%s' "${out[*]}"
}
