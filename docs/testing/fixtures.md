# Fixture homes

The repository ships real Claude Code and Codex homes, recorded from real installs with every value replaced, under `testdata/`. Every reader in the library is tested on them, and you can point your own tests and programs at them to get realistic files without a vendor install, a login or your own data. Use them whenever you need "a home that looks like a real one".

## Before you start

The fixture homes, relative to the repository root:

| Path | What it is |
|---|---|
| `claude/testdata/homes/2.1.323/userhome` | a whole user home recorded at Claude Code 2.1.323 |
| `claude/testdata/homes/2.1.325/userhome` | the same at 2.1.325: `.claude/` (the default home: settings, credentials, sessions, transcripts, history, skills, agents, commands, plugins, policy limits, stats), `.claude-work/` (a second home), `.claude.json` (global state) and `project/` (a project with `.mcp.json`, `CLAUDE.md` and `.claude/`) |
| `claude/testdata/homes/2.1.327/userhome` | the 2.1.325 tree with the 2.1.327 changes: a session waiting on a dialog (`status` `waiting`, `waitingFor`), three new `.claude.json` keys, `gitCommitSha` on installed plugins, `toolDenialBeforeRun` on a transcript record |
| `claude/testdata/stream/2.1.325` | a recorded `--output-format stream-json` run |
| `codex/testdata/homes/0.162.1` | one Codex home at 0.162.1: `config.toml`, `work.config.toml`, `auth.json`, `sessions/`, `session_index.jsonl`, `history.jsonl`, `hooks.json`, `rules/`, `skills/`, `plugins/`, `thread-writer-locks/`, `models_cache.json` |

The Claude fixtures are user homes (Claude Code keeps `.claude.json` next to `.claude/`, not inside it); the Codex fixture is a single home, the contents of `~/.codex`. Each Claude version with a fixture is part of the tested range (`claude.TestedMin`..`claude.TestedMax`, see [Tested vendor versions](../compat/compat.md)), and the Claude tests run every reader on every fixture version.

## Step 1 — Use a checked-in fixture home

A program (not a test) can read a fixture in place: build an `agentio.Host` whose `UserHome` is the Claude fixture and whose env selects the Codex fixture with `CODEX_HOME`. Fields you leave nil (`Secrets`, `Processes`, `HTTP`, `Exec`) are never reached by the Disk reads this program makes. This is what `go run ./testing` does in [examples/testing](../../examples/testing/).

<!-- snippet: examples/testing/main.go#repohost -->
```go
// repoFixtureHost builds a Host over the fixture homes under repoRoot
// without the testing package: a program (not a test) can still run on
// fixtures. Secrets, Processes, HTTP and Exec stay nil; the readers this
// program calls never reach them.
func repoFixtureHost(repoRoot string) (agentio.Host, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return agentio.Host{}, err
	}
	home := filepath.Join(root, claudeFixture)
	return agentio.Host{
		UserHome: home,
		User:     fixtureUser,
		Env: []string{
			"HOME=" + home,
			noBinaries,
			codex.EnvHome + "=" + filepath.Join(root, codexFixture),
		},
		Now: fixtureClock,
	}, nil
}
```

A test can use the same Host; reading in place is safe because the readers never write.

<!-- snippet: examples/testing/main_test.go#testmain -->
```go
// TestMainHost runs the same code on the Host main builds, which reads the
// checked-in fixtures in place (read-only) instead of a temp copy.
func TestMainHost(t *testing.T) {
	h, err := repoFixtureHost(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := run(context.Background(), &b, h); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "codex homes=[0.162.1] conversations=2") {
		t.Fatalf("got\n%s", b.String())
	}
}
```

Notice the Codex home is named `0.162.1` here: a home selected with `CODEX_HOME` is a non-default home and takes its name from the directory. Copied into `<userhome>/.codex` (Step 2) the same files are the default home, named `personal`.

## Step 2 — Copy a fixture before writing

Anything that writes (a Writer door, a test that adds a file, code that creates a lock) must work on a copy. `agenttest.CopyFixture(t, src, dst)` copies a tree into a temp dir; `agenttest.WriteFile(t, dir, rel, data)` writes one file under it, creating parent directories, and returns its path. The test below copies both fixtures, then adds a local settings layer to the fixture project and reads it back.

