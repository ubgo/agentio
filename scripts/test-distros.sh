#!/usr/bin/env bash
# test-distros.sh runs the library on real Linux distributions with Docker:
# every module's tests, `agentinfo -fixtures`, and `agentinfo` against an
# empty and a seeded home (HostFromOS: real /proc, real env, no keychain).
# Manual and slow (it pulls images and downloads modules); `task
# test:distros` runs it. Never part of `task check`.
#
# Tests run without -race: the Alpine (musl) image has no C toolchain and
# the race detector needs cgo. Each distro gets a copy of the worktree, so
# nothing in the repository is written.
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
# The modules pin each other at pushed pseudo-versions of this private
# repository; the containers have no GitHub credentials, so they read those
# (and every public module already downloaded here) from this machine's
# module cache through a read-only file:// proxy, falling back to the
# public proxy for anything missing.
modcache="$(go env GOMODCACHE)/cache/download"
modules=". claude codex codex/state builtin examples"
go_version="${GO_VERSION:-1.27.1}"

# distro name | image | command that installs Go and git when the image
# has none (empty for the golang images)
distros=(
  "debian|golang:${go_version}-bookworm|"
  "alpine|golang:${go_version}-alpine|apk add --no-cache git >/dev/null"
  "fedora|fedora:latest|dnf -y -q install golang git >/dev/null"
  "arch|archlinux:latest|pacman -Sy --noconfirm --needed go git >/dev/null"
)

inner=$(cat <<'INNER'
set -eu
if [ -n "$SETUP" ]; then sh -c "$SETUP"; fi
export GOTOOLCHAIN=auto GOFLAGS=-buildvcs=false
export GOPROXY=file:///modcache,https://proxy.golang.org GONOSUMDB='github.com/ubgo/*' GOPRIVATE=
cp -a /src /work && cd /work
. /etc/os-release
echo "== $PRETTY_NAME, $(go version), libc: $(ldd --version 2>&1 | head -1)"
for m in $MODULES; do
  (cd "$m" && go test -count=1 ./... >/tmp/test.log 2>&1) || { cat /tmp/test.log; echo "FAIL tests $m"; exit 1; }
  echo "ok tests $m"
done
cd examples
go build -o /tmp/agentinfo ./agentinfo
/tmp/agentinfo -fixtures >/tmp/fixtures.txt && grep -q '=== Codex (codex) ===' /tmp/fixtures.txt && echo "ok agentinfo -fixtures"
mkdir -p /tmp/empty
HOME=/tmp/empty /tmp/agentinfo -dir /tmp/empty >/tmp/empty.txt && grep -q 'not installed' /tmp/empty.txt && echo "ok agentinfo empty home"
mkdir -p /tmp/seed
cp -a /work/claude/testdata/homes/2.1.325/userhome/. /tmp/seed/
cp -a /work/codex/testdata/homes/0.162.1 /tmp/seed/.codex
HOME=/tmp/seed /tmp/agentinfo -dir /tmp/seed/project >/tmp/seed.txt
grep -q 'name=personal' /tmp/seed.txt && echo "ok agentinfo seeded home ($(grep -c . /tmp/seed.txt) lines)"
INNER
)

for d in "${distros[@]}"; do
  IFS='|' read -r name image setup <<<"$d"
  start=$(date +%s)
  docker run --rm -v "$repo:/src:ro" -v "$modcache:/modcache:ro" -e SETUP="$setup" -e MODULES="$modules" "$image" sh -c "$inner"
  echo "== $name done in $(( $(date +%s) - start ))s"
done
