// Command opencode walks the typed opencode API on a copy of the 1.18.30
// fixture user home: the install (version from the npm package.json, never
// running opencode), the data dir as the home, the legacy storage/
// sessions with their turns and cursor, the sessions and turns of an
// opencode.db built in the temp dir (opencode/store), the launch commands,
// process classification with a stop plan, the doctor and the capability
// gaps with their reasons. Everything runs on temp copies: no opencode
// binary, account, network or quota.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/opencode"
	"github.com/ubgo/agentio/opencode/store"
)

// Fixture locations, relative to the examples module.
const (
	defaultFixture = "../opencode/testdata/homes/1.18.30/userhome"
	defaultSchema  = "../opencode/store/testdata/schema-1.18.30.sql"
)

// Example constants.
const (
	installedVersion = "1.18.30"
	fixtureUser      = "fixture"
	homeTilde        = "~"
	dirPerm          = 0o700
	exePerm          = 0o755
	filePerm         = 0o600
	// driverName is registered by opencode/store (pure-Go
	// modernc.org/sqlite), imported above.
	driverName = "sqlite"
	// legacySession is the fixture's 0.14.7 session.
	legacySession = "ses_0f0000000001ffeFixtureAAAA"
	// dbSession is the session seeded into the example database.
	dbSession = "ses_0f0000000100ffeExampleDB01"
)

// seed is a made-up session in the shape opencode 1.18.30 writes: a user
// message, a tool-call step, the answer, and a newest assistant message
// still streaming (no time.completed).
const seed = `
INSERT INTO project (id, worktree, time_created, time_updated, sandboxes) VALUES ('global', '/', 1, 2, '[]');
INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated, agent, model, cost, tokens_input, tokens_output, tokens_reasoning, tokens_cache_read)
VALUES ('` + dbSession + `', 'global', 'calm-heron', '/work/app', 'Add a health check', '1.18.30', 1790000000000, 1790000090000,
  'build', '{"id":"gpt-5","providerID":"openai"}', 0.04, 2300, 410, 120, 1600);
INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
 ('msg_e1', '` + dbSession + `', 1790000000000, 1790000000000, '{"role":"user","time":{"created":1790000000000},"agent":"build","model":{"providerID":"openai","modelID":"gpt-5"}}'),
 ('msg_e2', '` + dbSession + `', 1790000001000, 1790000020000, '{"role":"assistant","time":{"created":1790000001000,"completed":1790000020000},"modelID":"gpt-5","providerID":"openai","mode":"build","agent":"build","path":{"cwd":"/work/app","root":"/work/app"},"cost":0.02,"tokens":{"total":1300,"input":1100,"output":200,"reasoning":100,"cache":{"read":800,"write":0}},"finish":"tool-calls"}'),
 ('msg_e3', '` + dbSession + `', 1790000021000, 1790000040000, '{"role":"assistant","time":{"created":1790000021000,"completed":1790000040000},"modelID":"gpt-5","providerID":"openai","mode":"build","agent":"build","path":{"cwd":"/work/app","root":"/work/app"},"cost":0.02,"tokens":{"total":1430,"input":1200,"output":210,"reasoning":20,"cache":{"read":800,"write":0}},"finish":"stop"}'),
 ('msg_e4', '` + dbSession + `', 1790000090000, 1790000090000, '{"role":"assistant","time":{"created":1790000090000},"modelID":"gpt-5","providerID":"openai","mode":"build","agent":"build","path":{"cwd":"/work/app","root":"/work/app"},"cost":0,"tokens":{"input":0,"output":0,"reasoning":0,"cache":{"read":0,"write":0}}}');
INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
 ('prt_e1', 'msg_e1', '` + dbSession + `', 1, 1, '{"type":"text","text":"add a /health endpoint"}'),
 ('prt_e2', 'msg_e2', '` + dbSession + `', 2, 2, '{"type":"text","text":"Reading the router first."}'),
 ('prt_e3', 'msg_e2', '` + dbSession + `', 3, 3, '{"type":"tool","callID":"call_e1","tool":"read","state":{"status":"completed","input":{},"output":"x","title":"router.go","metadata":{},"time":{"start":3,"end":4}},"metadata":{}}'),
 ('prt_e4', 'msg_e3', '` + dbSession + `', 5, 5, '{"type":"text","text":"Added GET /health returning 200."}');
`

