# Fakes and fixture hosts

Package `agenttest` builds an `agentio.Host` for tests and holds a fake for every input a Host carries: the secret store, the process table, the process runner and the HTTP client. Use it when you test code that reads Claude Code or Codex homes, so the test runs on fixture files, never on the real machine, and fails loudly if a read path ever runs a binary or calls the network.

Every library read goes through a Host (user home, environment, clock, HTTP client, keychain, process table, process runner), so swapping the Host is the whole trick: nothing in agentio reads `$HOME`, `os.Environ`, the clock or the network behind your back. See [The Host](../core/host.md) for the type itself.

## Before you start

- Import `github.com/ubgo/agentio/agenttest` from `_test.go` files only. It imports `testing` and is part of the root module, so it adds no dependency.
- Have a home to point at: one of the checked-in [fixture homes](fixtures.md), or files you write into the temp dir with `agenttest.WriteFile`.
- The snippets below come from [examples/testing/main_test.go](../../examples/testing/main_test.go); run them with `cd examples && go test ./testing`.

## Step 1 — Build a fixture Host

`agenttest.NewHost(t, opts...)` returns a Host whose every input is safe by default:

| Field | Default | Why |
|---|---|---|
| `UserHome` | `t.TempDir()` (empty) | nothing real is ever read |
| `User` | `"tester"` | keychain lookups name an account |
| `Env` | `HOME=<temp>`, `PATH=/nonexistent-agenttest-bin` | no vendor binary can be found |
| `Now` | `agenttest.FixedNow` (2026-11-13 12:00 UTC) | expiry and "since" checks are stable |
| `HTTP` | `agenttest.FailingHTTP(t)` | any request fails the test |
| `Secrets` | empty `FakeSecrets` | no keychain |
| `Processes` | empty `FakeProcesses` | no process table |
| `Exec` | `agenttest.FailExec{T: t}` | any process run fails the test |

