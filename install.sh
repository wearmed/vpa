#!/usr/bin/env bash
# Bootstrap installer for vur: git clone, cd vur, ./install.sh.
set -euo pipefail

SELF=$(readlink -f "${BASH_SOURCE[0]}")
ROOT=$(cd "$(dirname "$SELF")" && pwd)

c_red=$'\e[31m'; c_green=$'\e[32m'; c_yellow=$'\e[33m'; c_blue=$'\e[34m'; c_reset=$'\e[0m'
info() { printf '%s::%s %s\n' "$c_blue" "$c_reset" "$*"; }
ok()   { printf '%s::%s %s\n' "$c_green" "$c_reset" "$*"; }
warn() { printf '%s:: warning:%s %s\n' "$c_yellow" "$c_reset" "$*"; }
die()  { printf '%s:: error:%s %s\n' "$c_red" "$c_reset" "$*" >&2; exit 1; }

BIN_DIR="${VUR_BIN_DIR:-$HOME/.local/bin}"
[[ "${1:-}" == "--system" ]] && BIN_DIR=/usr/local/bin

[[ -x "$ROOT/vur" && -d "$ROOT/lib" ]] || die "run this from inside a cloned vur checkout (missing ./vur or ./lib)"

if [[ -r /etc/os-release ]] && ! grep -Eq '^ID="?void"?$' /etc/os-release; then
  warn "this doesn't look like Void Linux -- vur relies on xbps and won't work anywhere else"
  read -r -p "continue anyway? [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]] || exit 1
fi

command -v xbps-install >/dev/null 2>&1 || die "xbps-install not found -- is this really Void Linux?"

info "checking build-time dependencies"
missing=()
for bin_pkg in "git:git" "curl:curl" "jq:jq" "fakeroot:fakeroot"; do
  bin=${bin_pkg%%:*} pkg=${bin_pkg#*:}
  command -v "$bin" >/dev/null 2>&1 || missing+=("$pkg")
done
if [[ ${#missing[@]} -gt 0 ]]; then
  info "installing missing dependencies: ${missing[*]}"
  sudo xbps-install -Sy "${missing[@]}" || die "failed to install: ${missing[*]}"
else
  ok "all build-time dependencies already present"
fi

mkdir -p "$BIN_DIR"
ln -sf "$ROOT/vur" "$BIN_DIR/vur"
chmod +x "$ROOT/vur"
ok "linked $BIN_DIR/vur -> $ROOT/vur"

case ":$PATH:" in
  *":$BIN_DIR:"*) : ;;
  *) warn "$BIN_DIR is not on your PATH -- add this to your shell rc:
    export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

CONF_DIR="$HOME/.config/vur"
CONF_FILE="$CONF_DIR/vur.conf"
if [[ ! -e "$CONF_FILE" ]]; then
  mkdir -p "$CONF_DIR"
  cat > "$CONF_FILE" <<'EOF'
# vur configuration. Shell-sourced: KEY=value, one per line.
#
# NOCONFIRM=1        # never prompt for confirmation (same as passing --noconfirm)
# EDITOR=vim         # open PKGBUILDs in this editor when --edit is passed (falls back to $EDITOR/$VISUAL)
# CLEAN_AFTER=1      # remove a package's build directory after a successful install
EOF
  ok "wrote default config to $CONF_FILE"
else
  info "existing config at $CONF_FILE left untouched"
fi

ok "vur is installed. Try: vur search <term>"
