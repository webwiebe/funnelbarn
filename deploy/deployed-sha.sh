#!/usr/bin/env bash
# Print the commit SHA a deployment is running:
#   deploy/deployed-sha.sh <release-annotation> <container-image>
#
# The deploy writes the SHA to the deployment annotation
# funnelbarn.wiebe.xyz/release-sha after a release passed its smoke check, so
# the rollback logic does not depend on the image string. Images deployed by
# digest (repo@sha256:...) carry no tag. For a deployment that predates the
# annotation and still runs a tag-form image, the tag is the SHA.
# Prints nothing when the SHA cannot be determined.
set -euo pipefail

annotation="${1:-}" image="${2:-}"

if [ -n "$annotation" ]; then
  printf '%s\n' "$annotation"
  exit 0
fi

case "$image" in
  *@*) ;;                       # digest form: no tag to read
  *:*) printf '%s\n' "${image##*:}" ;;
esac
