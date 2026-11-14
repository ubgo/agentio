package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/gemini"
	"github.com/ubgo/agentio/pi"
)

// repoRoot is the repository root seen from this package's directory (go
// test runs a package in its own folder).
const repoRoot = "../.."

// snippet:start newhost
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

// snippet:end newhost

// snippet:start testcollect
// TestCollect runs the code under test on the fixture Host and checks its
// output. FailingHTTP and FailExec (agenttest defaults) prove the report
// reads disk only: a network call or a process run fails the test.
func TestCollect(t *testing.T) {
	h := newFixtureHost(t)
	var b strings.Builder
	if err := run(context.Background(), &b, h); err != nil {
		t.Fatal(err)
	}
	want := "claude homes=[personal work] conversations=3 capabilities=26\n" +
		"codex homes=[personal] conversations=2 capabilities=22\n" +
		"gemini homes=[] conversations=0 capabilities=14\n" +
		"pi homes=[] conversations=0 capabilities=13\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}

// snippet:end testcollect

// snippet:start testmain
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

// snippet:end testmain

// snippet:start secrets
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

// snippet:end secrets

// snippet:start processes
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

// snippet:end processes

// snippet:start exec
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

// snippet:end exec

// snippet:start failexec
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

// snippet:end failexec

// snippet:start http
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

// snippet:end http

// snippet:start cached
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

// snippet:end cached

// snippet:start conformance
// TestConformance runs the shared suite on both vendors' neutral views with
// a fixture home each. The suite fails the test if a read accessor runs a
// process or makes a network call, if the default home launches with the
// home variable set, or if a token login leaks the token.
func TestConformance(t *testing.T) {
	t.Run("claude", func(t *testing.T) {
		a := claude.New(newFixtureHost(t))
		agenttest.Conformance(t, a.Neutral(), a.DefaultHome().Neutral())
	})
	t.Run("codex", func(t *testing.T) {
		a := codex.New(newFixtureHost(t))
		agenttest.Conformance(t, a.Neutral(), a.DefaultHome().Neutral())
	})
}

// snippet:end conformance

// snippet:start write
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

// snippet:end write

// snippet:start loop
// toySpec is a config-defined agent with one home marker and no
// conversation reader: the loop in collect must handle it without a
// special case.
func toySpec() agentio.Spec {
	return agentio.Spec{
		ID: "toy", Name: "Toy Agent", Binary: "toy",
		DefaultHome: ".toy", HomeMarkers: []string{"config.json"},
		NewArgs: []string{"{args}"},
	}
}

// TestLoopOverAgents runs collect with the two built-ins plus the toy
// agent. The toy home is written into the copied user home, so the loop
// sees three agents and the toy one reports no conversations.
func TestLoopOverAgents(t *testing.T) {
	h := newFixtureHost(t)
	agenttest.WriteFile(t, h.UserHome, ".toy/config.json", "{}")
	rows, err := collect(context.Background(), h, toySpec())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]agentio.ID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	if !slices.Equal(ids, []agentio.ID{claude.ID, codex.ID, gemini.ID, pi.ID, "toy"}) {
		t.Fatalf("ids %v", ids)
	}
	toy := rows[len(rows)-1]
	if len(toy.Homes) != 1 || toy.Conversations != 0 {
		t.Fatalf("toy row %+v", toy)
	}
}

// snippet:end loop
