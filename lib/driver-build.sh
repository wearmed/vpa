#!/usr/bin/env bash
# Static, trusted driver invoked by run_build via `env -i ... bash driver-build.sh`.
# All per-package values arrive as inherited environment variables (never
# interpolated as shell text), so untrusted PKGBUILD content cannot affect
# how this script itself is parsed.
# shellcheck disable=SC2154  # srcdir/startdir are injected via `env -i` by lib/build.sh:run_build
set -e
cd "$srcdir"
# shellcheck disable=SC1091  # PKGBUILD is fetched at runtime; nothing to statically follow
source "$startdir/PKGBUILD"
declare -f prepare >/dev/null && prepare
declare -f build >/dev/null && build
exit 0
