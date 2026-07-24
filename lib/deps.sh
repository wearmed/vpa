# shellcheck shell=bash
# Dependency classification and recursive resolution against Void's xbps repos
# (falling back to AUR for anything Void doesn't have), using config/depmap.conf
# to bridge Arch/Void package-name drift.

# depmap_lookup <archname> -> the mapped Void name, the original name if
# unmapped, or the literal string "-" if explicitly marked as having no
# Void equivalent. User overrides are checked first and win.
depmap_lookup() {
  local name=$1 f line
  for f in "$USER_DEPMAP" "$DEFAULT_DEPMAP"; do
    [[ -r "$f" ]] || continue
    line=$(grep -E "^${name}=" "$f" 2>/dev/null | tail -n1) || true
    if [[ -n "$line" ]]; then
      printf '%s\n' "${line#*=}"
      return 0
    fi
  done
  printf '%s\n' "$name"
}

# dep_classify <archdepstring> -- sets DEP_CLASS, DEP_RESOLVED_NAME, DEP_REASON,
# DEP_AUR_BASE. DEP_CLASS is one of: installed, available, aur, unresolved.
dep_classify() {
  local raw=$1 bare mapped
  bare=$(strdep "$raw")
  mapped=$(depmap_lookup "$bare")
  DEP_AUR_BASE=

  if [[ "$mapped" == "-" ]]; then
    DEP_CLASS=unresolved DEP_RESOLVED_NAME=$bare
    DEP_REASON="no Void equivalent (per depmap.conf)"
    return 0
  fi
  if xbps-query "$mapped" >/dev/null 2>&1; then
    DEP_CLASS=installed DEP_RESOLVED_NAME=$mapped
    return 0
  fi
  if xbps-query -R --repository="$REPO_DIR" "$mapped" >/dev/null 2>&1; then
    DEP_CLASS=available DEP_RESOLVED_NAME=$mapped
    return 0
  fi
  local base
  base=$(aur_pkgbase "$bare" 2>/dev/null) || base=
  if [[ -n "$base" ]]; then
    DEP_CLASS=aur DEP_RESOLVED_NAME=$bare DEP_AUR_BASE=$base
    return 0
  fi
  DEP_CLASS=unresolved DEP_RESOLVED_NAME=$bare
  DEP_REASON="not installed, not in Void repos, not found in AUR"
}

# resolve_deps_init -- (re)initializes the build-plan globals. Call once
# before resolving any top-level requested packages.
resolve_deps_init() {
  PLAN_ORDER=()
  declare -gA PLAN_SEEN=()
  PLAN_STACK=()
  PLAN_UNRESOLVED=()
}

# resolve_pkgbase <pkgbase> <gitdir>
# Reviews+loads the PKGBUILD (via review_and_load, defined in the main `vur`
# script), classifies its depends+makedepends, recurses into AUR-only deps
# first, then appends pkgbase to PLAN_ORDER -- a post-order walk, so
# PLAN_ORDER always ends up in valid dependency-first build order.
#
# NOTE: this clobbers PB_* globals as it recurses. Callers must never assume
# PB_* still describes an earlier pkgbase after this returns -- reload with
# pkgbuild_load before acting on any specific entry in PLAN_ORDER.
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

# runtime_deps_string -- space-separated resolved runtime dependency patterns
# for xbps-create -D, built only from PB_DEPENDS (never PB_MAKEDEPENDS, which
# are build-time only and must not leak into shipped package metadata).
# xbps pkgpatterns require a version-comparison operator (a bare name like
# "bash" is rejected at install time with "can't guess pkgname for dependency"),
# so any dependency with no Arch-side version constraint gets ">=0" appended --
# the same convention void-packages templates use for an unconstrained dep.
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
