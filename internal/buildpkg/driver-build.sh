#!/usr/bin/env bash
# Invoked by run_build via `env -i ... bash driver-build.sh`. Values arrive
# via env, never interpolated as shell text.
# shellcheck disable=SC2154  # srcdir/startdir injected by lib/build.sh:run_build
set -e
cd "$srcdir"
# shellcheck disable=SC1091  # PKGBUILD is fetched at runtime
source "$startdir/PKGBUILD"
declare -f prepare >/dev/null && prepare
declare -f build >/dev/null && build
exit 0
