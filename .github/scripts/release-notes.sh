#!/usr/bin/env bash
# Print the release notes of a version from CHANGELOG.md, and check that
# the release is ready: the version has its own section and
# deploy/.env.example pins the image to it.
#
#   .github/scripts/release-notes.sh 0.1.0 > notes.md
#
# A prerelease (0.2.0-rc.1) may have no section of its own; it gets the
# Unreleased section and keeps .env.example on the last stable version.
set -euo pipefail

version=${1:?usage: release-notes.sh <version without v>}
cd "$(dirname "$0")/../.."

section() {
  awk -v head="## [$1]" '
    /^## \[/ { if (found) exit; found = index($0, head) == 1; next }
    /^\[[^]]+\]: / { next } # link references at the end
    found' CHANGELOG.md
}

notes=$(section "$version")
if [[ -z ${notes//[[:space:]]/} ]]; then
  if [[ $version != *-* ]]; then
    echo "CHANGELOG.md has no section \"## [$version] - <date>\"." >&2
    exit 1
  fi
  notes=$(section Unreleased)
fi

if [[ $version != *-* ]] &&
  ! grep -qx "PAGEWRIGHT_IMAGE=ghcr.io/kom3da/pagewright:$version" deploy/.env.example; then
  echo "deploy/.env.example must set PAGEWRIGHT_IMAGE=ghcr.io/kom3da/pagewright:$version." >&2
  exit 1
fi

printf '%s\n' "$notes"
