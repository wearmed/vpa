#!/usr/bin/env bash
# Builds vpa as real .xbps packages and puts them in a signed repository.
#
#   ./packaging/mkrepo.sh            build + sign into packaging/repo/
#   ./packaging/mkrepo.sh --publish  ...then upload it to the web server
#
# vpa is a static CGO_ENABLED=0 binary, so one build per CPU architecture
# covers both glibc and musl -- it links no libc at all. The packages still
# have to be built and indexed separately, because the architecture is
# baked into the package and into the repodata filename.
#
# Every version ever published stays in the repository. The index only ever
# points at the newest one, but keeping the older files on disk is what
# makes `vpa downgrade` possible -- xbps can install an exact .xbps that
# the index no longer mentions.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
REPO_DIR="${VPA_REPO_DIR:-$ROOT/packaging/repo}"
KEY="${VPA_REPO_KEY:-$HOME/.config/vpa-repo/privkey.pem}"
SIGNEDBY="${VPA_REPO_SIGNEDBY:-Suraj <emailwearmed@gmail.com>}"

PUBLISH_HOST="${VPA_PUBLISH_HOST:-root@wearmed.xyz}"
PUBLISH_PATH="${VPA_PUBLISH_PATH:-/var/www/vpa.wearmed.xyz/repo}"
PUBLISH_URL="${VPA_PUBLISH_URL:-https://vpa.wearmed.xyz/repo}"

# Architectures to publish. All of them share one directory: the repodata
# filename carries the architecture, so x86_64-repodata and
# x86_64-musl-repodata coexist happily and everyone gets one repo URL.
ARCHES=(x86_64 x86_64-musl aarch64 aarch64-musl)

c_green=$'\e[32m'; c_yellow=$'\e[33m'; c_red=$'\e[31m'; c_blue=$'\e[34m'; c_reset=$'\e[0m'
info() { printf '%s::%s %s\n' "$c_blue" "$c_reset" "$*"; }
ok()   { printf '%s::%s %s\n' "$c_green" "$c_reset" "$*"; }
warn() { printf '%s:: warning:%s %s\n' "$c_yellow" "$c_reset" "$*"; }
die()  { printf '%s:: error:%s %s\n' "$c_red" "$c_reset" "$*" >&2; exit 1; }

for t in go xbps-create xbps-rindex; do
  command -v "$t" >/dev/null || die "$t is required but not installed"
done

VERSION=$(grep -oP 'const Version = "\K[^"]+' "$ROOT/cmd/vpa/main.go") \
  || die "couldn't read the version out of cmd/vpa/main.go"
REVISION="${VPA_PKG_REVISION:-1}"
info "packaging vpa $VERSION (revision $REVISION)"

# goarch maps an xbps architecture to the GOARCH that produces it. The libc
# half of the name is irrelevant to a static binary, so it's stripped.
goarch() {
  case "${1%-musl}" in
    x86_64)  echo amd64 ;;
    aarch64) echo arm64 ;;
    i686)    echo 386 ;;
    armv7l)  echo arm ;;
    *) die "no GOARCH known for architecture '$1'" ;;
  esac
}

if [[ ! -f "$KEY" ]]; then
  warn "no signing key at $KEY"
  info "creating one now -- keep it safe and back it up"
  info "if it's ever lost, everyone who added this repo has to re-trust a new key"
  mkdir -p "$(dirname "$KEY")"
  ( umask 077; openssl genrsa -out "$KEY" 4096 2>/dev/null )
  chmod 600 "$KEY"
  ok "wrote $KEY"
fi

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

mkdir -p "$REPO_DIR"

