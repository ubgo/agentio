# Tested vendor versions

Each vendor module states the range of vendor versions it was tested against: [`claude.TestedMin`](ds:block?id=claude-testedmin-8f2tgsg2)..`claude.TestedMax` for Claude Code and [`codex.TestedMin`](ds:block?id=codex-testedmin-vxn77phg)..`codex.TestedMax` for Codex. Use the range to tell a user "this Claude Code is newer than the library has seen" before you trust every field, or to show the status in a doctor screen. [The library never refuses to work outside the range](ds:block?id=claude-versionwarnings-82bmgcbc): it builds and reads as usual and attaches a warning.

## Before you start

- Modules `github.com/ubgo/agentio` plus `claude` and/or `codex`. Everything on this page is Pure (no disk, no process, no network) except reading the installed version, which comes from [`Install()`](ds:block?id=claude-agent-install-472s4jn6) (Disk · Read) or a Runner's [`BinaryVersion`](ds:block?id=claude-runner-binaryversion-p4863j5t) (Process · Read).
- Versions are `agentio.Version` values; see [Versions](../core/versions.md) for parsing and comparing.
- The code is [examples/compat/main.go](../../examples/compat/main.go).

## Step 1 — Read the tested range

`TestedMin` and `TestedMax` are string constants; [`claude.Compat()`](ds:block?id=claude-compat-ngsh97c9) and [`codex.Compat()`](ds:block?id=codex-compat-s6qau3qq) parse them into an `agentio.Compat{Min, Max}`. A range moves only when a fixture home for a new vendor version is added to the module's `testdata/homes` (see [Fixture homes](../testing/fixtures.md)), so the range is exactly the set of versions whose real files the readers were tested on.

<!-- snippet: examples/compat/main.go#ranges -->
```go
// printRanges prints the tested range of each vendor module. TestedMin and
// TestedMax are string constants; Compat() parses them into versions.
func printRanges(w io.Writer) {
	fmt.Fprintf(w, "claude tested %s..%s\n", claude.TestedMin, claude.TestedMax)
	fmt.Fprintf(w, "codex  tested %s..%s\n", codex.TestedMin, codex.TestedMax)
	c := claude.Compat()
	fmt.Fprintf(w, "claude.Compat() = {Min: %s, Max: %s}\n", c.Min, c.Max)
}
```

## Step 2 — Check the installed version

[`Compat.Check(v)`](ds:block?id=compat-check-yqab46pa) places a version against the range and returns a `CompatStatus`: `CompatInRange`, `CompatNewer`, `CompatOlder`, or `CompatUnknown` for a zero (unknown) version or an unset range. [`Compat.Warning(v)`](ds:block?id=compat-warning-yeqfn6xw) returns the `agentio.Warning` the launch builders attach to `Command.Warnings` for the same case: `untested-version` outside the range, `version-unknown` for an unknown version, and nothing inside it.

<!-- snippet: examples/compat/main.go#check -->
```go
// printChecks places a few installed versions against the Claude range.
// Outside the range is a warning, never a refusal; an unknown (zero)
// version is CompatUnknown.
func printChecks(w io.Writer) {
	c := claude.Compat()
	for _, s := range []string{claude.TestedMax, claudeNewer, claudeOld, unknownVersion} {
		// ok is ignored on purpose: unknownVersion fails to parse and
		// yields the zero Version, which is exactly the case shown.
		v, _ := agentio.ParseVersion(s)
		status := c.Check(v)
		line := fmt.Sprintf("check %-9q -> %s", s, status)
		if warn, ok := c.Warning(v); ok {
			line += fmt.Sprintf(" (warning %s)", warn.Kind)
		}
		fmt.Fprintln(w, line)
	}
}
```

In a real program the version comes from the install: `inst, err := a.Install()` then `claude.Compat().Check(inst.Version)`. The [drift check](drift.md) [prints exactly that line for each vendor](ds:block?id=examples-drift-versionline-4gjggnvh).

## Try it

```sh
cd examples && go run ./compat
```