func main() {
	fixture := flag.String("fixture", defaultFixture, "fixture user home to copy")
	schema := flag.String("schema", defaultSchema, "recorded opencode.db schema")
	flag.Parse()
	if err := runMain(os.Stdout, *fixture, *schema); err != nil {
		fmt.Fprintln(os.Stderr, "opencode:", err)
		os.Exit(1)
	}
}

// runMain copies the fixture into a temp dir and runs every section.
// Disk · Write (temp dir).
func runMain(w io.Writer, fixture, schema string) error {
	tmp, err := os.MkdirTemp("", "agentio-opencode-")
	if err != nil {
		return err
	}
	// The temp dir holds only copies; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	h, err := setup(tmp, fixture, schema)
	if err != nil {
		return err
	}
	return run(context.Background(), w, h)
}

// setup copies fixture to <tmp>/home, lays out an npm install of
// opencode-ai in <tmp>/home/npm and builds opencode.db in the data dir
// from the recorded schema plus seed. Symlinks are resolved first (macOS
// /var -> /private/var) so printed paths shorten to ~. Disk · Write.
func setup(tmp, fixture, schema string) (agentio.Host, error) {
	tmp, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		return agentio.Host{}, err
	}
	home := filepath.Join(tmp, "home")
	if err := agentio.CopyTree(fixture, home); err != nil {
		return agentio.Host{}, err
	}
	pkg := filepath.Join(home, "npm", "lib", "node_modules", opencode.Package)
	script := filepath.Join(pkg, "bin", opencode.Binary)
	bin := filepath.Join(home, "npm", "bin")
	for _, d := range []string{filepath.Dir(script), bin} {
		if err := os.MkdirAll(d, dirPerm); err != nil {
			return agentio.Host{}, err
		}
	}
	manifest := `{"name":"` + opencode.Package + `","version":"` + installedVersion + `"}`
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(manifest), filePerm); err != nil {
		return agentio.Host{}, err
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env node\n"), exePerm); err != nil {
		return agentio.Host{}, err
	}
	if err := os.Symlink(script, filepath.Join(bin, opencode.Binary)); err != nil {
		return agentio.Host{}, err
	}
	h := agentio.Host{UserHome: home, User: fixtureUser, Env: []string{"HOME=" + home, "PATH=" + bin}}
	a := opencode.New(h)
	db, _ := a.DatabasePath(a.DefaultHome())
	if err := buildDB(db, schema); err != nil {
		return agentio.Host{}, err
	}
	return h, nil
}