<!-- snippet: examples/testing/main_test.go#newhost -->
```go
// newFixtureHost copies the Claude fixture user home and the Codex fixture
// home into a fresh temp dir and builds a Host over the copy. Copying keeps
// the checked-in fixtures untouched even when the code under test writes.
// Every other input is an agenttest default: fixed clock, empty fake
// secrets and process table, and an HTTP client and process runner that
// fail the test on use. opts override any of them.
func newFixtureHost(t *testing.T, opts ...agenttest.HostOption) agentio.Host {
	t.Helper()
	home := t.TempDir()
	agenttest.CopyFixture(t, filepath.Join(repoRoot, claudeFixture), home)
	agenttest.CopyFixture(t, filepath.Join(repoRoot, codexFixture), filepath.Join(home, ".codex"))
	base := []agenttest.HostOption{
		agenttest.WithUserHome(home),
		// WithEnv replaces the whole env: HOME is not added for you, and
		// a PATH with no binaries keeps a real claude/codex out of reach.
		agenttest.WithEnv("HOME="+home, "PATH="+filepath.Join(home, "bin")),
	}
	return agenttest.NewHost(t, append(base, opts...)...)
}
```

<!-- snippet: examples/testing/main_test.go#write -->
```go
// TestWriteFile adds a file to the copied home before the read: a local
// settings layer in the fixture project. The checked-in fixture never
// changes because newFixtureHost copied it first.
func TestWriteFile(t *testing.T) {
	h := newFixtureHost(t)
	a := claude.New(h)
	p, err := a.Project(filepath.Join(h.UserHome, "project"))
	if err != nil {
		t.Fatal(err)
	}
	agenttest.WriteFile(t, p.Dir, ".claude/settings.local.json", `{"model": "opus"}`)
	layer, err := a.LocalSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if !layer.Present || layer.Settings.Model != "opus" {
		t.Fatalf("layer %+v", layer)
	}
}
```

## Try it

```sh
cd examples && go run ./testing
```

```text
claude homes=[personal work] conversations=3 capabilities=31
codex homes=[0.162.1] conversations=2 capabilities=27
gemini homes=[] conversations=0 capabilities=18
pi homes=[] conversations=0 capabilities=16
opencode homes=[] conversations=0 capabilities=10
```

```sh
cd examples && go test -count=1 -v -run 'TestMainHost|TestWriteFile' ./testing
```

```text
=== RUN   TestMainHost
--- PASS: TestMainHost (0.00s)
=== RUN   TestWriteFile
--- PASS: TestWriteFile (0.00s)
PASS
ok  	github.com/ubgo/agentio/examples/testing	0.010s
```

## Pitfalls

- Never write into `testdata/` itself. Copy first; `CopyFixture` exists so a test that writes cannot change a checked-in file by accident.
- `CopyFixture` writes files `0600` and directories `0700` and does not keep modification times. Codex lists conversations newest first by file time, so after a copy the order is the copy order, not the recorded one; set times with `os.Chtimes` when your test depends on order (the Codex module's own tests do).
- Fixture values are placeholders: tokens read `sk-ant-oat01.FIXTURE-NOT-A-TOKEN`, strings `<s>`, project paths `/work/projN`. Assert on structure and keys, not on values you would expect from a real account.
- The fixture login expires relative to `agenttest.FixedNow` (2026-11-13 12:00 UTC). A Host with the real clock sees it as expired.
- A fixture user home has no `bin/`, and the `PATH` in these examples names a directory that does not exist, so `Install` returns `ErrNotInstalled`. To test install detection, create a fake binary layout in the copy.
- From another module, the fixtures live in the Go module cache (`go list -m -f '{{.Dir}}' github.com/ubgo/agentio/claude`), which is read-only: copy them before use.

## Related

- [Fakes and fixture hosts](agenttest.md): every `agenttest` fake and option.
- [Testing your own code](your-code.md): run your code on these homes.
- [The conformance suite](conformance.md).
- [Tested vendor versions](../compat/compat.md): how fixtures set the tested range.
- [Drift check](../compat/drift.md): what the fixtures do not cover yet.
- [examples/testing](../../examples/testing/) and the [feature index](../reference/features.md#testing-agenttest-and-fixtures).
- [Choosing guides](../choosing/README.md).
