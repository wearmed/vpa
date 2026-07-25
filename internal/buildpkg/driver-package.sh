#!/usr/bin/env bash
# Invoked by run_package via `fakeroot -- env -i ... bash driver-package.sh`.
# shellcheck disable=SC2154  # srcdir/startdir/pkgname/pkgdir injected by lib/build.sh:run_package
set -e
cd "$srcdir"
# shellcheck disable=SC1091  # PKGBUILD is fetched at runtime
source "$startdir/PKGBUILD"
fn="package_${pkgname}"
if declare -f "$fn" >/dev/null; then
  "$fn"
elif declare -f package >/dev/null; then
  package
else
  echo "vpa: no package() or ${fn}() found in PKGBUILD" >&2
  exit 1
fi
