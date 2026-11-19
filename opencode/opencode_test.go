package opencode

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/opencode/internal/featuregen"
)

// fixtureUserHome is the 1.18.30 fixture: a user home whose data dir holds
// a legacy storage/ tree with one session in the 0.14.7 shape and one in
// the 1.1.14 shape (every value replaced).
const fixtureUserHome = "testdata/homes/1.18.30/userhome"

// Fixture sessions.
const (
	sessionA = "ses_0f0000000001ffeFixtureAAAA"
	sessionB = "ses_0f0000000002ffeFixtureBBBB"
	dirA     = "/Users/alice/proj"
	dirB     = "/Users/alice/other"
)

// newAgent copies the fixture into a temp user home.
func newAgent(t *testing.T, env ...string) (*Agent, agentio.Host) {
	t.Helper()
	host := agenttest.NewHost(t)
	agenttest.CopyFixture(t, fixtureUserHome, host.UserHome)
	host.Env = append(host.Env, env...)
	return New(host), host
}

// envVals lists the values of key in env.
func envVals(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			out = append(out, v)
		}
	}
	return out
}

func TestSpecValidates(t *testing.T) {
	t.Parallel()
	if err := Spec().Validate(); err != nil {
		t.Fatalf("spec: %v", err)
	}
	c := Compat()
	if c.Min.String() != TestedMin || c.Max.String() != TestedMax {
		t.Fatalf("compat %v", c)
	}
	if New(agenttest.NewHost(t)).core == nil {
		t.Fatal("nil core")
	}
}

// TestFeaturesDrift regenerates features_gen.go in memory.
func TestFeaturesDrift(t *testing.T) {
	t.Parallel()
	tsv, err := os.ReadFile("features.tsv")
	if err != nil {
		t.Fatal(err)
	}
	want, err := featuregen.Generate(tsv)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("features_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("features_gen.go is stale: run go generate ./opencode")
	}
	if span := FeatureInfo(FeatureDatabase); span.Since.String() != "1.2.0" || FeatureSource(FeatureDatabase) == "" {
		t.Fatalf("database span %v", span)
	}
	if span := FeatureInfo(FeatureLegacyStorage); span.Until.String() != "1.2.0" {
		t.Fatalf("legacy span %v", span)
	}
	if _, err := featuregen.Parse([]byte("X\tx\t1.0.0\n")); !errors.Is(err, featuregen.ErrNoSource) {
		t.Fatalf("bound without source: %v", err)
	}
}

// TestConformance checks opencode's own capability declarations; the
// shared agenttest suite runs in TestSharedConformance.
func TestConformance(t *testing.T) {
	a, _ := newAgent(t)
	n := a.Neutral()
	sup := agentio.Supported(n)
	for _, c := range agentio.CapabilityValues() {
		has := slices.Contains(sup, c)
		reason, declared := agentio.Unsupported(n, c)
		if has == declared {
			t.Errorf("%s: supported=%v declared=%v (%q)", c, has, declared, reason)
		}
		if declared && reason == unsupportedReason(agentio.Capability(255)) {
			t.Errorf("%s: generic reason %q", c, reason)
		}
	}
}

func TestSharedConformance(t *testing.T) {
	a, _ := newAgent(t)
	agenttest.Conformance(t, a.Neutral(), a.DefaultHome().Neutral())
}

func TestHomes(t *testing.T) {
	t.Parallel()
	a, host := newAgent(t)
	def := filepath.Join(host.UserHome, ".local", "share", "opencode")
	hs, err := a.Homes()
	if err != nil || len(hs) != 1 || !hs[0].IsDefault() || hs[0].Dir() != def || hs[0].Name() != agentio.DefaultHomeName {
		t.Fatalf("homes = %v %v", hs, err)
	}
	if _, ok := a.HomeFromEnv(); ok {
		t.Fatal("HomeFromEnv without XDG_DATA_HOME")
	}
	if a.HomeAt(def) != a.DefaultHome() || a.HomeAt(".local/share/opencode") != a.DefaultHome() {
		t.Fatal("HomeAt(default) is not the default home")
	}
	if a.ConfigDir() != filepath.Join(host.UserHome, ".config", "opencode") {
		t.Fatalf("config dir %s", a.ConfigDir())
	}
	// XDG_DATA_HOME moves the default data dir; a relative value is ignored.
	alt := filepath.Join(host.UserHome, "xdg")
	b, _ := newAgent(t, XDGDataHomeEnv+"="+alt, XDGConfigHomeEnv+"="+filepath.Join(host.UserHome, "cfg"))
	h, ok := b.HomeFromEnv()
	if !ok || !h.IsDefault() || h.Dir() != filepath.Join(alt, "opencode") || b.ConfigDir() != filepath.Join(host.UserHome, "cfg", "opencode") {
		t.Fatalf("home from env = %v %v %s", h, ok, b.ConfigDir())
	}
	if hs, err := b.Homes(); err != nil || len(hs) != 0 {
		t.Fatalf("xdg homes without markers = %v %v", hs, err)
	}
	other := b.HomeAt(def)
	if other.IsDefault() || other.Name() != "share" {
		t.Fatalf("old default under XDG: %v %s", other, other.Name())
	}
	c, ch := newAgent(t, XDGDataHomeEnv+"=rel/dir")
	if c.DataDir() != filepath.Join(ch.UserHome, ".local", "share", "opencode") {
		t.Fatalf("relative XDG_DATA_HOME used: %s", c.DataDir())
	}
	if _, err := a.HomeOf(agentio.NewHome("x", "/x", false, "x")); !errors.Is(err, agentio.ErrForeignHome) {
		t.Fatalf("foreign home: %v", err)
	}
	if ok, err := a.IsHome(host.UserHome); ok || err != nil {
		t.Fatalf("user home is a home: %v %v", ok, err)
	}
	n := a.Neutral()
	if nh, err := n.Homes(); err != nil || len(nh) != 1 || n.DefaultHome() != a.DefaultHome().Neutral() {
		t.Fatalf("neutral homes %v %v", nh, err)
	}
	if ok, err := n.IsHome(def); !ok || err != nil {
		t.Fatalf("neutral IsHome %v %v", ok, err)
	}
	if _, ok := n.HomeFromEnv(); ok {
		t.Fatal("neutral HomeFromEnv")
	}
}

