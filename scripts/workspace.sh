#!/usr/bin/env bash
# workspace.sh writes a go.work over every module of the repository so the
# modules resolve each other from this checkout (a CI runner, a container),
# never from their pinned versions on the module proxy, which lag behind the
# working tree.
#
# `use` alone is not enough: workspace mode still loads the go.mod of the
# pinned pseudo-version each module requires of its siblings (a network
# fetch, which failed while the repository was private). A version-specific
# replace of every pinned sibling version to its local directory stops
# that fetch. go.work stays untracked; the standalone (GOWORK=off) builds
# in `task check` keep proving the pinned versions themselves.
set -euo pipefail
cd "$(dirname "$0")/.."
modules=(. ./claude ./codex ./codex/state ./builtin ./examples)
rm -f go.work go.work.sum
go work init "${modules[@]}"
for m in "${modules[@]}"; do cat "$m/go.mod"; done |
  grep -oE 'github\.com/ubgo/agentio(/[a-z/]+)? v[^ ]+' | sort -u |
  while read -r mod ver; do
    go work edit -replace "$mod@$ver=.${mod#github.com/ubgo/agentio}"
  done
echo "go.work: ${#modules[@]} modules, $(grep -c '^replace' go.work) pinned sibling versions replaced locally"
