# Drift check

Vendor CLIs change their files without notice: a new key in `settings.json`, a new permission mode, a new transcript record type. agentio never drops what it does not know (unknown keys land in typed `Unknown` fields, unknown enum spellings keep their text), and the drift check turns that into a report: it decodes every modelled file of every Claude Code and Codex home on a machine with the library's own readers and lists what the library does not model yet. Run it after a vendor release, before moving the [tested range](compat.md), or whenever a user reports "a field is missing".

## Before you start

- The check is the example program [examples/drift](../../examples/drift/) in the `examples` module; it is not part of the library API.
- Cost: Disk · Read plus one process-table read (live sessions). It never runs a vendor binary, never calls the network and never spends quota; the only secret-store read is Codex [`Identity`](ds:block?id=examples-drift-codexdrift-xspx77gd) on a home whose login is kept in the OS keyring.
- Output names files, keys and enum spellings only, never a value: map keys (project paths, server names) print as [`{}`](ds:block?id=examples-drift-walker-walk-22f5frvq) and string fields are never printed, so a report is safe to paste into an issue.

## Step 1 — Run the drift check

`task drift` (or `cd examples && go run ./drift`) checks the real homes of whoever runs it, through [`agentio.HostFromOS()`](ds:block?id=examples-drift-pickhost-k35cnr56). [`-fixtures`](ds:block?id=examples-drift-fixturehost-sm5ek39g) checks the fixture homes in this repository instead, which gives the same report on every machine. Both paths call the same `run` with a different Host:

<!-- snippet: examples/drift/main.go#pickhost -->
```go
// pickHost returns the real machine (the default: `task drift` checks the
// homes of whoever runs it) or a Host over the repository fixtures.
func pickHost(fixtures bool) (agentio.Host, error) {
	if !fixtures {
		return agentio.HostFromOS()
	}
	return fixtureHost(repoFromExamples)
}

// fixtureHost builds a Host over the fixture homes under repoRoot: the
// Claude fixture is a whole user home, the Codex fixture one home selected
// with CODEX_HOME. Processes is nil, so live sessions are read from their
// files without a process table; nothing else here reaches Secrets, HTTP
// or Exec.
func fixtureHost(repoRoot string) (agentio.Host, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return agentio.Host{}, err
	}
	home := filepath.Join(root, claudeFixture)
	return agentio.Host{
		UserHome: home,
		User:     fixtureUser,
		Env:      []string{"HOME=" + home, fixtureNoBinaries, codex.EnvHome + "=" + filepath.Join(root, codexFixture)},
		Now:      fixtureClock,
	}, nil
}
```

<!-- snippet: examples/drift/main.go#run -->
```go
// options are the optional inputs of one run.
type options struct {
	// helpPath is a saved `claude --help` text; "" skips the flag check.
	helpPath string
	// basePath is a saved earlier report; "" lists every finding.
	basePath string
}

// run checks every Claude and Codex home host can see and writes the
// report to out. host is a parameter so a test runs the check on fixture
// homes; main passes HostFromOS() unless -fixtures is given.
func run(ctx context.Context, host agentio.Host, opt options, out io.Writer) error {
	r := &report{}
	if opt.basePath != "" {
		if err := r.loadBaseline(opt.basePath); err != nil {
			return err
		}
	}
	if err := claudeDrift(ctx, claude.New(host), r, out); err != nil {
		return err
	}
	if err := codexDrift(ctx, codex.New(host), r, out); err != nil {
		return err
	}
	if opt.helpPath != "" {
		if err := helpDrift(opt.helpPath, r); err != nil {
			return err
		}
	}
	fmt.Fprintln(out)
	r.print(out)
	fmt.Fprintf(out, "%d distinct findings, %d already in the baseline\n", len(r.counts), r.suppressed)
	return nil
}
```

Because the Host is a parameter, the check also runs inside a test, on a copy of the fixtures, with `agenttest`'s fakes failing the test if it ever runs a binary or calls the network (see [Fakes and fixture hosts](../testing/agenttest.md)):

<!-- snippet: examples/drift/main_test.go#host -->
```go
// driftHost copies both fixture homes into a temp user home and returns a
// fixture Host over the copy: FailExec and FailingHTTP (the agenttest
// defaults) fail the test if the drift check ever runs a binary or calls
// the network, and the copy lets a test edit files without touching the
// checked-in fixtures.
func driftHost(t *testing.T) agentio.Host {
	t.Helper()
	home := t.TempDir()
	agenttest.CopyFixture(t, filepath.Join(repoRoot, claudeFixture), home)
	agenttest.CopyFixture(t, filepath.Join(repoRoot, codexFixture), filepath.Join(home, ".codex"))
	return agenttest.NewHost(t,
		agenttest.WithUserHome(home),
		agenttest.WithEnv("HOME="+home, "PATH="+filepath.Join(home, "bin")),
	)
}
```