func TestLaunch(t *testing.T) {
	t.Parallel()
	a, host := newAgent(t)
	def := a.DefaultHome().Target()
	for _, tc := range []struct {
		name string
		t    agentio.Target
		r    agentio.LaunchRequest
		want []string
	}{
		{"new", def, agentio.LaunchRequest{}, nil},
		{"resume", def, agentio.LaunchRequest{ResumeID: sessionA, Model: "openai/gpt-5"}, []string{"--session", sessionA, "--model", "openai/gpt-5"}},
		{"prompt", def, agentio.LaunchRequest{Prompt: "fix it"}, []string{"--prompt", "fix it"}},
		{"headless", def, agentio.LaunchRequest{Headless: true, Prompt: "fix it", ResumeID: sessionA}, []string{"run", "--format", "json", "--session", sessionA, "fix it"}},
		{"headless dash", def, agentio.LaunchRequest{Headless: true, Prompt: "-v please", Model: "a/b"}, []string{"run", "--format", "json", "--model", "a/b", "--", "-v please"}},
		{"full", agentio.Target{Home: a.DefaultHome().Neutral(), AllowDangerous: true}, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}, []string{"--auto"}},
		{"full headless", agentio.Target{Home: a.DefaultHome().Neutral(), AllowDangerous: true}, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull, Headless: true}, []string{"run", "--format", "json", "--auto"}},
	} {
		cmd, err := a.Launch(tc.t, tc.r)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !slices.Equal(cmd.Args, tc.want) || cmd.Path != Binary {
			t.Errorf("%s: args %q path %s, want %q", tc.name, cmd.Args, cmd.Path, tc.want)
		}
		if vals := envVals(cmd.Env, XDGDataHomeEnv); len(vals) != 0 {
			t.Errorf("%s: default home sets %s=%v", tc.name, XDGDataHomeEnv, vals)
		}
	}
	for name, r := range map[string]agentio.LaunchRequest{
		"ask":     {Autonomy: agentio.AutonomyAsk},
		"add-dir": {AddDirs: []string{"/x"}},
		"token":   {Credential: &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken("sk-fixture")}},
	} {
		if _, err := a.Launch(def, r); !errors.Is(err, agentio.ErrUnsupported) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := a.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}); !errors.Is(err, agentio.ErrDangerousNotAllowed) {
		t.Errorf("full without consent: %v", err)
	}
	if _, err := a.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull, Headless: true}); !errors.Is(err, agentio.ErrDangerousNotAllowed) {
		t.Errorf("full headless without consent: %v", err)
	}
	// Another data dir sets XDG_DATA_HOME to its base dir.
	alt := a.HomeAt(filepath.Join(host.UserHome, "alt", "opencode"))
	cmd, err := a.Launch(alt.Target(), agentio.LaunchRequest{})
	if err != nil || !slices.Equal(envVals(cmd.Env, XDGDataHomeEnv), []string{filepath.Join(host.UserHome, "alt")}) {
		t.Fatalf("alt home: %v %v", envVals(cmd.Env, XDGDataHomeEnv), err)
	}
	if _, err := a.Launch(a.HomeAt(filepath.Join(host.UserHome, "elsewhere")).Target(), agentio.LaunchRequest{}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Fatalf("unselectable home: %v", err)
	}
	if _, err := a.Launch(agentio.NewHome("x", "/x", false, "x").Target(), agentio.LaunchRequest{}); !errors.Is(err, agentio.ErrForeignHome) {
		t.Fatalf("foreign home: %v", err)
	}
	// A version that provably lacks a flag is refused.
	old := def
	old.Version = agentio.MustVersion("1.0.0")
	if _, err := a.Launch(old, agentio.LaunchRequest{ResumeID: "x"}); err != nil {
		t.Fatalf("unknown-bound flag refused: %v", err)
	}
	if err := checkFeature(agentio.MustVersion("1.1.0"), FeatureFlagFork); !errors.Is(err, agentio.ErrFeatureUnavailable) || !strings.Contains(err.Error(), "needs opencode >= 1.1.54") {
		t.Fatalf("fork on 1.1.0: %v", err)
	}
	if err := checkFeature(agentio.MustVersion("1.3.0"), FeatureLegacyStorage); err == nil || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("legacy on 1.3.0: %v", err)
	}
	b, _ := FeatureFlagFork.MarshalText()
	if string(b) != "--fork" || len(Features()) == 0 || FeatureUnknown.String() != "unknown" {
		t.Fatalf("feature text %s", b)
	}
}

