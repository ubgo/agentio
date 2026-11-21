#!/usr/bin/env bash
# claude-bump.sh does the mechanical part of supporting a new Claude Code
# release (docs/compat/drift.md, "Release routine"):
#
#   1. fetches the Claude Code CHANGELOG entries after the current TestedMax
#      up to the target version and saves them;
#   2. saves `claude --version` and `claude --help` of the installed binary
#      and diffs the help text with the previous saved one;
#   3. runs the drift check on this machine's real homes (read-only) against
#      the previous saved report, then saves the full report as the next
#      baseline;
#   4. prints a short report: new findings, help changes, and the env vars,
#      flags and settings keys the CHANGELOG names that the library does not
#      know yet, plus the CHANGELOG lines on files and behaviour agentio
#      reads (transcripts, settings, hooks, --resume, credentials, ...);
#   5. with --apply: raises claude.TestedMax, creates the fixture home
#      claude/testdata/homes/<version> from the previous one (version strings
#      moved, staged with `git add -f` so a global gitignore cannot drop a
#      file), moves the tested range in docs and example outputs, and runs
#      the claude and examples tests.
#
# Cost: Network (one GET of the public CHANGELOG), Process (`claude
# --version` and `claude --help` only; neither touches the login), Disk
# Read of the real homes (drift check). It never spends quota and writes
# nothing outside the saved-reports directory unless --apply is given.
# Reports name files, keys and flags only, never a value; they still name
# your home directories, so keep the directory outside the repository.
#
# Modelling what changed (readers, feature rows, fixture records), the
# CHANGELOG entry, the release tags and the downstream bumps stay human
# steps; the checklist is in docs/compat/drift.md.
set -euo pipefail

usage() {
  cat <<'EOF'
usage: scripts/claude-bump.sh [--apply] [--from VERSION] [--dir DIR] [--changelog FILE] VERSION

  VERSION          the Claude Code version to support, e.g. 2.1.330
  --apply          also raise TestedMax, create the fixture home, move the
                   tested range in docs and example outputs, run the tests
  --from VERSION   the previous tested version (default: claude.TestedMax)
  --dir DIR        where reports are saved (default: $AGENTIO_DRIFT_DIR,
                   else docsi/drift in the repository, which is gitignored)
  --changelog FILE read the CHANGELOG from FILE instead of fetching it
EOF
}

repo="$(cd "$(dirname "$0")/.." && pwd)"
changelog_url="https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md"
apply=0
from=""
dir="${AGENTIO_DRIFT_DIR:-}"
changelog_file=""
version=""

while [ $# -gt 0 ]; do
  case "$1" in
    --apply) apply=1 ;;
    --from) from="$2"; shift ;;
    --dir) dir="$2"; shift ;;
    --changelog) changelog_file="$2"; shift ;;
    -h | --help) usage; exit 0 ;;
    -*) echo "unknown flag: $1" >&2; usage >&2; exit 2 ;;
    *) version="$1" ;;
  esac
  shift
done

semver='^[0-9]+\.[0-9]+\.[0-9]+$'
if ! [[ "$version" =~ $semver ]]; then
  usage >&2
  exit 2
fi