The report starts with one line per vendor comparing the installed version with the tested range (`installed 2.1.326, tested 2.1.323..2.1.325: newer`, or [`install not found`](ds:block?id=examples-drift-versionline-4gjggnvh) when the binary is not on PATH), then a header line per home; for Claude a second line gives the number of transcripts read and the record versions seen in them.

## Step 2 — Read the findings

After a blank line come the findings, one per line, tab-separated so a saved report can be read back: `vendor`, `home`, `kind`, `source`, `detail`, [`(xN)`](ds:block?id=examples-drift-report-print-pgxsfup3) where N counts identical findings. `source` names the reader (`settings`, `global-config`, `sessions`, `transcript assistant`, `config.toml`, …) and `detail` is a path into the decoded value ([`Settings.Unknown.futureKey`](ds:block?id=examples-drift-walker-walk-22f5frvq), `[]` for a slice element, `{}` for a map value).

| Kind | Meaning | What to do |
|---|---|---|
| [`unknown-key`](ds:block?id=examples-drift-walker-walk-22f5frvq) | A key reached an `agentio.Unknown` field: the vendor writes it, no struct field models it. | Add a field when a caller needs the value; otherwise it is kept as raw JSON and harmless. |
| [`unknown-enum`](ds:block?id=examples-drift-walker-walk-22f5frvq) | An open enum value whose `IsKnown()` is false, printed with its type and spelling, e.g. `PermissionMode="futureMode"`. | Add the constant to the enum. |
| [`unknown-record`](ds:block?id=examples-drift-transcriptdrift-3bba7jgq) | A transcript record type the library has no constant for. | Model the record type. |
| [`new-record-key`](ds:block?id=examples-drift-transcriptdrift-3bba7jgq) | A top-level key on a transcript record of a type, first written by a Claude Code version newer than `claude.TestedMin` (among the sampled transcripts). | Check whether the record view needs it. |
| [`warning`](ds:block?id=examples-drift-walker-walk-22f5frvq) | A decode warning the reader returned (`agentio.Warnings`): kind, file, key, detail. | Read the detail; usually a value of an unexpected shape. |
| [`format-version`](ds:block?id=examples-drift-walker-read-nhu7788k) | A file declares a format version the library has no sample for (`ErrUnknownVersion`); only that reader fails. | Record a sample and support the version. |
| [`read-error`](ds:block?id=examples-drift-walker-read-nhu7788k) | Any other reader error. A missing file (`ErrNotFound`) or unsupported read (`ErrUnsupported`) is not a finding. | Investigate; usually a malformed file. |
| [`flag-not-in-table`](ds:block?id=examples-drift-helpdrift-bd9jjgwy) | A long flag in a saved `claude --help` that no `features.tsv` row names (Step 3). | Candidate feature row. |

This test writes a new settings key and an unknown permission mode into the copied home and finds both:

<!-- snippet: examples/drift/main_test.go#newkey -->
```go
// TestNewKeyAndEnum adds a settings key and a permission mode the library
// has never seen to the copied home. The check reports both: the key from
// Settings.Unknown, the mode from PermissionMode.IsKnown.
func TestNewKeyAndEnum(t *testing.T) {
	h := driftHost(t)
	agenttest.WriteFile(t, h.UserHome, ".claude/settings.json",
		`{"model": "opus", "futureKey": true, "permissions": {"defaultMode": "futureMode"}}`)
	out := runDrift(t, h, options{})
	wantLines(t, out,
		"claude\tpersonal\tunknown-key\tsettings\tSettings.Unknown.futureKey\t(x1)\n",
		"claude\tpersonal\tunknown-enum\tsettings\tSettings.Permissions.DefaultMode PermissionMode=\"futureMode\"\t(x1)\n",
	)
}
```

## Step 3 — Compare with a baseline

A real home has many known-unknown keys, so the useful question after a release is "what is new since last time". Save a report and pass it back with [`-baseline last.tsv`](ds:block?id=examples-drift-report-loadbaseline-4xbs9yxq): every finding whose vendor, kind, source and detail appear in the saved file is skipped, and [the summary line counts how many were suppressed](ds:block?id=examples-drift-run-xbppew9s). [Home and count are not part of the comparison](ds:block?id=examples-drift-finding-baselinekey-bm3savct), so the same key in another home, or seen more often, is not new drift.

<!-- snippet: examples/drift/main_test.go#baseline -->
```go
// TestBaseline saves one run and feeds it back with -baseline: every
// finding is already known, so the second report lists none of them and
// only the summary line counts them as suppressed.
func TestBaseline(t *testing.T) {
	h := driftHost(t)
	first := runDrift(t, h, options{})
	base := filepath.Join(t.TempDir(), "last.tsv")
	if err := os.WriteFile(base, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	second := runDrift(t, h, options{basePath: base})
	if strings.Contains(second, "\tunknown-key\t") {
		t.Fatalf("baseline did not suppress known findings:\n%s", second)
	}
	wantLines(t, second, "246 distinct findings, 246 already in the baseline\n")
}
```