func TestSessions(t *testing.T) {
	t.Parallel()
	a, _ := newAgent(t)
	h := a.DefaultHome()
	ctx := context.Background()
	ss, err := a.Sessions(ctx, h, agentio.ConversationQuery{})
	if err != nil || len(ss) != 2 || ss[0].ID != sessionB || ss[1].ID != sessionA {
		t.Fatalf("sessions = %v %v", ss, err)
	}
	s := ss[1]
	if s.Version != "0.14.7" || s.Directory != dirA || s.Title != "Fix the login form" || s.Share == nil || s.Share.URL == "" || s.ProjectID != "global" {
		t.Fatalf("session A = %+v", s)
	}
	c := s.Neutral(h)
	if c.Agent != ID || c.ID != sessionA || c.Dir != dirA || c.Created.IsZero() || !c.Updated.After(c.Created) || c.Path != s.Path {
		t.Fatalf("conversation = %+v", c)
	}
	if only, _ := a.Sessions(ctx, h, agentio.ConversationQuery{Dir: dirA + "/"}); len(only) != 1 || only[0].ID != sessionA {
		t.Fatalf("dir filter = %v", only)
	}
	if lim, _ := a.Sessions(ctx, h, agentio.ConversationQuery{Limit: 1}); len(lim) != 1 || lim[0].ID != sessionB {
		t.Fatalf("limit = %v", lim)
	}
	if since, _ := a.Sessions(ctx, h, agentio.ConversationQuery{Since: ss[0].Time.Updated.Time()}); len(since) != 1 {
		t.Fatalf("since = %v", since)
	}
	if ok, err := a.HasHistory(ctx, h, dirB); !ok || err != nil {
		t.Fatalf("history B: %v %v", ok, err)
	}
	if ok, _ := a.HasHistory(ctx, h, "/nowhere"); ok {
		t.Fatal("history for an unknown dir")
	}
	if _, err := a.Session(h, "ses_missing"); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("missing session: %v", err)
	}
	n := a.Neutral()
	cs, err := n.(agentio.ConversationLister).Conversations(ctx, h.Neutral(), agentio.ConversationQuery{})
	if err != nil || len(cs) != 2 {
		t.Fatalf("neutral conversations %v %v", cs, err)
	}
	if one, err := n.(agentio.ConversationLister).Conversation(h.Neutral(), sessionA); err != nil || one.Title != s.Title {
		t.Fatalf("neutral conversation %v %v", one, err)
	}
	if ok, err := n.(agentio.HistoryChecker).HasHistory(h.Neutral(), dirA); !ok || err != nil {
		t.Fatalf("neutral history %v %v", ok, err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := a.Sessions(cctx, h, agentio.ConversationQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
	if n, found, err := h.StorageMigration(); n != 2 || !found || err != nil {
		t.Fatalf("migration %d %v %v", n, found, err)
	}
}

// collect reads every turn from cursor from.
func collect(t *testing.T, read func(agentio.Offset, func(agentio.Turn) error) (agentio.Offset, error), from agentio.Offset) ([]agentio.Turn, agentio.Offset) {
	t.Helper()
	var out []agentio.Turn
	off, err := read(from, func(tu agentio.Turn) error {
		out = append(out, tu)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out, off
}

func TestTurns(t *testing.T) {
	t.Parallel()
	a, _ := newAgent(t)
	h := a.DefaultHome()
	read := func(id string) func(agentio.Offset, func(agentio.Turn) error) (agentio.Offset, error) {
		return func(from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
			return a.Turns(h, id, from, fn)
		}
	}
	ts, off := collect(t, read(sessionA), 0)
	if len(ts) != 4 || off != 4 {
		t.Fatalf("A turns = %d off %d", len(ts), off)
	}
	u, as := ts[0], ts[1]
	if u.Role != agentio.RoleUser || u.Text != "fix the login form" || u.ID != "msg_0f0000000001AAAAAAAAAAAA01" || u.ConversationID != sessionA || u.Dir != dirA || u.AgentVersion != "0.14.7" || u.Tokens != nil {
		t.Fatalf("user turn %+v", u)
	}
	want := agentio.TokenCounts{Input: 1200, CachedInput: 1000, Output: 300, Reasoning: 120}
	if as.Role != agentio.RoleAssistant || as.Text != "I fixed the login form." || as.Model != "openai/gpt-5-nano" || as.Tokens == nil || *as.Tokens != want || as.Interim || len(as.Raw) == 0 {
		t.Fatalf("assistant turn %+v", as)
	}
	if ts[2].Text != "now add a test" {
		t.Fatalf("synthetic text kept: %q", ts[2].Text)
	}
	if ts[3].Tokens.CacheWrite != 10 {
		t.Fatalf("cache write %+v", ts[3].Tokens)
	}
	if more, again := collect(t, read(sessionA), off); len(more) != 0 || again != off {
		t.Fatalf("re-read %d %d", len(more), again)
	}
	// B: the newest assistant message never completed, so the cursor stops
	// before it; the earlier abandoned one is passed because a message
	// follows it.
	tb, offB := collect(t, read(sessionB), 0)
	if len(tb) != 3 || offB != 3 || tb[0].Model != "ollama/gemma4" || tb[1].Role != agentio.RoleAssistant {
		t.Fatalf("B turns = %+v off %d", tb, offB)
	}
	if more, again := collect(t, read(sessionB), offB); len(more) != 0 || again != 3 {
		t.Fatalf("B re-read %d %d", len(more), again)
	}
	// A cursor past the end (a revert deleted messages) returns the count.
	if more, back := collect(t, read(sessionA), 9); len(more) != 0 || back != 4 {
		t.Fatalf("past end %d %d", len(more), back)
	}
	stop := errors.New("stop")
	if off, err := a.Turns(h, sessionA, 0, func(agentio.Turn) error { return stop }); !errors.Is(err, stop) || off != 1 {
		t.Fatalf("fn error: %v %d", err, off)
	}
	if _, err := a.Turns(h, "ses_missing", 0, func(agentio.Turn) error { return nil }); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	all, err := a.Messages(h, sessionA)
	if err != nil || len(all) != 4 || len(all[1].Parts) != 5 || all[1].Parts[2].Type != "tool" || all[1].Parts[2].Unknown["state"] == nil {
		t.Fatalf("messages %v", err)
	}
	if all[1].Info.Unknown != nil || all[1].Info.Path.Cwd != dirA || len(all[1].Info.System) != 2 {
		t.Fatalf("assistant info %+v", all[1].Info)
	}
	n := a.Neutral().(agentio.TurnReader)
	if nts, _ := collect(t, func(from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
		return n.Turns(h.Neutral(), sessionA, from, fn)
	}, 0); len(nts) != 4 {
		t.Fatalf("neutral turns %d", len(nts))
	}
}

func TestTurnRules(t *testing.T) {
	t.Parallel()
	rec := func(role, finish string, completed Millis, parts ...Part) MessageRecord {
		return MessageRecord{ID: "msg_x", SessionID: "ses_x", Info: Message{Role: role, Finish: finish, Time: MessageTime{Created: 1, Completed: completed}}, Parts: parts}
	}
	text := Part{Type: partText, Text: "t"}
	if rec(roleUser, "", 0).Complete(true) || !rec(roleUser, "", 0, text).Complete(true) || !rec(roleUser, "", 0).Complete(false) {
		t.Fatal("user completeness")
	}
	if rec(roleAssistant, "", 0).Complete(true) || !rec(roleAssistant, "", 5).Complete(true) {
		t.Fatal("assistant completeness")
	}
	aborted := rec(roleAssistant, "", 0)
	aborted.Info.Error = &MessageError{Name: "MessageAbortedError"}
	if !aborted.Complete(true) {
		t.Fatal("errored assistant is complete")
	}
	if tu, _ := rec(roleAssistant, finishToolCalls, 5, text).Turn(TurnContext{}); !tu.Interim || tu.StopReason != finishToolCalls {
		t.Fatalf("tool-calls turn %+v", tu)
	}
	if tu, _ := rec(roleAssistant, "stop", 5, text).Turn(TurnContext{}); tu.Interim {
		t.Fatalf("stop turn interim")
	}
	summary := rec(roleAssistant, "stop", 5, text)
	summary.Info.Summary = []byte("true")
	if tu, _ := summary.Turn(TurnContext{}); !tu.Interim || !summary.Info.IsCompactionSummary() {
		t.Fatal("compaction summary not interim")
	}
	if _, ok := rec("system", "", 5).Turn(TurnContext{}); ok {
		t.Fatal("unknown role became a turn")
	}
	cwd := rec(roleAssistant, "", 5)
	cwd.Info.Path = &MessagePath{Cwd: "/w"}
	if tu, _ := cwd.Turn(TurnContext{}); tu.Dir != "/w" {
		t.Fatalf("cwd fallback %q", tu.Dir)
	}
	if (Message{ModelID: "m"}).ModelName() != "m" || (Message{}).ModelName() != "" {
		t.Fatal("model name")
	}
	if !Millis(0).Time().IsZero() {
		t.Fatal("zero millis")
	}
	m, err := DecodeMessage([]byte(`{"role":"user","time":{"created":1},"variant":"high","tools":{"task":false}}`))
	if err != nil || !slices.Equal(m.Unknown.Keys(), []string{"tools", "variant"}) {
		t.Fatalf("unknown keys %v %v", m.Unknown.Keys(), err)
	}
	if _, err := DecodeMessage([]byte(`[]`)); err == nil {
		t.Fatal("array decoded as message")
	}
	if _, err := DecodePart([]byte(`"x"`)); err == nil {
		t.Fatal("string decoded as part")
	}
}

func TestTurnFiles(t *testing.T) {
	t.Parallel()
	a, host := newAgent(t)
	h := a.DefaultHome()
	s, err := a.Session(h, sessionA)
	if err != nil {
		t.Fatal(err)
	}
	msgDir := h.Path("storage", "message", sessionA)
	for _, p := range []string{s.Path, msgDir, filepath.Join(msgDir, "msg_0f0000000001AAAAAAAAAAAA01.json")} {
		ref, err := a.TranscriptOwner(p)
		if err != nil || ref.ConversationID != sessionA || ref.Home != h.Neutral() || ref.Agent != ID {
			t.Fatalf("owner(%s) = %+v %v", p, ref, err)
		}
		ts, off := collect(t, func(from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
			return a.TurnsAt(p, from, fn)
		}, 0)
		if len(ts) != 4 || off != 4 || ts[0].Dir != dirA {
			t.Fatalf("TurnsAt(%s) = %d %d", p, len(ts), off)
		}
	}
	for _, p := range []string{
		host.UserHome,
		filepath.Join(host.UserHome, "x", "message", sessionA),
		filepath.Join(host.UserHome, "storage", "other", sessionA),
		filepath.Join(host.UserHome, "storage", "session", "global", "notes.json"),
		filepath.Join(host.UserHome, "storage", "x", "global", sessionA+".json"),
	} {
		if _, err := a.TranscriptOwner(p); !errors.Is(err, agentio.ErrNotFound) {
			t.Errorf("owner(%s) err = %v", p, err)
		}
		if _, err := a.TurnsAt(p, 0, func(agentio.Turn) error { return nil }); !errors.Is(err, agentio.ErrNotFound) {
			t.Errorf("TurnsAt(%s) err = %v", p, err)
		}
	}
	// A message dir whose session file is gone still reads, without a dir.
	if err := os.Remove(s.Path); err != nil {
		t.Fatal(err)
	}
	ts, _ := collect(t, func(from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
		return a.TurnsAt(msgDir, from, fn)
	}, 0)
	if len(ts) != 4 || ts[0].Dir != "" || ts[1].Dir != dirA {
		t.Fatalf("orphan dir turns %d", len(ts))
	}
	n := a.Neutral().(agentio.TurnFileReader)
	if ref, err := n.TranscriptOwner(msgDir); err != nil || ref.ConversationID != sessionA {
		t.Fatalf("neutral owner %v", err)
	}
	if _, err := n.TurnsAt(msgDir, 0, func(agentio.Turn) error { return nil }); err != nil {
		t.Fatalf("neutral TurnsAt %v", err)
	}
}

func TestBrokenStorage(t *testing.T) {
	t.Parallel()
	a, _ := newAgent(t)
	h := a.DefaultHome()
	// A broken session file is skipped in listings, an error by id.
	agenttest.WriteFile(t, h.Path("storage", "session", "global"), "ses_0f0000000009ffeBroken.json", "{")
	if ss, err := a.Sessions(context.Background(), h, agentio.ConversationQuery{}); err != nil || len(ss) != 2 {
		t.Fatalf("broken skipped: %v %v", ss, err)
	}
	if _, err := a.Session(h, "ses_0f0000000009ffeBroken"); err == nil {
		t.Fatal("broken session by id")
	}
	// A broken part is an error for the turns read.
	agenttest.WriteFile(t, h.Path("storage", "part", "msg_0f0000000004AAAAAAAAAAAA04"), "prt_zz.json", "[")
	if _, err := a.Turns(h, sessionA, 0, func(agentio.Turn) error { return nil }); err == nil {
		t.Fatal("broken part read")
	}
	agenttest.WriteFile(t, h.Path("storage", "message", sessionB), "msg_0f0000000019BBBBBBBBBBBB09.json", "[")
	if _, err := a.Turns(h, sessionB, 0, func(agentio.Turn) error { return nil }); err == nil {
		t.Fatal("broken message read")
	}
	agenttest.WriteFile(t, h.StorageDir(), "migration", "x")
	if _, found, err := h.StorageMigration(); !found || err == nil {
		t.Fatal("broken migration file")
	}
	empty := New(agenttest.NewHost(t))
	if n, found, err := empty.DefaultHome().StorageMigration(); n != 0 || found || err != nil {
		t.Fatal("missing migration file")
	}
	if ss, err := empty.Sessions(context.Background(), empty.DefaultHome(), agentio.ConversationQuery{}); err != nil || len(ss) != 0 {
		t.Fatalf("empty home sessions %v %v", ss, err)
	}
}

func TestDatabasePath(t *testing.T) {
	t.Parallel()
	a, host := newAgent(t)
	h := a.DefaultHome()
	if p, ok := a.DatabasePath(h); !ok || p != h.Path("opencode.db") {
		t.Fatalf("default db %s %v", p, ok)
	}
	abs := filepath.Join(host.UserHome, "db", "x.db")
	for env, rel := range map[string]bool{abs: false, "custom.db": true} {
		b, _ := newAgent(t, DBEnv+"="+env)
		want := env
		if rel {
			want = b.DefaultHome().Path(env)
		}
		if p, ok := b.DatabasePath(b.DefaultHome()); !ok || p != want {
			t.Errorf("OPENCODE_DB=%s: %s %v", env, p, ok)
		}
	}
	m, _ := newAgent(t, DBEnv+"=:memory:")
	if _, ok := m.DatabasePath(m.DefaultHome()); ok {
		t.Fatal("memory db on disk")
	}
	for ch, want := range map[string]string{"latest": "opencode.db", "beta": "opencode.db", "prod": "opencode.db", "dev": "opencode-dev.db", "feat/x y": "opencode-feat-x-y.db"} {
		if got := ChannelDatabasePath(h, ch); got != h.Path(want) {
			t.Errorf("channel %s: %s", ch, got)
		}
	}
}

func TestClassifyProcess(t *testing.T) {
	t.Parallel()
	a, _ := newAgent(t)
	for _, tc := range []struct {
		argv    []string
		kind    agentio.ProcessKind
		session bool
	}{
		{[]string{"/Users/alice/.opencode/bin/opencode"}, agentio.ProcessCLI, true},
		{[]string{"opencode", "/Users/alice/proj"}, agentio.ProcessCLI, true},
		{[]string{"opencode", "--model", "run", "-s", "ses_1"}, agentio.ProcessCLI, true},
		{[]string{"opencode", "run", "hello"}, agentio.ProcessExec, true},
		{[]string{"opencode", "--print-logs", "run", "hello"}, agentio.ProcessExec, true},
		{[]string{`C:\Users\alice\opencode.exe`, "serve"}, agentio.ProcessAppServer, false},
		{[]string{"opencode", "acp"}, agentio.ProcessAppServer, false},
		{[]string{"opencode", "attach", "http://localhost:4096"}, agentio.ProcessBridge, false},
		{[]string{"opencode", "providers", "list"}, agentio.ProcessLauncher, false},
		{[]string{"opencode", "session", "list"}, agentio.ProcessLauncher, false},
		{[]string{"node", "/usr/local/lib/node_modules/opencode-ai/bin/opencode"}, agentio.ProcessLauncher, false},
		{[]string{"bun", "/Users/alice/.bun/bin/opencode", "run", "x"}, agentio.ProcessLauncher, false},
	} {
		info, ok := a.ClassifyProcess(agentio.Process{PID: 7, Argv: tc.argv})
		if !ok || info.Agent != ID || info.Kind != tc.kind || info.Session != tc.session {
			t.Errorf("%q = %+v %v, want %s session=%v", tc.argv, info, ok, tc.kind, tc.session)
		}
	}
	for _, argv := range [][]string{nil, {"opencode-helper"}, {"node", "/x/claude"}, {"vim", "opencode"}} {
		if info, ok := a.ClassifyProcess(agentio.Process{Argv: argv}); ok {
			t.Errorf("%q classified %+v", argv, info)
		}
	}
	if info, ok := a.Neutral().(agentio.ProcessClassifier).ClassifyProcess(agentio.Process{Argv: []string{"opencode"}}); !ok || !info.Session {
		t.Fatal("neutral classify")
	}
}

func TestProcessSession(t *testing.T) {
	t.Parallel()
	procs := &agenttest.FakeProcesses{Cwds: map[int]string{11: "/Users/alice/cwd", 12: "/w"}}
	host := agenttest.NewHost(t, agenttest.WithProcesses(procs))
	agenttest.CopyFixture(t, fixtureUserHome, host.UserHome)
	a := New(host)
	ctx := context.Background()
	s, found, err := a.ProcessSession(ctx, agentio.Process{PID: 11, Argv: []string{"opencode", "-s", sessionA}})
	if err != nil || !found || s.ConversationID != sessionA || s.Confidence != agentio.JoinArgv || s.Dir != dirA || s.Version != "0.14.7" || s.Path == "" || s.Home != a.DefaultHome().Neutral() {
		t.Fatalf("session -s = %+v %v %v", s, found, err)
	}
	s, _, _ = a.ProcessSession(ctx, agentio.Process{PID: 11, Argv: []string{"opencode", "run", "--session=ses_new", "x"}})
	if s.ConversationID != "ses_new" || s.Path != "" || s.Dir != "/Users/alice/cwd" {
		t.Fatalf("session= %+v", s)
	}
	s, _, _ = a.ProcessSession(ctx, agentio.Process{PID: 12, Argv: []string{"opencode", "--session", sessionA, "--fork"}})
	if s.ConversationID != "" || s.Confidence != agentio.JoinUnknown || s.Dir != "/w" {
		t.Fatalf("fork %+v", s)
	}
	if _, found, _ := a.ProcessSession(ctx, agentio.Process{PID: 13, Argv: []string{"opencode", "serve"}}); found {
		t.Fatal("server is a session")
	}
	// A readable start env names the data dir.
	alt := filepath.Join(host.UserHome, "xdg")
	procs.Envs = map[int][]string{11: {"HOME=" + host.UserHome, XDGDataHomeEnv + "=" + alt}, 12: {"HOME=" + host.UserHome}}
	s, _, _ = a.ProcessSession(ctx, agentio.Process{PID: 11, Argv: []string{"opencode"}})
	if s.Home.Dir() != filepath.Join(alt, "opencode") || s.Home.IsDefault() {
		t.Fatalf("env home %v", s.Home)
	}
	s, _, _ = a.ProcessSession(ctx, agentio.Process{PID: 12, Argv: []string{"opencode"}})
	if s.Home != a.DefaultHome().Neutral() {
		t.Fatalf("env without xdg %v", s.Home)
	}
	s, _, _ = a.ProcessSession(ctx, agentio.Process{PID: 14, Argv: []string{"opencode"}})
	if s.Home != a.DefaultHome().Neutral() {
		t.Fatalf("unreadable env %v", s.Home)
	}
	ns, found, err := a.Neutral().(agentio.ProcessSessionReader).ProcessSession(ctx, agentio.Process{PID: 11, Argv: []string{"opencode", "-s", "ses_x"}})
	if err != nil || !found || ns.ConversationID != "ses_x" {
		t.Fatalf("neutral process session %v", err)
	}
}

func TestStopPlan(t *testing.T) {
	t.Parallel()
	a, host := newAgent(t)
	n := a.Neutral().(agentio.StopPlanner)
	p := agentio.Process{PID: 21, Argv: []string{"opencode", "-s", sessionA}}
	plan, err := n.StopPlan(p, nil, agentio.ProcessSession{Agent: ID, PID: 21, Home: a.DefaultHome().Neutral(), ConversationID: sessionA, Dir: dirA})
	if err != nil || !slices.Equal(plan.Targets, []int{21}) || plan.Graceful != agentio.StopSignalTerminate || plan.Resume == nil || !slices.Contains(plan.Resume.Args, sessionA) {
		t.Fatalf("plan %+v %v", plan, err)
	}
	alt := a.HomeAt(filepath.Join(host.UserHome, "alt", "opencode"))
	plan, _ = n.StopPlan(p, nil, agentio.ProcessSession{Home: alt.Neutral(), ConversationID: sessionA})
	if plan.Resume == nil || len(envVals(plan.Resume.Env, XDGDataHomeEnv)) != 1 {
		t.Fatalf("alt resume %+v", plan.Resume)
	}
	bad := a.HomeAt(filepath.Join(host.UserHome, "bad"))
	if plan, _ = n.StopPlan(p, nil, agentio.ProcessSession{Home: bad.Neutral(), ConversationID: sessionA}); plan.Resume != nil {
		t.Fatal("resume on an unselectable home")
	}
	if plan, _ = n.StopPlan(agentio.Process{PID: 22, Argv: []string{"opencode", "serve"}}, nil, agentio.ProcessSession{}); plan.Refuse == "" || len(plan.Targets) != 0 {
		t.Fatalf("server plan %+v", plan)
	}
	if _, err := n.StopPlan(agentio.Process{PID: 23, Argv: []string{"vim"}}, nil, agentio.ProcessSession{}); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("foreign process: %v", err)
	}
}

func TestDoctor(t *testing.T) {
	t.Parallel()
	a, _ := newAgent(t)
	h := a.DefaultHome()
	byID := func(cs []agentio.Check) map[agentio.CheckID]agentio.Check {
		out := map[agentio.CheckID]agentio.Check{}
		for _, c := range cs {
			out[c.ID] = c
		}
		return out
	}
	cs := byID(a.Checks(context.Background(), h))
	if c := cs[CheckDatabase]; c.Status != agentio.CheckOK || !strings.Contains(c.Message, "2 legacy sessions") {
		t.Fatalf("database check %+v", c)
	}
	if cs[agentio.CheckHome].Status != agentio.CheckOK || cs[CheckAutoUpdate].Status != agentio.CheckInfo || cs[agentio.CheckVersionTested].Status != agentio.CheckWarn {
		t.Fatalf("checks %+v", cs)
	}
	agenttest.WriteFile(t, h.Dir(), "opencode.db", "")
	if c := byID(a.Checks(context.Background(), h))[CheckDatabase]; c.Status != agentio.CheckInfo || !strings.Contains(c.Message, "opencode/store") {
		t.Fatalf("db check %+v", c)
	}
	m, _ := newAgent(t, DBEnv+"=:memory:", DisableAutoUpdateEnv+"=1", XDGDataHomeEnv+"=/srv/xdg")
	mc := byID(m.Checks(context.Background(), m.DefaultHome()))
	if mc[CheckDatabase].Status != agentio.CheckWarn || mc[CheckDataHome].Status != agentio.CheckInfo {
		t.Fatalf("memory checks %+v", mc)
	}
	if _, ok := mc[CheckAutoUpdate]; ok {
		t.Fatal("auto-update reported with OPENCODE_DISABLE_AUTOUPDATE")
	}
	empty := New(agenttest.NewHost(t))
	if c := byID(empty.Checks(context.Background(), empty.DefaultHome()))[CheckDatabase]; c.Message != "no sessions yet" {
		t.Fatalf("empty %+v", c)
	}
	nc := a.Neutral().(agentio.Doctor).Checks(context.Background(), agentio.NewHome("x", "/x", false, "x"))
	if len(nc) != 1 || nc[0].Status != agentio.CheckFail {
		t.Fatalf("foreign doctor %+v", nc)
	}
}

func TestInstall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pkg := filepath.Join(dir, "lib", "node_modules", "opencode-ai")
	agenttest.WriteFile(t, pkg, "package.json", `{"name":"opencode-ai","version":"1.18.30"}`)
	bin := agenttest.WriteFile(t, pkg, "bin/"+agenttest.ExeName("opencode"), "#!/usr/bin/env node\n")
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	h := agenttest.NewHost(t, agenttest.WithEnv("PATH="+filepath.Dir(bin), "PATHEXT=.EXE"))
	in, err := New(h).Install()
	if err != nil || in.Version.String() != "1.18.30" || in.Method != agentio.InstallNPM {
		t.Fatalf("install %+v %v", in, err)
	}
	if _, err := New(agenttest.NewHost(t)).Install(); !errors.Is(err, agentio.ErrNotInstalled) {
		t.Fatalf("not installed: %v", err)
	}
}

func TestBasicsAndVersion(t *testing.T) {
	t.Parallel()
	exec := &agenttest.FakeExec{Default: agentio.ProcessResult{Stdout: []byte("1.18.30\n")}}
	h := agenttest.NewHost(t, agenttest.WithExec(exec))
	a := New(h)
	if a.ID() != ID || a.Name() != "opencode" || a.Host().UserHome != h.UserHome || a.Spec().Binary != Binary || !slices.Equal(HomeMarkers(), []string{"opencode.db", "storage"}) {
		t.Fatal("basics")
	}
	d := a.DefaultHome()
	if d.String() != "opencode:"+d.Dir() || d.Target().Home != d.Neutral() {
		t.Fatalf("home string %s", d)
	}
	if b, err := d.MarshalJSON(); err != nil || !strings.Contains(string(b), "opencode") {
		t.Fatalf("home json %s %v", b, err)
	}
	if _, err := a.LookPath(); !errors.Is(err, agentio.ErrNotInstalled) {
		t.Fatalf("look path %v", err)
	}
	v, err := a.Runner().BinaryVersion(context.Background())
	if err != nil || v.String() != "1.18.30" || len(exec.Calls) != 1 || !slices.Equal(exec.Calls[0].Args, []string{"--version"}) {
		t.Fatalf("version %v %v %v", v, err, exec.Calls)
	}
	bv, ok := agentio.BinaryVersionOf(a.Neutral())
	if !ok {
		t.Fatal("no binary version")
	}
	if v, err := bv.BinaryVersion(context.Background()); err != nil || v.String() != "1.18.30" {
		t.Fatalf("neutral version %v %v", v, err)
	}
	for _, f := range Features() {
		if f.String() == "unknown" {
			t.Errorf("feature %d has no spelling", f)
		}
		_ = FeatureSource(f)
	}
}