for arch in "${ARCHES[@]}"; do
  ga=$(goarch "$arch")
  destdir="$STAGE/$arch"
  mkdir -p "$destdir/usr/bin" "$destdir/usr/share/licenses/vpa"

  info "building vpa for $arch (GOARCH=$ga)"
  ( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$ga" \
      go build -trimpath -ldflags="-s -w" -o "$destdir/usr/bin/vpa" ./cmd/vpa )
  chmod 755 "$destdir/usr/bin/vpa"
  install -m644 "$ROOT/LICENSE" "$destdir/usr/share/licenses/vpa/LICENSE"

  # Dependencies are the tools vpa shells out to. They're deliberately
  # minimal: everything AUR-specific (git, fakeroot, bsdtar) is only needed
  # if you actually build something from the AUR, and flatpak is optional
  # by design, so none of them are hard requirements for installing vpa.
  ( cd "$STAGE" && xbps-create \
      --architecture "$arch" \
      --pkgver "vpa-${VERSION}_${REVISION}" \
      --desc "The endgame Void Linux package manager" \
      --homepage "https://vpa.wearmed.xyz" \
      --license "GPL-3.0-or-later" \
      --maintainer "$SIGNEDBY" \
      --dependencies "xbps>=0.59 curl>=0" \
      --quiet \
      "$arch" >/dev/null )

  mv "$STAGE/vpa-${VERSION}_${REVISION}.${arch}.xbps" "$REPO_DIR/"
  # xbps-rindex --sign-pkg silently leaves an existing .sig2 alone, so a
  # rebuilt package would keep the previous build's signature. Local
  # installs don't notice (signatures are only enforced for remote
  # repositories), which makes it fail exactly where it matters.
  rm -f "$REPO_DIR/vpa-${VERSION}_${REVISION}.${arch}.xbps.sig2"
  ok "built vpa-${VERSION}_${REVISION}.${arch}.xbps"
done

# xbps-rindex only considers packages matching the architecture it is
# running as, so cross-architecture indexing needs XBPS_ARCH set per pass.
# Without it, everything but the host's own architecture is silently
# skipped with "ignoring ..., unmatched arch".
for arch in "${ARCHES[@]}"; do
  info "indexing $arch"
  # -f because rebuilding a version that is already indexed is normal
  # during development. Without it xbps-rindex keeps the existing entry,
  # leaving the index pointing at the previous build's checksum, and every
  # install then fails with "checksum does not match repository index".
  XBPS_ARCH="$arch" xbps-rindex -f -a "$REPO_DIR"/*."$arch".xbps >/dev/null
  XBPS_ARCH="$arch" xbps-rindex --sign --privkey "$KEY" \
    --signedby "$SIGNEDBY" "$REPO_DIR" >/dev/null
  XBPS_ARCH="$arch" xbps-rindex --sign-pkg --privkey "$KEY" \
    "$REPO_DIR"/*."$arch".xbps >/dev/null
done
ok "indexed and signed"

# A checksum list over the packages. xbps already verifies a SHA256 from the
# repodata and an RSA signature per package, so this is not what makes the
# repository trustworthy -- it is here so the install script can confirm the
# bytes it ended up with match what was published, and so anyone can check a
# package by hand without xbps.
( cd "$REPO_DIR" && sha256sum ./*.xbps | sed 's| \./| |' > sha256sums.txt )
ok "wrote sha256sums.txt ($(wc -l < "$REPO_DIR/sha256sums.txt") packages)"

echo
ok "repository ready at $REPO_DIR"
find "$REPO_DIR" -name '*-repodata' -printf '  %f\n' | sort
echo "  $(find "$REPO_DIR" -name '*.xbps' | wc -l) package files across $(printf '%s\n' "${ARCHES[@]}" | wc -l) architectures"

if [[ "${1:-}" == "--publish" ]]; then
  command -v rsync >/dev/null || die "rsync is required to publish"
  info "publishing to $PUBLISH_HOST:$PUBLISH_PATH"
  # shellcheck disable=SC2029  # expanding the path locally is the intent
  ssh "$PUBLISH_HOST" "mkdir -p '$PUBLISH_PATH'"

  # Two passes, packages before repodata. The index is what tells a client a
  # version exists, so publishing it first opens a window where anyone
  # syncing is told to fetch a package that is still uploading -- their
  # upgrade then fails partway, after xbps has already removed the old
  # version. Uploading the packages first closes that window.
  #
  # Not --delete: an older release someone pinned should keep working.
  rsync -av --chmod=D755,F644 \
    --exclude='*-repodata' --exclude='sha256sums.txt' \
    "$REPO_DIR/" "$PUBLISH_HOST:$PUBLISH_PATH/"
  rsync -av --chmod=D755,F644 \
    --include='*-repodata' --include='sha256sums.txt' --exclude='*' \
    "$REPO_DIR/" "$PUBLISH_HOST:$PUBLISH_PATH/"
  ok "published"
  echo
  info "install with:"
  echo "  sudo vpa addrepo $PUBLISH_URL"
  echo "  sudo xbps-install -S vpa"
fi
