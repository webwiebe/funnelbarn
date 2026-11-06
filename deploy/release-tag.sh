#!/usr/bin/env bash
# Print the release tag a deploy reports as its version:
#   deploy/release-tag.sh <vX.Y.Z tag or commit sha>
#
# A vX.Y.Z tag prints itself. A commit prints the highest vX.Y.Z tag on that
# commit. binary-release creates that tag on the same push to main, beside CI,
# so when it is not there yet the script fetches tags and tries again, up to
# RELEASE_TAG_ATTEMPTS times (default 10) with RELEASE_TAG_WAIT seconds between
# them (default 30). A commit without a release tag fails, so production and
# staging never report a bare commit sha (#489). Run it in a checkout with tags,
# e.g. actions/checkout with fetch-depth: 0.
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: $0 <vX.Y.Z tag or commit sha>" >&2
  exit 2
fi
input="$1"
attempts="${RELEASE_TAG_ATTEMPTS:-10}"
wait="${RELEASE_TAG_WAIT:-30}"
semver='^v[0-9]+\.[0-9]+\.[0-9]+$'

if [[ "$input" =~ $semver ]]; then
  if git rev-parse -q --verify "refs/tags/${input}" >/dev/null; then
    echo "$input"
    exit 0
  fi
  echo "::error::tag ${input} not found" >&2
  exit 1
fi

sha="$(git rev-parse -q --verify "${input}^{commit}")" || {
  echo "::error::${input} is neither a vX.Y.Z tag nor a commit in this checkout" >&2
  exit 1
}

for i in $(seq 1 "$attempts"); do
  tag="$(git tag --points-at "$sha" --sort=-v:refname | grep -E "$semver" | head -n 1 || true)"
  if [ -n "$tag" ]; then
    echo "$tag"
    exit 0
  fi
  if [ "$i" -lt "$attempts" ]; then
    echo "attempt ${i}/${attempts}: no release tag on ${sha} yet, fetching tags in ${wait}s" >&2
    sleep "$wait"
    git fetch -q --tags --force origin
  fi
done
echo "::error::no vX.Y.Z tag points at ${sha} after ${attempts} attempts; binary-release creates it on every push to main" >&2
exit 1