[`-claude-help help.txt`](ds:block?id=examples-drift-helpdrift-bd9jjgwy) takes a saved `claude --help` text and lists every long flag no row of `claude/features.tsv` names. The table only holds features the library emits or gates, so a hit is a candidate row, not an error. Save the text yourself (`claude --help > help.txt`); the drift check never runs the binary.

<!-- snippet: examples/drift/main_test.go#help -->
```go
// TestClaudeHelp feeds a saved `claude --help` text: a flag the feature
// table names is quiet, a flag it does not name is a candidate row.
func TestClaudeHelp(t *testing.T) {
	help := filepath.Join(t.TempDir(), "help.txt")
	text := "Options:\n  --worktree [name]   Create a git worktree\n  --brand-new-flag    Something new\n"
	if err := os.WriteFile(help, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	out := runDrift(t, driftHost(t), options{helpPath: help})
	wantLines(t, out, "claude\t-\tflag-not-in-table\t--help\t--brand-new-flag\t(x1)\n")
	if strings.Contains(out, "\t--worktree\t") {
		t.Fatal("--worktree is in features.tsv and must not be reported")
	}
}
```

A release routine is then: `task drift -- -baseline last.tsv -claude-help help.txt`, read what is left, update the readers or `features.tsv`, and save the new report as the next baseline.

## Try it

```sh
cd examples && go run ./drift -fixtures | grep -E 'install|home|transcripts|skills|distinct'
```

```text
claude: install not found (agentio: binary not found on PATH: claude)
claude home personal
  transcripts read 2, record versions 2.1.325
claude home work
  transcripts read 1, record versions 2.1.325
codex: install not found (agentio: binary not found on PATH: codex)
codex home 0.162.1
claude	personal	unknown-key	skills	[].Unknown.license	(x1)
246 distinct findings, 0 already in the baseline
```

```sh
cd examples && go test -count=1 -v ./drift
```

```text
=== RUN   TestFixtures
--- PASS: TestFixtures (0.03s)
=== RUN   TestFixturesFlagHost
--- PASS: TestFixturesFlagHost (0.02s)
=== RUN   TestNewKeyAndEnum
--- PASS: TestNewKeyAndEnum (0.02s)
=== RUN   TestBaseline
--- PASS: TestBaseline (0.04s)
=== RUN   TestClaudeHelp
--- PASS: TestClaudeHelp (0.02s)
PASS
ok  	github.com/ubgo/agentio/examples/drift	0.131s
```

## Pitfalls

- Without [`-fixtures`](ds:block?id=examples-drift-pickhost-k35cnr56) the check reads the real homes of the user running it (that is the point of `task drift`). In tests, always pass a fixture Host to `run`.
- The check samples to stay fast on large homes: [the newest 200 conversations per home](ds:block?id=examples-drift-transcriptsample-22745jby), the last 256 KiB of each transcript, and the newest 2000 prompt-history entries. Drift that lives only in older data is not seen.
- [`new-record-key`](ds:block?id=examples-drift-transcriptdrift-3bba7jgq) compares against the oldest version that wrote a key among the sampled transcripts. When every sampled transcript is newer than `claude.TestedMin` (the 2.1.325 fixture only has 2.1.325 records, so it reports 129 such findings), every key looks new. Use a baseline to see only what changed.
- Codex is checked for [`config.toml`](ds:block?id=examples-drift-codexdrift-xspx77gd) (with its warnings), the login (`auth.json`) and conversations only; other Codex files are not walked yet.
- A baseline must be a saved report: only lines with [exactly six tab-separated columns](ds:block?id=examples-drift-report-loadbaseline-4xbs9yxq) are read, the header lines are ignored. Edited or reformatted files silently suppress nothing.
- [`-claude-help`](ds:block?id=examples-drift-helpdrift-bd9jjgwy) matches long flags only (`--name`); a flag that `features.tsv` spells together with a value (`--output-format stream-json`) counts as known by each `--` token in its spelling.
- [`install not found`](ds:block?id=examples-drift-versionline-4gjggnvh) is reported, not an error: the homes are still checked. A home with no files for a reader is quiet, not a finding.

## Related

- [Tested vendor versions](compat.md) and [Per-feature version table](features.md).
- [Optional values, unknown keys and warnings](../core/values.md): the `Unknown` and `Warnings` types the check walks.
- [Fixture homes](../testing/fixtures.md): the homes `-fixtures` reads, and where a new vendor sample goes.
- [Fakes and fixture hosts](../testing/agenttest.md).
- [examples/drift](../../examples/drift/) and the [feature index](../reference/features.md#compat-and-drift).
- [Choosing guides](../choosing/README.md).
