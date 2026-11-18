# Per-feature version table

Vendor CLIs add flags and files over time, so a launch that is valid on today's Claude Code or Codex can be invalid on the version a user has installed. Each vendor module carries a table of version-gated features (flags, files and behaviours) with the version each appeared in, generated from a `features.tsv` file. The launch builders consult it: a flag the installed version provably lacks returns a typed [`FeatureUnavailableError`](ds:block?id=claude-checkfeature-am2yhjzp) instead of a command that would fail at run time. Use the table directly to hide options in a UI, or to explain an error to a user.

## Before you start

- Modules `github.com/ubgo/agentio` plus `claude` and/or `codex`. Every call on this page is Pure: the table is compiled into the module.
- The installed version, usually from `Install()`; see [Tested vendor versions](compat.md#step-2--check-the-installed-version).
- The code is [examples/compat/main.go](../../examples/compat/main.go).

## Step 1 — List features

`claude.Feature` and `codex.Feature` are small integer enums; [`String()`](ds:block?id=claude-feature-string-4gkzaspt) returns the vendor spelling (`--worktree`, `CLAUDE_CONFIG_DIR`, `<name>.config.toml`). List them with [`claude.Features()`](ds:block?id=claude-features-ej42vjsy) and [`codex.Features()`](ds:block?id=codex-features-sgjqr7gr) (one name for both vendors). [`FeatureInfo(f)`](ds:block?id=claude-featureinfo-7cywwf7k) returns the `agentio.FeatureSpan` and [`FeatureSource(f)`](ds:block?id=claude-featuresource-fcy46dbg) names the vendor changelog or release note line behind the bounds.

The table source is `claude/features.tsv` and `codex/features.tsv`, one row per feature with the columns `name`, `spelling`, `since`, `until`, `source`; `go generate` turns it into `features_gen.go`. A bound is filled only when an explicit vendor changelog or release-note line was found, and the row then names it in `source`; otherwise the bound stays empty. Many rows have no bounds at all: they are flags the builders emit, listed so every emitted flag has a row, but they cannot be gated.

<!-- snippet: examples/compat/main.go#features -->
```go
// printFeatures counts the rows of both feature tables and prints the
// Claude rows gated since listFrom. A row with no known bound is a flag the
// builder emits but cannot gate.
func printFeatures(w io.Writer) {
	gated, total := 0, 0
	for _, f := range claude.Features() {
		total++
		span := claude.FeatureInfo(f)
		if !span.Known() {
			continue
		}
		gated++
		if span.Since.Compare(agentio.MustVersion(listFrom)) >= 0 {
			fmt.Fprintf(w, "claude %-28s since %-8s %s\n", f, span.Since, claude.FeatureSource(f))
		}
	}
	fmt.Fprintf(w, "claude features: %d rows, %d with a known version\n", total, gated)
	gated, total = 0, 0
	for _, f := range codex.Features() {
		total++
		if codex.FeatureInfo(f).Known() {
			gated++
		}
	}
	fmt.Fprintf(w, "codex features: %d rows, %d with a known version\n", total, gated)
}
```

## Step 2 — Check a feature against a version

[`FeatureSpan{Since, Until}`](ds:block?id=featurespan-ckruwege) means "available from `Since` (inclusive) until `Until` (exclusive)", where a zero bound is unknown. [`span.Known()`](ds:block?id=featurespan-known-h6gygtav) reports whether either bound is set. [`span.In(v)`](ds:block?id=featurespan-in-vh7rmz94) is true unless `v` is provably before `Since` or at/after `Until`; an unknown (zero) `v` is always in. The rule is "build and warn, never refuse on a guess": the library only refuses when it is sure.

<!-- snippet: examples/compat/main.go#spans -->
```go
// printSpans asks one feature's span about three versions. In is true
// unless the version is provably outside the span; an unknown version is
// always in (build and warn, never refuse on a guess).
func printSpans(w io.Writer) {
	span := claude.FeatureInfo(claude.FeatureFlagWorktree)
	for _, s := range []string{claudeOld, claude.TestedMax, unknownVersion} {
		v, _ := agentio.ParseVersion(s) // zero Version for unknownVersion, on purpose
		fmt.Fprintf(w, "%s in %-9q: %t\n", claude.FeatureFlagWorktree, s, span.In(v))
	}
}
```

## Step 3 — Handle FeatureUnavailableError

The typed launch builders ([`claude.Agent.Launch`](ds:block?id=claude-agent-launch-r7fsfwtp), `codex.Agent.New`, [`codex.Agent.Exec`](ds:block?id=codex-builder-flag-f2rrxpyh) and friends) check every flag they emit against `Target.Version`. When the version provably lacks one, they return [`*claude.FeatureUnavailableError`](ds:block?id=claude-featureunavailableerror-c76rq36g) or [`*codex.FeatureUnavailableError`](ds:block?id=codex-featureunavailableerror-vhup5my8) with `Feature`, `Installed`, `Since` and `Until`. Both match [`agentio.ErrFeatureUnavailable`](ds:block?id=claude-featureunavailableerror-is-anzr93zz) with `errors.Is`, so code that handles several agents needs one branch; use `errors.As` when you want the fields for a message. With a newer, untested version the builders build the command and attach an [`untested-version`](ds:block?id=claude-versionwarnings-82bmgcbc) warning instead.

<!-- snippet: examples/compat/main.go#gates -->
```go
// launchGates builds a launch that asks for a version-gated flag with an
// old Target.Version. The builder refuses with a typed error that matches
// agentio.ErrFeatureUnavailable; with a newer, untested version it builds
// the command and attaches a warning instead.
func launchGates(w io.Writer) error {
	h := exampleHost()

	ca := claude.New(h)
	t := ca.DefaultHome().Target()
	t.Version = agentio.MustVersion(claudeOld)
	worktree := ""
	_, err := ca.Launch(t, claude.LaunchOptions{Worktree: &worktree})
	var cerr *claude.FeatureUnavailableError
	if !errors.As(err, &cerr) {
		return fmt.Errorf("claude launch on %s: want FeatureUnavailableError, got %v", claudeOld, err)
	}
	fmt.Fprintf(w, "claude launch: %v\n", err)
	fmt.Fprintf(w, "  feature=%s installed=%s since=%s is ErrFeatureUnavailable=%t\n",
		cerr.Feature, cerr.Installed, cerr.Since, errors.Is(err, agentio.ErrFeatureUnavailable))

	t.Version = agentio.MustVersion(claudeNewer)
	cmd, err := ca.Launch(t, claude.LaunchOptions{Worktree: &worktree})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "claude %s: built, untested warning=%t\n", claudeNewer, cmd.Warnings.Has(agentio.WarnUntestedVersion))

	xa := codex.New(h)
	xt := xa.DefaultHome().Target()
	xt.Version = agentio.MustVersion(codexOld)
	_, err = xa.New(xt, codex.LaunchOptions{NoDaemon: true})
	var xerr *codex.FeatureUnavailableError
	if !errors.As(err, &xerr) {
		return fmt.Errorf("codex launch on %s: want FeatureUnavailableError, got %v", codexOld, err)
	}
	fmt.Fprintf(w, "codex launch: %v\n", err)
	return nil
}
```

The vendor launch pages show the same error in the context of a full launch: [Claude launch](../claude/launch.md#step-4--handle-unavailable-features) and [Codex launch](../codex/launch.md#step-4--handle-unavailable-features).

## Try it

```sh
cd examples && go run ./compat | tail -10
```

```text
claude --restricted                 since 2.1.248  Claude Code CHANGELOG.md 2.1.248
claude mcp add-json -               since 2.1.327  Claude Code CHANGELOG.md 2.1.327 (server JSON from stdin; MCPAddCommand still passes it on argv)
claude features: 68 rows, 20 with a known version
codex features: 39 rows, 9 with a known version
--worktree in "2.1.40" : false
--worktree in "2.1.325": true
--worktree in ""       : true
claude launch: claude: --worktree needs Claude Code >= 2.1.49 (installed 2.1.40)
  feature=--worktree installed=2.1.40 since=2.1.49 is ErrFeatureUnavailable=true
claude 2.1.400: built, untested warning=true
codex launch: codex 0.150.0: --no-daemon needs codex >= 0.156.0
```

## Pitfalls

- [`codex.Features()`](ds:block?id=codex-features-sgjqr7gr) (the version-gated launch features) is not [`codex.Runner.Features`](ds:block?id=codex-runner-features-7eabzur3) (the `codex features list` runtime switches); the package function lists this table, the Runner method runs the binary.
- A feature with no known bound is never gated: [`In`](ds:block?id=featurespan-in-vh7rmz94) is true for every version. Absence of an error does not prove the installed CLI has the flag, only that the library cannot prove it lacks it.
- `Target.Version` defaults to zero (unknown), and an unknown version passes every check (with a [`version-unknown`](ds:block?id=claude-versionwarnings-82bmgcbc) warning on the command). Set `Target.Version` from the install if you want the gate to work.
- The error texts differ by vendor ([`claude: --worktree needs Claude Code >= 2.1.49 (installed 2.1.40)`](ds:block?id=claude-featureunavailableerror-error-duumjrvf) vs [`codex 0.150.0: --no-daemon needs codex >= 0.156.0`](ds:block?id=codex-featureunavailableerror-error-mhyfpqrm)). Branch on `errors.Is(err, agentio.ErrFeatureUnavailable)` or `errors.As`, never on the message.
- The neutral [`agentio.Agent.Launch`](ds:block?id=claude-neutral-launch-q63vggs3) goes through the same builders, so it returns the same typed errors; `errors.Is` with the neutral sentinel works on both paths.
- `features.tsv` lists only features the library emits or reads, not every vendor flag. The [drift check](drift.md#step-3--compare-with-a-baseline) can compare a saved `claude --help` against it to find candidate rows.
- Do not edit `features_gen.go`; edit `features.tsv` and run `go generate` in the vendor module.

## Related

- [Tested vendor versions](compat.md): the module-wide range.
- [Claude launch](../claude/launch.md#step-4--handle-unavailable-features) and [Codex launch](../codex/launch.md#step-4--handle-unavailable-features).
- [Errors](../core/errors.md): `ErrFeatureUnavailable` and the other sentinels.
- [Drift check](drift.md): flags missing from the table.
- [examples/compat](../../examples/compat/) and the [feature index](../reference/features.md#compat-and-drift).
- [Choosing guides](../choosing/README.md).