// buildDB writes opencode.db from the schema file and seed. Demo-only
// writer: opencode/store itself never opens the file writable.
// Disk · Write.
func buildDB(path, schema string) error {
	ddl, err := os.ReadFile(schema)
	if err != nil {
		return err
	}
	db, err := sql.Open(driverName, path)
	if err != nil {
		return err
	}
	// The writer is done after these statements; a close error would only
	// hide the statement error returned below.
	defer func() { _ = db.Close() }()
	for _, s := range []string{string(ddl), seed} {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// printer prints lines with the temp user home shortened to ~.
type printer struct {
	w    io.Writer
	home string
}

// line prints one line. Disk · Write (the writer).
func (p printer) line(s string) {
	_, _ = fmt.Fprintln(p.w, strings.ReplaceAll(s, p.home, homeTilde))
}

// run prints every section for opencode on h.
func run(ctx context.Context, w io.Writer, h agentio.Host) error {
	a := opencode.New(h)
	p := printer{w: w, home: h.UserHome}
	steps := []func() error{
		func() error { return install(p, a) },
		func() error { return homes(p, a) },
		func() error { return legacy(ctx, p, a) },
		func() error { return database(ctx, p, a) },
		func() error { return launch(p, a) },
		func() error { return processes(p, a) },
		func() error { doctor(ctx, p, a); return nil },
		func() error { capabilities(p, a); return nil },
	}
	for _, s := range steps {
		if err := s(); err != nil {
			return err
		}
	}
	return nil
}

// install reads opencode's install from disk: the PATH hit, the file it
// links to, and the version from the npm package.json above it.
func install(p printer, a *opencode.Agent) error {
	p.line("== install ==")
	in, err := a.Install()
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("install: %s -> %s method=%s version=%s tested=%s..%s", in.Path, in.RealPath, in.Method, in.Version, opencode.TestedMin, opencode.TestedMax))
	return nil
}

// snippet:start homes
// homes shows the data dir (the home: XDG_DATA_HOME/opencode, else
// ~/.local/share/opencode), the config dir and the database file.
func homes(p printer, a *opencode.Agent) error {
	p.line("== homes ==")
	hs, err := a.Homes()
	if err != nil {
		return err
	}
	for _, h := range hs {
		db, _ := a.DatabasePath(h)
		p.line(fmt.Sprintf("home: %s default=%t dir=%s", h.Name(), h.IsDefault(), h.Dir()))
		p.line(fmt.Sprintf("  database=%s config=%s", db, a.ConfigDir()))
	}
	return nil
}

// snippet:end homes

// snippet:start legacy
// legacy lists the sessions of the storage/ tree that opencode 1.2.0
// migrated into opencode.db, then reads one session's turns twice: the
// second read resumes from the cursor and finds nothing new.
func legacy(ctx context.Context, p printer, a *opencode.Agent) error {
	p.line("== legacy storage ==")
	h := a.DefaultHome()
	ss, err := a.Sessions(ctx, h, agentio.ConversationQuery{})
	if err != nil {
		return err
	}
	for _, s := range ss {
		p.line(fmt.Sprintf("session %s %q dir=%s v%s", s.ID, s.Title, s.Directory, s.Version))
	}
	show := func(t agentio.Turn) error {
		tokens := ""
		if t.Tokens != nil {
			tokens = fmt.Sprintf(" in=%d cached=%d out=%d reasoning=%d", t.Tokens.Input, t.Tokens.CachedInput, t.Tokens.Output, t.Tokens.Reasoning)
		}
		p.line(fmt.Sprintf("  %s %s %q%s", t.ID, t.Role, t.Text, tokens))
		return nil
	}
	cursor, err := a.Turns(h, legacySession, 0, show)
	if err != nil {
		return err
	}
	again, err := a.Turns(h, legacySession, cursor, show)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("  cursor=%d, re-read from it: cursor=%d", cursor, again))
	return nil
}

// snippet:end legacy

// snippet:start store
// database opens opencode.db read-only with opencode/store, lists the
// resumable sessions with opencode's own totals, and reads one session's
// turns. The newest assistant message is still streaming, so the cursor
// stops before it.
func database(ctx context.Context, p printer, a *opencode.Agent) error {
	p.line("== opencode.db ==")
	db, err := store.OpenHome(a, a.DefaultHome())
	if err != nil {
		return err
	}
	// Read-only handle: a close error loses nothing.
	defer func() { _ = db.Close() }()
	ss, err := db.Sessions(ctx, store.SessionQuery{Roots: true, Active: true})
	if err != nil {
		return err
	}
	for _, s := range ss {
		p.line(fmt.Sprintf("session %s %q dir=%s v%s agent=%s cost=$%.2f in=%d out=%d", s.ID, s.Title, s.Directory, s.Version, s.Agent, s.Cost, s.Tokens.Input, s.Tokens.Output))
	}
	cursor, err := db.Turns(ctx, dbSession, 0, func(t agentio.Turn) error {
		p.line(fmt.Sprintf("  %s %s interim=%t model=%s %q", t.ID, t.Role, t.Interim, t.Model, t.Text))
		return nil
	})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("  cursor=%d (msg_e4 is still streaming)", cursor))
	return nil
}

