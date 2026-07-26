#!/usr/bin/env bash
# Bootstrap installer for vpa. Either:
#   git clone ... && cd vpa && ./install.sh
# or, piped straight from a fresh checkout:
#   curl -fsSL https://vpa.wearmed.xyz/install.sh | bash
#
# Quiet by default: one line while it works, one when it's done. Anything
# that actually needs a decision or has gone wrong still speaks up, and
# --verbose shows every step.
set -euo pipefail

REPO_URL="https://git.wearmed.xyz/suraj/vpa.git"
SRC_DIR="${VPA_SRC_DIR:-$HOME/.local/share/vpa}"

c_red=$'\e[31m'; c_green=$'\e[32m'; c_yellow=$'\e[33m'; c_blue=$'\e[34m'; c_reset=$'\e[0m'
VERBOSE=0
BIN_DIR="${VPA_BIN_DIR:-$HOME/.local/bin}"

for arg in "$@"; do
  case "$arg" in
    --system)  BIN_DIR=/usr/local/bin ;;
    --verbose|-v) VERBOSE=1 ;;
    --help|-h)
      printf 'usage: install.sh [--system] [--verbose]\n\n'
      printf '  --system   install to /usr/local/bin instead of ~/.local/bin\n'
      printf '  --verbose  show every step\n'
      exit 0 ;;
  esac
done

# step is the running commentary: shown only with --verbose, since the
# point of the default output is that there isn't any.
step() { [[ $VERBOSE -eq 1 ]] && printf '%s::%s %s\n' "$c_blue" "$c_reset" "$*"; return 0; }
warn() { printf '%s:: warning:%s %s\n' "$c_yellow" "$c_reset" "$*" >&2; }
die()  { printf '%s:: error:%s %s\n' "$c_red" "$c_reset" "$*" >&2; exit 1; }

# LOG collects the output of everything run through quietly(), so a failure
# can show what happened instead of just an exit code.
LOG=$(mktemp -t vpa-install-XXXXXX.log)
cleanup() { rm -f "$LOG"; }
trap cleanup EXIT

# quietly runs a command, swallowing its output unless it fails (or
# --verbose). The message it was given becomes the error text.
quietly() {
  local what="$1"; shift
  if [[ $VERBOSE -eq 1 ]]; then
    "$@" || die "$what"
    return 0
  fi
  if ! "$@" >>"$LOG" 2>&1; then
    printf '\n'
    warn "$what"
    printf -- '--- output ---\n' >&2
    tail -n 30 "$LOG" >&2
    exit 1
  fi
}

printf 'Installing VPA...\n'

# When run locally from a checkout, build in place; when piped via curl
# (no usable $BASH_SOURCE path), fetch/update a persistent clone instead.
SELF=$(readlink -f "${BASH_SOURCE[0]:-}" 2>/dev/null || true)
ROOT=""
[[ -n "$SELF" ]] && ROOT=$(cd "$(dirname "$SELF")" && pwd)

if [[ -z "$ROOT" || ! -f "$ROOT/go.mod" || ! -d "$ROOT/cmd/vpa" ]]; then
  command -v git >/dev/null 2>&1 || die "git is required to fetch vpa -- install it and re-run"
  if [[ -d "$SRC_DIR/.git" ]]; then
    step "updating existing vpa checkout at $SRC_DIR"
    quietly "couldn't update the checkout at $SRC_DIR" git -C "$SRC_DIR" pull --quiet
  else
    step "cloning vpa into $SRC_DIR"
    quietly "couldn't clone $REPO_URL" git clone --quiet "$REPO_URL" "$SRC_DIR"
  fi
  ROOT="$SRC_DIR"
fi

# Never silent: continuing on a non-Void system is the user's call, and
# xbps missing means nothing after this point can work.
if [[ -r /etc/os-release ]] && ! grep -Eq '^ID="?void"?$' /etc/os-release; then
  warn "this doesn't look like Void Linux -- vpa relies on xbps and won't work anywhere else"
  # Piped through `curl | bash`, stdin is the script itself, so a plain
  # read hits EOF immediately: the prompt would be skipped, and under
  # `set -e` the non-zero return would kill the script with no message.
  # Ask the terminal directly, and refuse rather than assume when there
  # isn't one.
  reply=""
  # Opening it is the only real test: /dev/tty can exist and still fail to
  # open when there's no controlling terminal (cron, a container, CI).
  if { exec 3</dev/tty; } 2>/dev/null; then
    read -r -p "continue anyway? [y/N] " reply <&3 || reply=""
    exec 3<&-
  else
    warn "no terminal to ask on -- re-run without piping if you meant to continue"
  fi
  [[ "$reply" =~ ^[Yy]$ ]] || exit 1
fi
command -v xbps-install >/dev/null 2>&1 || die "xbps-install not found -- is this really Void Linux?"

step "checking build-time dependencies"
missing=()
for bin_pkg in "go:go" "git:git" "curl:curl" "fakeroot:fakeroot"; do
  bin=${bin_pkg%%:*} pkg=${bin_pkg#*:}
  command -v "$bin" >/dev/null 2>&1 || missing+=("$pkg")
done
if [[ ${#missing[@]} -gt 0 ]]; then
  # Announced even when quiet: this asks for a sudo password and installs
  # packages, which shouldn't happen behind a silent progress line.
  printf 'Installing build dependencies: %s\n' "${missing[*]}"
  quietly "failed to install: ${missing[*]}" sudo xbps-install -Sy "${missing[@]}"
fi

build_vpa() (
  cd "$ROOT" && CGO_ENABLED=0 go build -ldflags="-s -w" -o vpa ./cmd/vpa
)

step "building vpa"
quietly "the build failed" build_vpa

mkdir -p "$BIN_DIR"
ln -sf "$ROOT/vpa" "$BIN_DIR/vpa"
step "linked $BIN_DIR/vpa -> $ROOT/vpa"

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
  step "wrote default config to $CONF_FILE"
fi

VERSION=$("$ROOT/vpa" --version 2>/dev/null || echo vpa)
printf '%sDone.%s %s\n' "$c_green" "$c_reset" "$VERSION"

# Kept loud: without this the command simply won't be found, which looks
# like the install failing rather than a PATH problem.
case ":$PATH:" in
  *":$BIN_DIR:"*) : ;;
  *) warn "$BIN_DIR is not on your PATH -- add this to your shell rc:
    export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

# An xbps-managed vpa earlier in PATH would keep being the one that runs.
if command -v xbps-query >/dev/null 2>&1 && xbps-query vpa >/dev/null 2>&1; then
  warn "the vpa package is also installed; whichever comes first in PATH is the one you'll run"
fi
