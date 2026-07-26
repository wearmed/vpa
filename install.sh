#!/usr/bin/env bash
# Bootstrap installer for vpa. Either:
#   git clone ... && cd vpa && ./install.sh
# or, piped straight from a fresh checkout:
#   curl -fsSL https://vpa.wearmed.xyz/install.sh | bash
set -euo pipefail

REPO_URL="https://git.wearmed.xyz/suraj/vpa.git"
SRC_DIR="${VPA_SRC_DIR:-$HOME/.local/share/vpa}"

c_red=$'\e[31m'; c_green=$'\e[32m'; c_yellow=$'\e[33m'; c_blue=$'\e[34m'; c_reset=$'\e[0m'
info() { printf '%s::%s %s\n' "$c_blue" "$c_reset" "$*"; }
ok()   { printf '%s::%s %s\n' "$c_green" "$c_reset" "$*"; }
warn() { printf '%s:: warning:%s %s\n' "$c_yellow" "$c_reset" "$*"; }
die()  { printf '%s:: error:%s %s\n' "$c_red" "$c_reset" "$*" >&2; exit 1; }

BIN_DIR="${VPA_BIN_DIR:-$HOME/.local/bin}"
[[ "${1:-}" == "--system" ]] && BIN_DIR=/usr/local/bin

# When run locally from a checkout, build in place; when piped via curl
# (no usable $BASH_SOURCE path), fetch/update a persistent clone instead.
SELF=$(readlink -f "${BASH_SOURCE[0]:-}" 2>/dev/null || true)
ROOT=""
[[ -n "$SELF" ]] && ROOT=$(cd "$(dirname "$SELF")" && pwd)

if [[ -z "$ROOT" || ! -f "$ROOT/go.mod" || ! -d "$ROOT/cmd/vpa" ]]; then
  command -v git >/dev/null 2>&1 || die "git is required to fetch vpa -- install it and re-run"
  if [[ -d "$SRC_DIR/.git" ]]; then
    info "updating existing vpa checkout at $SRC_DIR"
    git -C "$SRC_DIR" pull --quiet || die "git pull failed in $SRC_DIR"
  else
    info "cloning vpa into $SRC_DIR"
    git clone --quiet "$REPO_URL" "$SRC_DIR" || die "failed to clone $REPO_URL"
  fi
  ROOT="$SRC_DIR"
fi

if [[ -r /etc/os-release ]] && ! grep -Eq '^ID="?void"?$' /etc/os-release; then
  warn "this doesn't look like Void Linux -- vpa relies on xbps and won't work anywhere else"
  read -r -p "continue anyway? [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]] || exit 1
fi

command -v xbps-install >/dev/null 2>&1 || die "xbps-install not found -- is this really Void Linux?"

info "checking build-time dependencies"
missing=()
for bin_pkg in "go:go" "git:git" "curl:curl" "fakeroot:fakeroot"; do
  bin=${bin_pkg%%:*} pkg=${bin_pkg#*:}
  command -v "$bin" >/dev/null 2>&1 || missing+=("$pkg")
done
if [[ ${#missing[@]} -gt 0 ]]; then
  info "installing missing dependencies: ${missing[*]}"
  sudo xbps-install -Sy "${missing[@]}" || die "failed to install: ${missing[*]}"
else
  ok "all build-time dependencies already present"
fi

info "building vpa (static binary, no runtime deps besides git/fakeroot/xbps)"
( cd "$ROOT" && CGO_ENABLED=0 go build -ldflags="-s -w" -o vpa ./cmd/vpa ) \
  || die "go build failed"

mkdir -p "$BIN_DIR"
ln -sf "$ROOT/vpa" "$BIN_DIR/vpa"
ok "linked $BIN_DIR/vpa -> $ROOT/vpa"

case ":$PATH:" in
  *":$BIN_DIR:"*) : ;;
  *) warn "$BIN_DIR is not on your PATH -- add this to your shell rc:
    export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

CONF_DIR="$HOME/.config/vpa"
CONF_FILE="$CONF_DIR/vpa.conf"
if [[ ! -e "$CONF_FILE" ]]; then
  mkdir -p "$CONF_DIR"
  cat > "$CONF_FILE" <<'EOF'
# vpa configuration. Shell-sourced: KEY=value, one per line.
#
# NOCONFIRM=1        # never prompt for confirmation (same as passing --noconfirm)
# EDITOR=vim         # open PKGBUILDs in this editor when --edit is passed (falls back to $EDITOR/$VISUAL)
# CLEAN_AFTER=1      # remove a package's build directory after a successful install
EOF
  ok "wrote default config to $CONF_FILE"
else
  info "existing config at $CONF_FILE left untouched"
fi

ok "vpa is installed. Try: vpa search <term>"
