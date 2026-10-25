#!/bin/sh
# Record a workload's rollout revision before a deploy, and put that revision
# back after a failed one (issue #396).
#
#   restore-revision.sh record  NAMESPACE KIND/NAME
#   restore-revision.sh restore NAMESPACE KIND/NAME RECORDED
#
# KIND/NAME is a Deployment or StatefulSet, e.g. deployment/funnelbarn.
# `record` prints the newest revision, which is the one running.
#
# `restore` compares the newest revision with RECORDED. Equal means the apply
# never changed the workload, so there is nothing to undo. A plain `rollout
# undo` would still step back one revision, past the release that was running
# (2026-10-23: the writer went from fe5cb49 to 4047837 after an apply timed
# out). Different means the apply landed, and `undo --to-revision=RECORDED`
# puts back exactly what ran before. That is also the release the migration
# check in deploy-production.yml compared against.
set -eu

newest() {
	kubectl -n "$1" rollout history "$2" | awk '$1 ~ /^[0-9]+$/ { r = $1 } END { print r }'
}

case "${1:-}" in
record)
	[ "$#" -eq 3 ] || { echo "usage: $0 record NAMESPACE KIND/NAME" >&2; exit 2; }
	newest "$2" "$3"
	;;
restore)
	[ "$#" -eq 4 ] || { echo "usage: $0 restore NAMESPACE KIND/NAME RECORDED" >&2; exit 2; }
	ns="$2" target="$3" recorded="$4"
	if [ -z "$recorded" ]; then
		echo "FAIL $ns/$target: no revision was recorded before the deploy; see deploy/RUNBOOK.md" >&2
		exit 1
	fi
	current="$(newest "$ns" "$target")"
	if [ "$current" = "$recorded" ]; then
		echo "$ns/$target: still on revision $recorded, the apply did not change it; nothing to undo"
		exit 0
	fi
	echo "$ns/$target: revision $current -> back to recorded revision $recorded"
	kubectl -n "$ns" rollout undo "$target" --to-revision="$recorded"
	;;
*)
	echo "usage: $0 record|restore ..." >&2
	exit 2
	;;
esac
