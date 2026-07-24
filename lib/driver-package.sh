#!/usr/bin/env bash
# Static, trusted driver invoked by run_package via
# `fakeroot -- env -i ... bash driver-package.sh`. Same environment-only
# argument-passing rule as driver-build.sh applies here.
# shellcheck disable=SC2154  # srcdir/startdir/pkgname/pkgdir are injected via `env -i` by lib/build.sh:run_package
set -e
cd "$srcdir"
# shellcheck disable=SC1091  # PKGBUILD is fetched at runtime; nothing to statically follow
source "$startdir/PKGBUILD"
fn="package_${pkgname}"
if declare -f "$fn" >/dev/null; then
  "$fn"
elif declare -f package >/dev/null; then
  package
else
  echo "vur: no package() or ${fn}() found in PKGBUILD" >&2
  exit 1
fi