features_go="$repo/claude/features.go"
tested_min="$(sed -n 's/^[[:space:]]*TestedMin = "\(.*\)"/\1/p' "$features_go")"
tested_max="$(sed -n 's/^[[:space:]]*TestedMax = "\(.*\)"/\1/p' "$features_go")"
# vle A B: true when version A <= version B.
vle() { [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n1)" = "$1" ]; }

dir="${dir:-$repo/docsi/drift}"
mkdir -p "$dir"
log="$dir/claude-changelog-$version.md"
help_new="$dir/claude-help-$version.txt"
base_new="$dir/claude-$version.tsv"
findings="$dir/claude-$version-new.tsv"

# newest saved file of a kind (claude-<ver>.tsv, claude-help-<ver>.txt)
# whose version is below the target.
previous() {
  local prefix="$1" suffix="$2" best="" bestv="" v f
  for f in "$dir/$prefix"*"$suffix"; do
    [ -e "$f" ] || continue
    v="${f##*/"$prefix"}"
    v="${v%"$suffix"}"
    [[ "$v" =~ $semver ]] || continue
    vle "$version" "$v" && continue
    if [ -z "$bestv" ] || vle "$bestv" "$v"; then best="$f" bestv="$v"; fi
  done
  echo "$best"
}

# The previous version defaults to TestedMax. After --apply has raised it
# to the target, a rerun starts from the newest saved report instead.
if [ -z "$from" ] && [ "$version" = "$tested_max" ]; then
  from="$(previous claude- .tsv)"
  from="${from##*/claude-}"
  from="${from%.tsv}"
fi
from="${from:-$tested_max}"
if ! [[ "$from" =~ $semver ]]; then
  echo "cannot tell the previous version (got '$from'); pass --from" >&2
  exit 2
fi
if vle "$version" "$from"; then
  echo "target $version is not newer than $from" >&2
  exit 2
fi

echo "== Claude Code $from -> $version (tested $tested_min..$tested_max), reports in $dir"

# 1. CHANGELOG entries in (from, version].
if [ -n "$changelog_file" ]; then
  cp "$changelog_file" "$dir/.changelog-full.md"
else
  curl -fsSL "$changelog_url" -o "$dir/.changelog-full.md"
fi
awk -v from="$from" -v to="$version" '
  function cmp(a, b,   x, y, i) {
    split(a, x, "."); split(b, y, ".")
    for (i = 1; i <= 3; i++) { if (x[i] + 0 < y[i] + 0) return -1; if (x[i] + 0 > y[i] + 0) return 1 }
    return 0
  }
  /^## [0-9]+\.[0-9]+\.[0-9]+/ { v = $2; keep = cmp(v, from) > 0 && cmp(v, to) <= 0 }
  keep { print }
' "$dir/.changelog-full.md" > "$log"
rm -f "$dir/.changelog-full.md"
entries="$(grep -c '^## ' "$log" || true)"
echo "changelog: $entries entr$([ "$entries" = 1 ] && echo y || echo ies) saved to $log"
if [ "$entries" = 0 ]; then
  echo "  (no entry for $version yet; the report below has no CHANGELOG part)"
fi

# 2. Installed binary: version and help text.
installed=""
if command -v claude >/dev/null; then
  installed="$(claude --version 2>/dev/null | awk '{print $1}')"
  claude --help > "$help_new" 2>&1 || true
fi
if [ -z "$installed" ]; then
  echo "claude: not on PATH; help diff and drift on real homes skipped" >&2
elif [ "$installed" != "$version" ]; then
  echo "WARNING: installed claude is $installed, not $version; the help text and drift findings describe $installed"
fi

help_old="$(previous claude-help- .txt)"
echo
echo "== claude --help ($(basename "${help_old:-none}") -> $(basename "$help_new"))"
if [ -n "$installed" ] && [ -n "$help_old" ]; then
  diff "$help_old" "$help_new" && echo "unchanged"
else
  echo "nothing to compare"
fi

# 3. Drift check against the previous baseline, then the full report as the
# next baseline. Both runs read the same homes; neither writes to them.
base_old="$(previous claude- .tsv)"
echo
echo "== drift check (baseline $(basename "${base_old:-none}"))"
drift_args=()
[ -n "$base_old" ] && drift_args+=(-baseline "$base_old")
[ -s "$help_new" ] && drift_args+=(-claude-help "$help_new")
(cd "$repo/examples" && go run ./drift "${drift_args[@]+"${drift_args[@]}"}") > "$findings"
(cd "$repo/examples" && go run ./drift) > "$base_new"
grep -E '^(claude|codex)(:| home|	)|^  transcripts|distinct findings' "$findings" | grep -v '^codex	' || true
echo "new findings: $findings; next baseline: $base_new"

# 4. What the CHANGELOG names that the library does not know.
known_env="$(cat "$repo/claude/env.go" "$repo/claude/features.tsv" "$repo/claude/constants.go" 2>/dev/null)"
known_go="$(cat "$repo"/claude/*.go)"
echo
echo "== CHANGELOG names not known to the library"
echo "-- env vars (not in KnownEnv, features.tsv or constants):"
grep -oE '\b(CLAUDE|ANTHROPIC|DISABLE|ENABLE|MCP|BASH|SELF_HOSTED|OTEL)_[A-Z0-9_]+' "$log" | sort -u |
  while read -r e; do grep -q "\"$e\"\|	$e	" <<<"$known_env" || echo "  $e"; done
echo "-- flags (not in features.tsv; [help] = in claude --help):"
grep -oE '(^|[^A-Za-z0-9-])--[a-z][a-z0-9-]+' "$log" | sed 's/^[^-]*//' | sort -u |
  while read -r f; do
    grep -q -- "$f\b" "$repo/claude/features.tsv" && continue
    if [ -s "$help_new" ] && grep -q -- "$f\b" "$help_new"; then echo "  $f [help]"; else echo "  $f"; fi
  done
echo "-- settings and JSON keys (camelCase in backticks, no json tag in claude/):"
bt='`'
grep -oE "${bt}[a-z]+[A-Z][A-Za-z0-9]*${bt}" "$log" | tr -d "$bt" | sort -u |
  while read -r k; do grep -q "json:\"${k}[,\"]" <<<"$known_go" || echo "  $k"; done
echo "-- lines on what agentio reads (transcripts, settings, hooks, resume, login, processes, rate limits):"
grep -inE 'transcript|session file|settings\.json|managed settings|\.claude\.json|hook|--resume|--continue|apiKeyHelper|setup-token|credential|keychain|oauth|rate.limit|ratelimit|process name|CLAUDE_CONFIG_DIR' "$log" |
  sed 's/^/  /' || echo "  none"

# 5. --apply: the mechanical edits.
if [ "$apply" = 1 ]; then
  echo
  echo "== apply: TestedMax $tested_max -> $version"
  cd "$repo"
  if [ "$tested_max" != "$version" ]; then
    sed -i.bak "s/TestedMax = \"$tested_max\"/TestedMax = \"$version\"/" "$features_go" && rm -f "$features_go.bak"
  fi

  old_fx="claude/testdata/homes/$from"
  new_fx="claude/testdata/homes/$version"
  if [ -e "$new_fx" ]; then
    echo "fixture $new_fx exists; left as it is"
  elif [ -z "$(git ls-files "$old_fx" | head -n1)" ]; then
    echo "no tracked fixture at $old_fx; create $new_fx by hand" >&2
  else
    # git archive copies the tracked files only (force-added ones included),
    # never an untracked or ignored leftover of the working tree.
    tmp="$(mktemp -d)"
    git archive HEAD "$old_fx" | tar -x -C "$tmp"
    mv "$tmp/$old_fx" "$new_fx"
    rm -rf "$tmp"
    grep -rlF "$from" "$new_fx" | while read -r f; do
      sed -i.bak "s/${from//./\\.}/$version/g" "$f" && rm -f "$f.bak"
    done
    git add -f "$new_fx"
    echo "fixture $new_fx: copied from $old_fx, version strings moved, staged ($(git ls-files "$new_fx" | wc -l | tr -d ' ') files)"
    echo "  add a record or key with made-up values for every new shape the drift check found"
  fi

  # The tested range as docs and example outputs print it, and TestedMax
  # as the examples print it ("check \"X\"", "\"X (Claude Code)", "Max: X").
  old_range="$tested_min..$tested_max"
  new_range="$tested_min..$version"
  esc() { printf '%s' "${1//./\\.}"; }
  git ls-files '*.go' '*.md' | grep -v '^CHANGELOG.md$' | grep -v '/testdata/' |
    xargs grep -lF "$old_range" 2>/dev/null | while read -r f; do
      sed -i.bak "s/$(esc "$old_range")/$new_range/g" "$f" && rm -f "$f.bak" && echo "  range: $f"
    done
  git ls-files 'examples/*' 'docs/*' | grep -E '\.(go|md)$' |
    xargs grep -lE "\"$(esc "$tested_max")|Max: $(esc "$tested_max")" 2>/dev/null | while read -r f; do
      sed -i.bak -e "s/\"$(esc "$tested_max")/\"$version/g" -e "s/Max: $(esc "$tested_max")/Max: $version/g" "$f" &&
        rm -f "$f.bak" && echo "  printed TestedMax: $f"
    done

  echo
  echo "== apply: references to $tested_max left (review: keep the ones about that version)"
  git grep -nF "$tested_max" -- ':!CHANGELOG.md' ':!.ds' ':!claude/testdata' || echo "  none"

  echo
  echo "== apply: tests"
  (cd claude && go test -count=1 ./...) && (cd examples && go test -count=1 ./...)
  echo
  echo "next: model the findings, add feature rows (features.tsv + go generate ./claude/...), a fixture test, the CHANGELOG entry; task docs:snippets; task check; ds ack what docsync flags"
fi
