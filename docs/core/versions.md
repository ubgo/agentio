# Versions

`agentio.Version` is a vendor CLI version such as `2.1.325`, `0.162.1` or `1.0.0-beta.2`. The library reads it from disk (the npm `package.json`, the native versions folder) or from `--version` output, compares it against the range each vendor module was tested with, and passes it to launch builders so they can check flags. The zero `Version` means unknown, and everything that consumes a version treats unknown as "build and warn", never as a refusal. You want this page to parse, compare or display a version.

## Before you start

- Module: `github.com/ubgo/agentio` (stdlib only).
- Nothing on disk.

## Step 1 — Parse a version

`agentio.ParseVersion(s)` finds the first number in `s` that starts a word and parses it, with an optional `-pre` suffix. It scans instead of parsing strictly because vendor `--version` output wraps the number in product text that changes between releases. It returns `(Version, bool)`; `ok` is false when there is no number. `MustVersion(s)` is for constants known at compile time and yields the zero Version on a typo, never a panic.

<!-- snippet: examples/values/versions.go#parse-version -->
```go
// parseVersions reads the version out of real `--version` output shapes.
// ParseVersion scans for the first number that starts a word, so product
// text around it does not matter and the 64 in "x86_64" is skipped; ok is
// false when there is no such number. Pure.
func parseVersions(p printer) error {
	for _, s := range []string{
		"2.1.325 (Claude Code)",
		"codex-cli 0.162.1",
		"v1.2",
		"1.0.0-beta.2",
		"x86_64 build 7",
		"no version here",
	} {
		v, ok := agentio.ParseVersion(s)
		p.line(fmt.Sprintf("parse %q: ok=%t version=%q major=%d pre=%q", s, ok, v, v.Major, v.Pre))
	}
	// MustVersion is for constants known at compile time; a typo gives the
	// zero (unknown) Version, never a panic.
	p.line(fmt.Sprintf("must: %s typo-is-zero=%t", agentio.MustVersion("2.1.323"), agentio.MustVersion("two").IsZero()))
	return nil
}
```

`Version` has `Major`, `Minor`, `Patch`, `Pre` (the pre-release without the dash) and `Raw` (the text it was parsed from). `String()` renders `M.m.p[-pre]` and is empty for the zero Version; it also implements `MarshalText` / `UnmarshalText`, so it reads and writes as a plain string in JSON and YAML.

## Step 2 — Compare versions

`Compare` orders versions semver-style and returns -1, 0 or +1: a release sorts after its pre-releases, and pre-release parts compare numerically when both are numbers (`beta.2` before `beta.11`). `Raw` is ignored. It fits `slices.SortFunc` directly.

<!-- snippet: examples/values/versions.go#compare-versions -->
```go
// compareVersions orders versions semver-style: a release sorts after its
// pre-releases, numeric pre-release parts compare as numbers. The zero
// Version means "unknown", which callers treat as build-and-warn. Pure.
func compareVersions(p printer) error {
	vs := []agentio.Version{
		agentio.MustVersion("1.0.0"),
		agentio.MustVersion("1.0.0-beta.11"),
		agentio.MustVersion("1.0.0-beta.2"),
		agentio.MustVersion("0.162.1"),
		agentio.MustVersion("1.0.0-alpha"),
	}
	slices.SortFunc(vs, agentio.Version.Compare)
	p.line(fmt.Sprintf("sorted: %v", vs))
	installed, minimum := agentio.MustVersion("2.1.325"), agentio.MustVersion("2.1.300")
	p.line(fmt.Sprintf("compare: 2.1.325 vs 2.1.300 = %d (at least minimum: %t)", installed.Compare(minimum), installed.Compare(minimum) >= 0))
	var unknown agentio.Version
	p.line(fmt.Sprintf("unknown: IsZero=%t String=%q", unknown.IsZero(), unknown.String()))
	return nil
}
```

To check an installed version against the range a vendor module was tested with, use `Compat.Check` and `Compat.Warning` ([Tested vendor versions](../compat/compat.md)); to check one flag or file, use `FeatureSpan.In` ([Per-feature version table](../compat/features.md)).

## Try it

```sh
cd examples && go run ./values
```

The lines for this page:

```text
parse "2.1.325 (Claude Code)": ok=true version="2.1.325" major=2 pre=""
parse "codex-cli 0.162.1": ok=true version="0.162.1" major=0 pre=""
parse "v1.2": ok=true version="1.2.0" major=1 pre=""
parse "1.0.0-beta.2": ok=true version="1.0.0-beta.2" major=1 pre="beta.2"
parse "x86_64 build 7": ok=true version="7.0.0" major=7 pre=""
parse "no version here": ok=false version="" major=0 pre=""
must: 2.1.323 typo-is-zero=true
sorted: [0.162.1 1.0.0-alpha 1.0.0-beta.2 1.0.0-beta.11 1.0.0]
compare: 2.1.325 vs 2.1.300 = 1 (at least minimum: true)
unknown: IsZero=true String=""
```

## Pitfalls

- **`ParseVersion` takes the first number that starts a word.** A number inside a word is skipped: the `86` and `64` in `x86_64`, the `1` in `dev1.2`, the `18` in `node-18` (a digit after a letter, digit, `_`, `.`, `-` or `+` is inside the word; a leading `v` is allowed). So `x86_64 build 7` parses as `7.0.0`. Pass it the version text itself, not a whole line of unrelated output.
- **Missing parts are zero.** `v1.2` is `1.2.0`; the `v` prefix is accepted.
- **The zero Version is "unknown", not `0.0.0`.** `IsZero()` is true and `String()` is `""`. A real `0.0.0` cannot be told apart, which no vendor ships.
- **Vendor `ParseVersion` has a different signature.** `claude.ParseVersion` and `codex.ParseVersion` return `(agentio.Version, error)`, the root function returns `(Version, bool)`.
- **Unknown never refuses.** `Compat.Check` gives `CompatUnknown`, `FeatureSpan.In` gives true, and launch builders add a `version-unknown` warning and build anyway.
- **`UnmarshalText` never fails.** Text without a version decodes to the zero Version, so a bad value in a config file silently becomes "unknown"; check `IsZero()` when the version is required.

## Related

- [Optional values, unknown keys and warnings](values.md)
- [Tested vendor versions](../compat/compat.md)
- [Per-feature version table](../compat/features.md)
- [Install, version and identity](../capabilities/install-identity.md)
- Example: [`examples/values`](../../examples/values/)
- [Feature index](../reference/features.md)
