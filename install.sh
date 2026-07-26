#!/usr/bin/env bash
# Bootstrap installer for vpa.
#
#   curl -fsSL https://vpa.wearmed.xyz/install.sh | bash
#
# Adds vpa's package repository and installs vpa from it, so vpa is an
# ordinary xbps package that upgrades with the rest of the system. Nothing
# is built and nothing is written outside xbps's control.
#
# Says what it is doing, one short line per step, without turning into a
# log. Anything needing a decision, or anything that has gone wrong, still
# speaks up, and --verbose adds the detail underneath each step.
set -euo pipefail

REPO_URL="${VPA_REPO_URL:-https://vpa.wearmed.xyz/repo}"
REPO_CONF="${VPA_REPO_CONF:-/etc/xbps.d/vpa.conf}"

# The fingerprint of the key the repository is signed with. Shown before
# the trust prompt so it can be compared against a second source rather
# than accepted purely because xbps asked.
REPO_FINGERPRINT="77:a8:39:cc:3d:df:8c:a7:12:d5:fe:56:d2:fb:86:13"

# Optional at runtime: vpa only needs these to build something from the AUR,
# and works without them for everything else. Installed anyway so the first
# AUR build doesn't stop halfway to ask for a compiler.
EXTRA_DEPS=(git fakeroot bsdtar xz zstd bzip2 base-devel)

c_red=$'\e[31m'; c_green=$'\e[32m'; c_yellow=$'\e[33m'; c_blue=$'\e[34m'; c_reset=$'\e[0m'
VERBOSE=0
WITH_AUR=1
FORCE=0

for arg in "$@"; do
  case "$arg" in
    --verbose|-v)  VERBOSE=1 ;;
    --minimal)     WITH_AUR=0 ;;
    --force)       FORCE=1 ;;
    --help|-h)
      printf 'usage: install.sh [--minimal] [--force] [--verbose]\n\n'
      printf '  --minimal  install vpa alone, without the AUR build tools\n'
      printf '  --force    run even if vpa is already installed\n'
      printf '  --verbose  show every step\n'
      exit 0 ;;
  esac
done

# say is the running commentary: one short line per thing being done, so
# the install accounts for itself without turning into a log.
say()  { printf '  %s\n' "$*"; }
# step is the detail underneath that, shown only with --verbose.
step() { [[ $VERBOSE -eq 1 ]] && printf '%s  ::%s %s\n' "$c_blue" "$c_reset" "$*"; return 0; }
warn() { printf '%s:: warning:%s %s\n' "$c_yellow" "$c_reset" "$*" >&2; }
die()  { printf '%s:: error:%s %s\n' "$c_red" "$c_reset" "$*" >&2; exit 1; }

LOG=$(mktemp -t vpa-install-XXXXXX.log)
trap 'rm -f "$LOG"' EXIT

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

# This script only installs. Upgrading is xbps's job once the repository is
# configured, so re-running it on a working install has nothing to do --
# and doing it anyway would re-prompt for a password and reinstall packages
# for no reason.
if [[ $FORCE -eq 0 ]] && { command -v vpa >/dev/null 2>&1 || xbps-query vpa >/dev/null 2>&1; }; then
  installed=$(vpa --version 2>/dev/null || xbps-query -p pkgver vpa 2>/dev/null || echo vpa)
  printf 'VPA is already installed (%s).\n\n' "$installed"
  printf 'Run "vpa" to get started, or "vpa update" to upgrade it.\n'
  printf 'Pass --force to run the installer anyway.\n'
  exit 0
fi

printf 'Installing VPA...\n'

# vpa drives xbps and expects runit services, so anything else is a hard
# stop rather than a warning -- there is no partial success to salvage.
refuse() {
  [[ $VERBOSE -eq 1 ]] && warn "$1"
  die "Please run the script from Void Linux"
}

# /etc/os-release is parsed rather than sourced: it's shell syntax, but
# executing a file just to read one field is a habit worth not having.
os_id=""
[[ -r /etc/os-release ]] && os_id=$(sed -n 's/^ID="\?\([^"]*\)"\?$/\1/p' /etc/os-release | head -1)
[[ "$os_id" == "void" ]] || refuse "ID in /etc/os-release is '${os_id:-unset}', not 'void'"

command -v xbps-install >/dev/null 2>&1 || refuse "xbps-install not found"

# PID 1's name is the direct answer, and /proc needs no extra package --
# ps is only a fallback for the odd system without /proc mounted.
init=""
if [[ -r /proc/1/comm ]]; then
  init=$(tr -d '\0\n' < /proc/1/comm)
elif command -v ps >/dev/null 2>&1; then
  init=$(ps -p 1 -o comm= 2>/dev/null | tr -d ' ')
fi
[[ "$init" == "runit" ]] || refuse "init system is '${init:-unknown}', not runit"

# sudo is how every write below happens, so check it before the first one
# rather than failing partway through.
command -v sudo >/dev/null 2>&1 || die "sudo is required to install packages"

# Announced before the prompt appears, so the password request isn't a
# surprise arriving under a silent progress line.
if ! sudo -n true 2>/dev/null; then
  say "Administrator access is needed to install packages."
fi