Options replace one field each: `WithUserHome`, `WithEnv`, `WithNow`, `WithHTTP`, `WithSecrets`, `WithProcesses`, `WithExec`, and `WithSystem(goos, root)` sets `GOOS` and `SystemRoot` together (a fixture host has neither, so it never reads the machine's `/etc`). The helper below copies the fixture homes into a temp dir and points the Host at the copy; every later test in this page builds on it.

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

The test that uses it runs a whole report on the fixture Host. The defaults are what make it a real check: if the report ever ran `claude --version` or fetched usage, `FailExec` or `FailingHTTP` would fail the test.

<!-- snippet: examples/testing/main_test.go#testcollect -->
```go
// TestCollect runs the code under test on the fixture Host and checks its
// output. FailingHTTP and FailExec (agenttest defaults) prove the report
// reads disk only: a network call or a process run fails the test.
func TestCollect(t *testing.T) {
	h := newFixtureHost(t)
	var b strings.Builder
	if err := run(context.Background(), &b, h); err != nil {
		t.Fatal(err)
	}
	want := "claude homes=[personal work] conversations=3 capabilities=25\n" +
		"codex homes=[personal] conversations=2 capabilities=22\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}
```

## Step 2 — Fake secrets and processes

`FakeSecrets` is an in-memory keychain keyed by `SecretKey{Service, Account}`. `Put` stores a value, `Get` looks up the exact key and, for account `""`, falls back to any entry of the service (the way `security -s` does on macOS). Every lookup is appended to `Calls`, so a test can assert the lookup order: Claude reads the account named after `Host.User` first and only then any entry of the service, because stale same-service entries exist on real machines. Set `Err` to make every `Get` fail.

<!-- snippet: examples/testing/main_test.go#secrets -->
```go
// TestFakeSecrets puts a login in the fake keychain and reads it back
// through claude.Credential. Calls records every lookup, so the test can
// check the order: the account named after Host.User first, then any entry
// of the service.
func TestFakeSecrets(t *testing.T) {
	secrets := agenttest.NewFakeSecrets()
	h := newFixtureHost(t, agenttest.WithSecrets(secrets))
	h.User = "user"
	a := claude.New(h)
	home := a.DefaultHome()
	svc := claude.KeychainServices(home.Dir(), home.IsDefault())[0]
	secrets.Put(svc, "user", `{"claudeAiOauth":{"accessToken":"keychain-copy","expiresAt":1893456000000,"subscriptionType":"pro"}}`)

	cred, err := a.Credential(home)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Source != claude.SourceKeychain || cred.SubscriptionType != "pro" {
		t.Fatalf("credential from %s, plan %q", cred.Source, cred.SubscriptionType)
	}
	if len(secrets.Calls) == 0 || secrets.Calls[0] != (agenttest.SecretKey{Service: svc, Account: "user"}) {
		t.Fatalf("first lookup %v, want the named account", secrets.Calls)
	}
}
```

`FakeProcesses` is a fixed process table: `List` is what `Processes` returns, `Cwds` maps a pid to its working directory (an unknown pid returns `ErrFakeNotFound`), and `Err` makes `Processes` fail. Live-session readers match a session file to a process by pid and start time, so give the fake process a `Start` that agrees with the file.

<!-- snippet: examples/testing/main_test.go#processes -->
```go
// fixtureSessionStart is startedAt of the fixture session file
// .claude/sessions/4242.json (Unix milliseconds).
const fixtureSessionStart = 1794597025793

// TestFakeProcesses lists Claude live sessions against a fixed process
// table. The fixture home has session files for pids 4242 and 4243; only
// 4242 is in the table with a start time that matches, so only it is alive.
func TestFakeProcesses(t *testing.T) {
	started := time.UnixMilli(fixtureSessionStart)
	procs := &agenttest.FakeProcesses{List: []agentio.Process{
		{PID: 4242, Start: started.Add(-3 * time.Second), Argv: []string{claude.Binary}},
	}}
	a := claude.New(newFixtureHost(t, agenttest.WithProcesses(procs)))
	sessions, err := a.LiveSessions(a.DefaultHome())
	if err != nil {
		t.Fatal(err)
	}
	alive := map[int]bool{}
	for _, s := range sessions {
		alive[s.PID] = s.Alive
	}
	if !alive[4242] || alive[4243] {
		t.Fatalf("alive = %v, want only 4242", alive)
	}
}
```

## Step 3 — Fake the vendor binary

Read paths never run a process; only Runner doors do (see [Doors](../capabilities/doors.md)). To test a Runner call, give the Host a `*FakeExec`: `Results` maps a command line (`Command.String()`, for example `claude --version`) to a canned `agentio.ProcessResult`, unmatched calls get `Default`, and every call is recorded in `Calls` with its full environment, so you can check the home variable rule too.

<!-- snippet: examples/testing/main_test.go#exec -->
```go
// TestFakeExec runs the Runner door against canned results keyed by the
// command line (Command.String()). No claude binary exists: FakeExec never
// starts a process, and Calls records what would have run.
func TestFakeExec(t *testing.T) {
	fx := &agenttest.FakeExec{Results: map[string]agentio.ProcessResult{
		"claude --version": {Stdout: []byte("2.1.325 (Claude Code)\n")},
	}}
	a := claude.New(newFixtureHost(t, agenttest.WithExec(fx)))
	v, err := a.Runner().BinaryVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != claude.TestedMax || len(fx.Calls) != 1 {
		t.Fatalf("version %s after %d calls", v, len(fx.Calls))
	}
	if _, set := agentio.EnvLookup(fx.Calls[0].Env, claude.ConfigDirEnv); set {
		t.Fatalf("default home run carries %s", claude.ConfigDirEnv)
	}
}
```

`FailExec` is the opposite: it reports `agenttest: unexpected process run: <argv>` through `T.Errorf` and returns `ErrForbidden`, so the code under test still sees an error and keeps going. `newFixtureHost` leaves it bound to `t`, so the test below proves a read path runs no process: `Install` reads PATH and disk only and returns `ErrNotInstalled` on the fixture's empty PATH.

<!-- snippet: examples/testing/main_test.go#failexec -->
```go
// TestFailExec shows the default runner guarding a read path. newFixtureHost
// leaves Exec as FailExec bound to t: a process run would fail this test
// with "agenttest: unexpected process run: <argv>" and hand the caller
// ErrForbidden. Install reads PATH and disk only, so with no claude on the
// fixture PATH it returns ErrNotInstalled and FailExec never fires.
func TestFailExec(t *testing.T) {
	a := claude.New(newFixtureHost(t))
	if _, err := a.Install(); !errors.Is(err, agentio.ErrNotInstalled) {
		t.Fatalf("Install err %v, want ErrNotInstalled", err)
	}
}
```

## Step 4 — Point HTTP at a test server

Network reads (usage, subscription, token probes) use `Host.HTTP`. Start an `httptest.Server`, pass its client with `WithHTTP(srv.Client())`, and point the vendor client at it with `WithBaseURL(srv.URL)`. The credential still comes from the fixture home, so the request carries a bearer token you can assert on without it ever being real.

<!-- snippet: examples/testing/main_test.go#http -->
```go
// usageBody is a /api/oauth/usage response with one five-hour window, in
// the shape the Claude usage client decodes.
const usageBody = `{"limits":[{"kind":"session","group":"session","percent":42,"severity":"normal","resets_at":"2026-11-13T15:00:00Z","scope":null,"is_active":true}]}`

// TestHTTP points the Claude usage client at an httptest server: WithHTTP
// gives the Host the server's client and WithBaseURL replaces the vendor
// URL. The fixture home's .credentials.json supplies the bearer token.
func TestHTTP(t *testing.T) {
	var gotAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
		_, _ = w.Write([]byte(usageBody))
	}))
	defer srv.Close()
	a := claude.New(newFixtureHost(t, agenttest.WithHTTP(srv.Client()))).WithBaseURL(srv.URL)
	u, err := a.Usage(context.Background(), a.DefaultHome())
	if err != nil {
		t.Fatal(err)
	}
	if !gotAuth || u.Source != agentio.UsageServer || len(u.Windows) != 1 || u.Windows[0].Percent != 42 {
		t.Fatalf("usage %+v (auth sent %t)", u.Usage, gotAuth)
	}
}
```

Keep `FailingHTTP` (the default) everywhere else. It is the cheapest proof that a Disk read stays a Disk read: here `CachedUsage` reads the usage Claude Code cached in `.claude.json`, and a network call would fail the test.

<!-- snippet: examples/testing/main_test.go#cached -->
```go
// TestCachedUsageNoNetwork reads the usage Claude Code cached in
// .claude.json. The Host keeps the default FailingHTTP client, so the test
// fails if this Disk read ever reaches the network.
func TestCachedUsageNoNetwork(t *testing.T) {
	a := claude.New(newFixtureHost(t))
	u, found, err := a.CachedUsage(a.DefaultHome())
	if err != nil || !found {
		t.Fatalf("found %t, err %v", found, err)
	}
	if u.Source != agentio.UsageCache || len(u.Windows) == 0 {
		t.Fatalf("usage %+v", u.Usage)
	}
}
```

## Try it

```sh
cd examples && go test -count=1 -v -run 'TestFakeSecrets|TestFakeProcesses|TestFakeExec|TestFailExec|TestHTTP|TestCachedUsageNoNetwork' ./testing
```

```text
=== RUN   TestFakeSecrets
--- PASS: TestFakeSecrets (0.00s)
=== RUN   TestFakeProcesses
--- PASS: TestFakeProcesses (0.00s)
=== RUN   TestFakeExec
--- PASS: TestFakeExec (0.00s)
=== RUN   TestFailExec
--- PASS: TestFailExec (0.00s)
=== RUN   TestHTTP
--- PASS: TestHTTP (0.01s)
=== RUN   TestCachedUsageNoNetwork
--- PASS: TestCachedUsageNoNetwork (0.01s)
PASS
ok  	github.com/ubgo/agentio/examples/testing	0.035s
```

## Pitfalls

- `WithEnv` replaces the whole environment. `HOME` is not added for you, and if you drop `PATH` the agent has no PATH at all. Pass both, as `newFixtureHost` does.
- The default `UserHome` is an empty temp dir. An agent built on a bare `NewHost(t)` finds no homes and `Install` returns `ErrNotInstalled`; copy a fixture or write files first.
- `FailExec` and `FailingHTTP` call `Errorf`, not `Fatal`: the test keeps running and the code under test gets `ErrForbidden`. Look for the `agenttest: unexpected process run` or `agenttest: unexpected HTTP` line in the failure output, not for a panic.
- `FakeExec` matches on `Command.String()`, which is the binary name plus shell-quoted args (never the env). Launch builders never walk PATH, so the key is `claude --version`, not an absolute path. An unmatched call returns `Default`, whose zero value is a successful run with empty output, not an error.
- `FakeSecrets.Get` with account `""` returns any entry of the service when there is no exact match; with several entries for one service the one returned is not fixed. Store one entry per service, or name the account.
- The fixture logins expire relative to `FixedNow`. If you replace the clock with `time.Now`, Disk reads of the fixture credential report it as expired.
- `FailExec` binds to the `t` passed to `NewHost`. A Host built in a parent test and used inside `t.Run` reports a stray call on the parent; build one Host per subtest.

## Related

- [Fixture homes](fixtures.md): the checked-in homes and how to copy one before writing.
- [The conformance suite](conformance.md): one call that runs every shared check on an agent.
- [Testing your own code](your-code.md): inject a Host into your code and test a loop over agents.
- [The Host](../core/host.md) and [Doors](../capabilities/doors.md).
- [examples/testing](../../examples/testing/) and the [feature index](../reference/features.md#testing-agenttest-and-fixtures).
- [Choosing guides](../choosing/README.md).