// snippet:end store

// snippet:start launch
// launch builds commands through the neutral view: a new TUI run with a
// prompt, a resume, a headless run with full autonomy, and the levels
// opencode refuses.
func launch(p printer, a *opencode.Agent) error {
	p.line("== launch ==")
	n := a.Neutral()
	def := n.DefaultHome().Target()
	cmd, err := n.Launch(def, agentio.LaunchRequest{Prompt: "fix the failing tests", Model: "openai/gpt-5"})
	if err != nil {
		return err
	}
	p.line("new: " + cmd.String())
	cmd, err = n.Launch(def, agentio.LaunchRequest{ResumeID: legacySession, Dir: "/home/alice/proj"})
	if err != nil {
		return err
	}
	p.line("resume: " + cmd.String())
	full := def
	full.AllowDangerous = true
	cmd, err = n.Launch(full, agentio.LaunchRequest{Headless: true, Autonomy: agentio.AutonomyFull, Prompt: "run the tests"})
	if err != nil {
		return err
	}
	p.line("headless full: " + cmd.String())
	if _, err := n.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyAsk}); err != nil {
		p.line("ask: " + err.Error())
	}
	return nil
}

// snippet:end launch

// snippet:start processes
// processes classifies a few command lines and plans a stop for the TUI.
func processes(p printer, a *opencode.Agent) error {
	p.line("== processes ==")
	n := a.Neutral()
	rows := []agentio.Process{
		{PID: 101, Argv: []string{"/home/u/.opencode/bin/opencode", "-s", legacySession}},
		{PID: 102, Argv: []string{"opencode", "run", "fix it"}},
		{PID: 103, Argv: []string{"opencode", "serve", "--port", "4096"}},
		{PID: 104, Argv: []string{"node", "/usr/lib/node_modules/opencode-ai/bin/opencode"}},
	}
	for _, r := range rows {
		info, ok := a.ClassifyProcess(r)
		p.line(fmt.Sprintf("pid %d: opencode=%t kind=%s session=%t", r.PID, ok, info.Kind, info.Session))
	}
	planner, ok := agentio.StopPlannerOf(n)
	if !ok {
		return fmt.Errorf("opencode has no stop planner")
	}
	s := agentio.ProcessSession{Agent: opencode.ID, PID: 101, Home: n.DefaultHome(), ConversationID: legacySession, Dir: "/home/alice/proj"}
	plan, err := planner.StopPlan(rows[0], rows, s)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("stop 101: targets=%v signal=%s grace=%s resume=%s", plan.Targets, plan.Graceful, plan.Grace, plan.Resume))
	plan, err = planner.StopPlan(rows[2], rows, agentio.ProcessSession{})
	if err != nil {
		return err
	}
	p.line("stop 103: refused: " + plan.Refuse)
	return nil
}

// snippet:end processes

// doctor prints every check.
func doctor(ctx context.Context, p printer, a *opencode.Agent) {
	p.line("== doctor ==")
	for _, c := range a.Checks(ctx, a.DefaultHome()) {
		p.line(fmt.Sprintf("%-15s %-5s %s", c.ID, c.Status, c.Message))
	}
}

// capabilities prints what the neutral view supports and why it lacks the
// rest (a few lines).
func capabilities(p printer, a *opencode.Agent) {
	p.line("== capabilities ==")
	n := a.Neutral()
	p.line(fmt.Sprintf("supported: %v", agentio.Supported(n)))
	for _, c := range []agentio.Capability{agentio.CapUsage, agentio.CapHooks, agentio.CapConfig} {
		reason, _ := agentio.Unsupported(n, c)
		p.line(fmt.Sprintf("lacks %s: %s", c, reason))
	}
}