if [[ -r "$REPO_CONF" ]] && grep -qF "repository=$REPO_URL" "$REPO_CONF" 2>/dev/null; then
  say "Repository already added."
  step "already configured in $REPO_CONF"
else
  say "Adding the repository..."
  step "$REPO_URL -> $REPO_CONF"
  write_repo_conf() {
    printf 'repository=%s\n' "$REPO_URL" | sudo tee "$REPO_CONF" >/dev/null
  }
  quietly "couldn't write $REPO_CONF" write_repo_conf
  step "wrote $REPO_CONF"
fi

# Importing the signing key is a yes/no prompt the first time. Under
# `curl | bash` the script itself is stdin, so xbps would read EOF and fail
# with "Resource temporarily unavailable" -- the prompt has to be pointed at
# the terminal explicitly. Opening /dev/tty is the test: it can exist and
# still fail to open when there is no controlling terminal.
# Querying a repository does not need its key -- only installing from one
# does -- so asking xbps whether it can see the package says nothing about
# trust. The imported key is a file named after its fingerprint, so look for
# exactly the key this script expects.
key_trusted() {
  [[ -e "/var/db/xbps/keys/${REPO_FINGERPRINT}.plist" ]]
}

if ! key_trusted; then
  say "Trusting the signing key..."
  printf '\nThe repository is signed with this key:\n  %s\n' "$REPO_FINGERPRINT"
  printf 'xbps will ask you to trust it.\n\n'
  if { exec 3</dev/tty; } 2>/dev/null; then
    sudo xbps-install -S <&3 || true
    exec 3<&-
  else
    # shellcheck disable=SC2024  # $LOG is user-owned; this shell should write it
    sudo xbps-install -S >>"$LOG" 2>&1 || true
  fi
fi

pkgs=(vpa)
if [[ $WITH_AUR -eq 1 ]]; then
  pkgs+=("${EXTRA_DEPS[@]}")
  say "Installing the package and AUR build tools..."
  step "${pkgs[*]}"
else
  say "Installing the package..."
fi

if ! quietly_out=$(sudo xbps-install -Sy "${pkgs[@]}" 2>&1); then
  printf '\n'
  warn "installing vpa failed"
  printf -- '--- output ---\n' >&2
  printf '%s\n' "$quietly_out" | tail -n 30 >&2
  if printf '%s' "$quietly_out" | grep -q 'not signed\|signature\|pubkey'; then
    warn "the repository's key was not trusted -- re-run this script from a terminal"
  fi
  exit 1
fi
[[ $VERBOSE -eq 1 ]] && printf '%s\n' "$quietly_out"

command -v vpa >/dev/null 2>&1 || die "vpa installed but isn't on PATH -- expected /usr/bin/vpa"

# Confirm the package file xbps downloaded matches the checksum published
# alongside it.
#
# This is not what makes the repository trustworthy: xbps already checks a
# SHA256 from the repodata and an RSA signature against a key you trusted, and
# both the packages and this list come from the same server, so it cannot
# detect a compromised origin. What it does catch is a truncated download, a
# corrupted cache, or a mirror serving something stale -- and it makes the
# expected value visible so it can be compared elsewhere.
verify_sha256() {
  command -v sha256sum >/dev/null 2>&1 || { step "sha256sum unavailable, skipping"; return 0; }
  say "Verifying the download..."

  local arch pkgver cached sums expected actual
  arch=$(xbps-uhelper arch 2>/dev/null) || return 0
  pkgver=$(xbps-query -p pkgver vpa 2>/dev/null) || return 0
  cached="/var/cache/xbps/${pkgver}.${arch}.xbps"

  if [[ ! -r "$cached" ]]; then
    # Already installed at this version, so nothing was downloaded.
    step "no cached package to check for $pkgver"
    return 0
  fi

  sums=$(curl -fsSL --max-time 30 "$REPO_URL/sha256sums.txt" 2>/dev/null) || {
    warn "couldn't fetch sha256sums.txt -- skipping the checksum check"
    return 0
  }
  expected=$(printf '%s\n' "$sums" | awk -v f="${pkgver}.${arch}.xbps" '$2 == f {print $1}')
  if [[ -z "$expected" ]]; then
    step "no published checksum for ${pkgver}.${arch}.xbps"
    return 0
  fi

  actual=$(sha256sum "$cached" | cut -d' ' -f1)
  if [[ "$actual" != "$expected" ]]; then
    warn "SHA256 MISMATCH for ${pkgver}.${arch}.xbps"
    warn "  expected $expected"
    warn "  got      $actual"
    die "the downloaded package does not match what the repository published"
  fi
  step "sha256 verified: ${expected:0:16}... for ${pkgver}.${arch}.xbps"
}

verify_sha256

VERSION=$(vpa --version 2>/dev/null || echo vpa)
printf '%sDone.%s %s\n' "$c_green" "$c_reset" "$VERSION"

# A leftover source install from an earlier version of this script would
# shadow the packaged one and keep being what actually runs.
for stale in "$HOME/.local/bin/vpa" /usr/local/bin/vpa; do
  if [[ -e "$stale" && "$(command -v vpa)" != "/usr/bin/vpa" ]]; then
    warn "'$stale' comes earlier in your PATH than the package, so that's what runs."
    warn "remove it with: rm $stale"
  fi
done

printf '\nTo get started, run "vpa" in your terminal\n'
