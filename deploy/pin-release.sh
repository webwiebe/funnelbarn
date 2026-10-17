#!/usr/bin/env bash
# Pin one env's manifests to a release before they are applied:
#   deploy/pin-release.sh <env> <image-tag> <version>
#
# The kustomization's `newTag: local` becomes the image tag and every
# `unreleased # set by the deploy workflow` becomes the version. The writer
# Deployment and the reader StatefulSet then roll out from a single
# `kubectl apply -k`, both on the same image. Setting the image afterwards with
# `kubectl set image` would first roll the StatefulSet onto the nonexistent
# `:local` image, and a StatefulSet stuck on a pod that never becomes ready
# does not recover by itself.
#
# Edits the checkout in place, so run it in CI before the manifests are copied.
set -euo pipefail

if [ "$#" -ne 3 ] && [ "$#" -ne 5 ]; then
  echo "usage: $0 <env> <image-tag> <version> [<service-digest> <web-digest>]" >&2
  exit 2
fi
env="$1" tag="$2" version="$3"
dir="deploy/k8s/${env}"

perl -pi -e "s/^(\s*newTag:) local\$/\$1 ${tag}/" "${dir}/kustomization.yaml"

# With digests, kustomize renders image@sha256:... (the tag stays as a label in
# the file only). A mutable tag can then no longer change what gets deployed.
if [ "$#" -eq 5 ]; then
  for pair in "service:$4" "web:$5"; do
    name="${pair%%:*}" digest="${pair#*:}"
    case "$digest" in
      sha256:[0-9a-f][0-9a-f]*) ;;
      *) echo "::error::bad ${name} digest: ${digest}" >&2; exit 1 ;;
    esac
    perl -0pi -e "s{(newName: \S+/funnelbarn/${name}\n(\s*)newTag: \S+)}{\$1\n\$2digest: ${digest}}" \
      "${dir}/kustomization.yaml"
    grep -q "digest: ${digest}" "${dir}/kustomization.yaml" || {
      echo "::error::could not pin ${name} to ${digest}" >&2
      exit 1
    }
  done
fi
for f in "${dir}"/*.yaml; do
  perl -pi -e "s/value: unreleased # set by the deploy workflow\$/value: \"${version}\"/" "$f"
done

if grep -n "newTag: local\|value: unreleased" "${dir}"/*.yaml; then
  echo "::error::placeholders left in ${dir}" >&2
  exit 1
fi
echo "Pinned ${dir} to image tag ${tag}, version ${version}"