```text
claude tested 2.1.323..2.1.330
codex  tested 0.162.1..0.162.1
claude.Compat() = {Min: 2.1.323, Max: 2.1.330}
check "2.1.330" -> in-range
check "2.1.400" -> newer (warning untested-version)
check "2.1.40"  -> older (warning untested-version)
check ""        -> unknown (warning version-unknown)
claude --from-pr                    since 2.1.27   Claude Code CHANGELOG.md 2.1.27
claude --worktree                   since 2.1.49   Claude Code CHANGELOG.md 2.1.49
claude --name                       since 2.1.76   Claude Code CHANGELOG.md 2.1.76
claude --plugin-dir .zip            since 2.1.128  Claude Code CHANGELOG.md 2.1.128 (behaviour)
claude --fallback-model interactive since 2.1.166  Claude Code CHANGELOG.md 2.1.166 (behaviour; print-only before)
claude --restricted                 since 2.1.248  Claude Code CHANGELOG.md 2.1.248
claude mcp add-json -               since 2.1.327  Claude Code CHANGELOG.md 2.1.327 (server JSON from stdin; MCPAddCommand still passes it on argv)
claude mcp_tool hook onFailure      since 2.1.328  Claude Code CHANGELOG.md 2.1.328 ("onFailure": "block" for mcp_tool hooks, as command and http hooks had)
claude CLAUDE_CODE_DISABLE_DEFERRED_TOOL_RESUME since 2.1.328  Claude Code CHANGELOG.md 2.1.328 (stops claude -p --resume / --continue from running a tool call a hook deferred)
claude CLAUDE_CODE_FULL_SIGN_IN_LINK since 2.1.330  Claude Code CHANGELOG.md 2.1.330 (=1 prints the full sign-in link of /login, claude auth login and claude setup-token instead of the short one)
claude features: 71 rows, 23 with a known version
codex features: 39 rows, 9 with a known version
--worktree in "2.1.40" : false
--worktree in "2.1.330": true
--worktree in ""       : true
claude launch: claude: --worktree needs Claude Code >= 2.1.49 (installed 2.1.40)
  feature=--worktree installed=2.1.40 since=2.1.49 is ErrFeatureUnavailable=true
claude 2.1.400: built, untested warning=true
codex launch: codex 0.150.0: --no-daemon needs codex >= 0.156.0
```

The first seven lines are this page; the rest is the [per-feature version table](features.md).

## Pitfalls

- [Outside the range is a warning, never a refusal.](ds:block?id=compat-check-yqab46pa) Do not turn `CompatNewer` into an error: a newer vendor usually works, and the readers keep unknown keys in typed `Unknown` fields instead of dropping them.
- An older version is reported with the same [`untested-version`](ds:block?id=compat-warning-yeqfn6xw) warning as a newer one; branch on the `CompatStatus` (`CompatOlder` vs `CompatNewer`) when the direction matters.
- A zero `agentio.Version` means unknown, and `Check` returns [`CompatUnknown`](ds:block?id=compat-check-yqab46pa) for it. [`Home.Target()`](ds:block?id=home-target-heq5sahh) leaves `Target.Version` zero; set it from `Install()` if you want launch warnings and feature checks to use the real version.
- The Codex range is a single version today ([`0.162.1..0.162.1`](ds:block?id=codex-testedmin-vxn77phg)): only one Codex fixture home exists, so every other Codex version is untested.
- Three `ParseVersion` functions exist with the same shape: `agentio.ParseVersion(s)`, [`claude.ParseVersion`](ds:block?id=claude-parseversion-n4kvb682) and `codex.ParseVersion` all return `(agentio.Version, bool)` (the vendor ones call the root function), so parsing text with no version is not an error.
- `TestedMin`/`TestedMax` are per module, not per feature. A flag can need a newer version than `TestedMin`; check flags with the [per-feature table](features.md).

## Related

- [Per-feature version table](features.md): which flags exist in which versions.
- [Drift check](drift.md): what changed in files of a version outside the range.
- [Versions](../core/versions.md) and [Optional values, unknown keys and warnings](../core/values.md).
- [Fixture homes](../testing/fixtures.md): why the range moves only with a fixture.
- [examples/compat](../../examples/compat/) and the [feature index](../reference/features.md#compat-and-drift).
- [Choosing guides](../choosing/README.md).
