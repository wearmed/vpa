# shellcheck shell=bash
# Shared state, logging and small helpers used by every other lib/*.sh file.
# Expects $ROOT (the vur project directory) to already be exported by the caller.
#
# shellcheck disable=SC2034  # these globals are consumed by other lib/*.sh files, not this one

CACHE_DIR="${VUR_CACHE:-$HOME/.cache/vur}"
BUILD_DIR="$CACHE_DIR/build"
REPO_DIR="$CACHE_DIR/repo"
MANIFEST="$CACHE_DIR/installed.db"
USER_DEPMAP="$HOME/.config/vur/depmap.conf"     # used by lib/deps.sh
DEFAULT_DEPMAP="$ROOT/config/depmap.conf"       # used by lib/deps.sh

AUR_RPC="https://aur.archlinux.org/rpc/v5"      # used by lib/aur.sh
AUR_GIT="https://aur.archlinux.org"              # used by lib/aur.sh

mkdir -p "$BUILD_DIR" "$REPO_DIR" "$(dirname "$MANIFEST")"
touch "$MANIFEST"

if [[ -t 2 ]]; then
  c_red=$'\e[31m'; c_green=$'\e[32m'; c_yellow=$'\e[33m'; c_blue=$'\e[34m'; c_bold=$'\e[1m'; c_reset=$'\e[0m'
else
  c_red=; c_green=; c_yellow=; c_blue=; c_bold=; c_reset=  # c_bold used by lib/aur.sh
fi

info() { printf '%s::%s %s\n' "$c_blue" "$c_reset" "$*" >&2; }
ok()   { printf '%s::%s %s\n' "$c_green" "$c_reset" "$*" >&2; }
warn() { printf '%s:: warning:%s %s\n' "$c_yellow" "$c_reset" "$*" >&2; }
die()  { printf '%s:: error:%s %s\n' "$c_red" "$c_reset" "$*" >&2; exit 1; }

confirm() {
  local prompt=${1:-"Continue?"} reply
  read -r -p "$prompt [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]]
}

# require_bin <binary> [xbps-package-name]
# Aborts unless the binary is present, offering to install it via xbps first.
require_bin() {
  local bin=$1 pkg=${2:-$1}
  command -v "$bin" >/dev/null 2>&1 && return 0
  warn "'$bin' is required but not installed (package: $pkg)."
  if confirm "Install $pkg now with sudo xbps-install?"; then
    sudo xbps-install -Sy "$pkg" || die "failed to install $pkg"
  else
    die "cannot continue without $bin"
  fi
}

# manifest_set <pkgname> <pkgver-pkgrel>
manifest_set() {
  local name=$1 ver=$2
  local tmp; tmp="$(mktemp "$MANIFEST.XXXXXX")"
  grep -v -E "^${name}	" "$MANIFEST" > "$tmp" 2>/dev/null || true
  printf '%s\t%s\n' "$name" "$ver" >> "$tmp"
  mv "$tmp" "$MANIFEST"
}

manifest_remove() {
  local name=$1
  local tmp; tmp="$(mktemp "$MANIFEST.XXXXXX")"
  grep -v -E "^${name}	" "$MANIFEST" > "$tmp" 2>/dev/null || true
  mv "$tmp" "$MANIFEST"
}

manifest_ver() {
  local name=$1
  awk -F'\t' -v n="$name" '$1==n{print $2}' "$MANIFEST"
}

void_arch() {
  xbps-uhelper arch
}
