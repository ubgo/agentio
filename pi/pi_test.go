package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/pi/internal/featuregen"
)

// fixtureUserHome is the 1.5.2 fixture: the files pi wrote on a first
// start with a throwaway HOME (auth.json and models-store.json, both {})
// plus one session recorded against a local mock provider.
const fixtureUserHome = "testdata/homes/1.5.2/userhome"

// fixtureSessionID and fixtureCwd are the recorded session's id and cwd.
const (
	fixtureSessionID = "0190f0e0-0000-7000-8000-000000000001"
	fixtureCwd       = "/home/fixture/proj"
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
		t.Fatal("features_gen.go is stale: run go generate ./pi")
	}
	if span := FeatureInfo(FeatureFlagNoMCP); span.Since.String() != "1.0.4" || FeatureSource(FeatureFlagNoMCP) == "" {
		t.Fatalf("no-mcp span %v", span)
	}
}

// TestConformance checks pi's own capability declarations; the shared
// agenttest suite runs in TestSharedConformance.
func TestConformance(t *testing.T) {
	a, _ := newAgent(t)
	n := a.Neutral()
	sup := agentio.Supported(n)
	for _, c := range agentio.CapabilityValues() {
		has := slices.Contains(sup, c)
		if reason, declared := agentio.Unsupported(n, c); has == declared {
			t.Errorf("%s: supported=%v declared=%v (%q)", c, has, declared, reason)
		}
	}
	cmd, err := n.Launch(n.DefaultHome().Target(), agentio.LaunchRequest{ResumeID: "abc"})
	if err != nil || !slices.Contains(cmd.Args, "abc") {
		t.Fatalf("resume: %v %v", cmd.Args, err)
	}
}

func TestHomes(t *testing.T) {
	t.Parallel()
	a, host := newAgent(t)
	hs, err := a.Homes()
	if err != nil || len(hs) != 1 || !hs[0].IsDefault() || hs[0].Dir() != filepath.Join(host.UserHome, ".pi", "agent") {
		t.Fatalf("homes = %v %v", hs, err)
	}
	alt := filepath.Join(host.UserHome, "alt-agent")
	agenttest.WriteFile(t, alt, "auth.json", "{}")
	b, _ := newAgent(t, HomeEnv+"="+alt)
	h, ok := b.HomeFromEnv()
	if !ok || h.Dir() != alt {
		t.Fatalf("home from env = %v %v", h, ok)
	}
	// The variable names the agent dir itself: launches set it as is.
	cmd, err := b.Neutral().Launch(h.Target(), agentio.LaunchRequest{})
	if err != nil || !slices.Equal(envVals(cmd.Env, HomeEnv), []string{alt}) {
		t.Fatalf("env = %v %v", cmd.Env, err)
	}
	cmd, err = a.Neutral().Launch(a.DefaultHome().Target(), agentio.LaunchRequest{})
	if err != nil || len(envVals(cmd.Env, HomeEnv)) != 0 {
		t.Fatalf("default home env = %v %v", cmd.Env, err)
	}
	if ok, err := a.IsHome(host.UserHome); err != nil || ok {
		t.Errorf("user home is not an agent dir: %v %v", ok, err)
	}
}

func TestLaunch(t *testing.T) {
	t.Parallel()
	a, _ := newAgent(t, "OPENAI_API_KEY=user-openai")
	n := a.Neutral()
	def := n.DefaultHome().Target()
	cmd, err := n.Launch(def, agentio.LaunchRequest{Prompt: "-list files", Model: "anthropic/claude-sonnet:high", Autonomy: agentio.AutonomyReadOnly, Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--mode", "json", "--model", "anthropic/claude-sonnet:high", "--tools", "read,grep,find,ls", "--no-mcp", "--no-extensions", "--", "-list files"}
	if cmd.Path != Binary || !slices.Equal(cmd.Args, want) {
		t.Errorf("args = %v, want %v", cmd.Args, want)
	}
	for _, level := range []agentio.Autonomy{agentio.AutonomyAsk, agentio.AutonomyAcceptEdits} {
		_, err := n.Launch(def, agentio.LaunchRequest{Autonomy: level})
		if !errors.Is(err, agentio.ErrUnsupported) || !strings.Contains(err.Error(), noPermissionModesReason) {
			t.Errorf("%s = %v", level, err)
		}
	}
	if _, err := n.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}); !errors.Is(err, agentio.ErrDangerousNotAllowed) {
		t.Errorf("full without AllowDangerous = %v", err)
	}
	full := def
	full.AllowDangerous = true
	if cmd, err := n.Launch(full, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}); err != nil || len(cmd.Args) != 0 {
		t.Errorf("full = %v %v", cmd.Args, err)
	}
	if _, err := n.Launch(def, agentio.LaunchRequest{AddDirs: []string{"x"}}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("add-dir = %v", err)
	}
	// The Spec templates describe the same launch for a config reader.
	s := Spec()
	if !slices.Equal(s.AutonomyArgs[agentio.AutonomyReadOnly], want[4:8]) || len(s.AutonomyArgs[agentio.AutonomyFull]) != 0 {
		t.Errorf("spec autonomy = %v", s.AutonomyArgs)
	}
	if _, ok := s.AutonomyArgs[agentio.AutonomyAsk]; ok {
		t.Error("spec must not map ask")
	}
}

func TestLaunchOptions(t *testing.T) {
	t.Parallel()
	yes := true
	o := LaunchOptions{Print: true, Provider: "openai", Model: "gpt-x", Thinking: ThinkingHigh, SessionID: "run.1", Name: "n", AppendSystemPrompt: []string{"a", "b"}, Approve: &yes, Offline: true, ExcludeTools: []string{"bash"}, ExtraArgs: []string{"--verbose"}, Prompt: "hi"}
	got, err := o.Args(agentio.MustVersion("1.5.2"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--print", "--provider", "openai", "--model", "gpt-x", "--thinking", "high", "--exclude-tools", "bash", "--session-id", "run.1", "--name", "n", "--append-system-prompt", "a", "--append-system-prompt", "b", "--approve", "--offline", "--verbose", "--", "hi"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %v\nwant   %v", got, want)
	}
	if _, err := (LaunchOptions{NoMCP: true}).Args(agentio.MustVersion("0.99.0")); !errors.Is(err, agentio.ErrFeatureUnavailable) {
		t.Errorf("--no-mcp on 0.99.0 = %v", err)
	}
	if _, err := (LaunchOptions{NoMCP: true}).Args(agentio.Version{}); err != nil {
		t.Errorf("unknown version passes: %v", err)
	}
	for name, bad := range map[string]LaunchOptions{
		"provider alone":   {Provider: "openai"},
		"fork and session": {Fork: "x", Session: "y"},
		"id and continue":  {SessionID: "x", Continue: true},
		"bad mode":         {Mode: "xml"},
		"bad thinking":     {Thinking: "huge"},
	} {
		if _, err := bad.Args(agentio.Version{}); !errors.Is(err, ErrInvalidLaunch) {
			t.Errorf("%s = %v", name, err)
		}
	}
}

// TestTokenLogin: the provider picks the variable, another provider's key
// survives (pi authenticates by these variables), and the token stays out
// of argv and every rendering.
func TestTokenLogin(t *testing.T) {
	t.Parallel()
	const secret = "test-NOT-A-REAL-KEY-0000"
	a, _ := newAgent(t, "OPENAI_API_KEY=user-openai", "ANTHROPIC_API_KEY=inherited")
	n := a.Neutral()
	cred := &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Provider: "anthropic", Token: agentio.NewToken(secret)}
	cmd, err := n.Launch(n.DefaultHome().Target(), agentio.LaunchRequest{Credential: cred})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(envVals(cmd.Env, "ANTHROPIC_API_KEY"), []string{secret}) || !slices.Equal(envVals(cmd.Env, "OPENAI_API_KEY"), []string{"user-openai"}) {
		t.Errorf("env = %v", cmd.Redacted().Env)
	}
	j, _ := json.Marshal(cmd)
	if strings.Contains(strings.Join(cmd.Args, " ")+cmd.String()+string(j), secret) {
		t.Error("token leaked outside Command.Env")
	}
	for _, c := range []agentio.TokenCredential{
		{Kind: agentio.TokenAPIKey, Token: agentio.NewToken(secret)},
		{Kind: agentio.TokenAPIKey, Provider: "amazon-bedrock", Token: agentio.NewToken(secret)},
		{Kind: agentio.TokenOAuthToken, Provider: "anthropic", Token: agentio.NewToken(secret)},
	} {
		_, err := a.TokenEnv(c)
		if !errors.Is(err, agentio.ErrUnsupported) || strings.Contains(err.Error(), secret) {
			t.Errorf("%+v = %v", c.Provider, err)
		}
	}
	// The Spec carries the same map for config readers, with nothing scrubbed.
	for _, e := range Spec().TokenEnv {
		if e.Scrub == nil || len(e.Scrub) != 0 {
			t.Errorf("%s scrubs %v", e.Provider, e.Scrub)
		}
	}
	if len(Spec().CredentialEnv) != 0 {
		t.Error("pi must not scrub provider variables")
	}
}

func TestEncodeSessionDir(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"/home/fixture/proj": "--home-fixture-proj--",
		`C:\work\x`:          "--C--work-x--",
		"/work/a.b c/d":      "--work-a.b c-d--",
		`\\srv\share\p`:      "--" + `-srv-share-p` + "--",
		"/home/user/x:y":     "--home-user-x-y--",
	} {
		if got := EncodeSessionDir(in); got != want {
			t.Errorf("EncodeSessionDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSessionsFixture(t *testing.T) {
	t.Parallel()
	a, _ := newAgent(t)
	h := a.DefaultHome()
	ss, err := a.Sessions(h, agentio.ConversationQuery{})
	if err != nil || len(ss) != 1 {
		t.Fatalf("sessions = %v %v", ss, err)
	}
	s := ss[0]
	if s.ID != fixtureSessionID || s.Cwd != fixtureCwd || s.Version != SessionVersion || s.Name != "my name" || s.FirstPrompt != "hi there" || s.LastPrompt != "via partial id" || s.Model != "localmock/mock-1" || s.Title() != "my name" {
		t.Errorf("session = %+v", s)
	}
	if !s.Timestamp.Equal(time.Date(2026, 11, 14, 11, 41, 31, 118e6, time.UTC)) {
		t.Errorf("created = %v", s.Timestamp)
	}
	c, err := a.Neutral().(agentio.ConversationLister).Conversation(h.Neutral(), fixtureSessionID)
	if err != nil || c.Title != "my name" || c.Dir != fixtureCwd || c.Path != s.Path {
		t.Errorf("conversation = %+v %v", c, err)
	}
	if _, err := a.FindSession(h, "nope"); !errors.Is(err, agentio.ErrNotFound) {
		t.Errorf("missing = %v", err)
	}
	var turns []agentio.Turn
	off, err := a.Turns(h, fixtureSessionID, 0, func(tr agentio.Turn) error { turns = append(turns, tr); return nil })
	if err != nil || len(turns) != 8 {
		t.Fatalf("turns = %d %v", len(turns), err)
	}
	if turns[0].Role != agentio.RoleUser || turns[1].Role != agentio.RoleAssistant || turns[1].Tokens == nil || turns[1].Tokens.Input != 120 || turns[1].Model != "localmock/mock-1" || turns[1].StopReason != "stop" {
		t.Errorf("turn 1 = %+v", turns[1])
	}
	more, err := a.Turns(h, fixtureSessionID, off, func(agentio.Turn) error { t.Error("re-read"); return nil })
	if err != nil || more != off {
		t.Errorf("incremental = %d %v", more, err)
	}
	u, err := a.SessionUsage(h, fixtureSessionID)
	if err != nil || u.Messages != 4 || u.Tokens.Input != 480 || u.Tokens.Output != 16 {
		t.Errorf("usage = %+v %v", u, err)
	}
}

// writeSession writes a session file for cwd under h, cwd encoded with
// json.Marshal so a Windows path stays valid JSON.
func writeSession(t *testing.T, root, cwd, id, prompt string) string {
	t.Helper()
	hdr, err := json.Marshal(SessionHeader{Type: entrySession, Version: SessionVersion, ID: id, Timestamp: time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC), Cwd: cwd})
	if err != nil {
		t.Fatal(err)
	}
	msg := `{"type":"message","id":"aaaaaaaa","parentId":null,"timestamp":"2026-11-14T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":` + string(mustJSON(t, prompt)) + `}],"timestamp":1794650401000}}`
	return agenttest.WriteFile(t, root, filepath.Join(EncodeSessionDir(cwd), "2026-11-14T10-00-00-000Z_"+id+sessionExt), string(hdr)+"\n"+msg+"\n")
}

func mustJSON(t *testing.T, v string) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestSessionsByDir uses host paths (filepath, json.Marshal), so the cwd
// encoding is exercised with the runner's own path form.
func TestSessionsByDir(t *testing.T) {
	t.Parallel()
	a, host := newAgent(t)
	h := a.DefaultHome()
	proj := filepath.Join(host.UserHome, "work", "proj")
	writeSession(t, a.Paths(h).Sessions, proj, "s1", "hello")
	ss, err := a.Sessions(h, agentio.ConversationQuery{Dir: proj})
	if err != nil || len(ss) != 1 || ss[0].ID != "s1" || ss[0].Title() != "hello" {
		t.Fatalf("by dir = %+v %v", ss, err)
	}
	if ok, err := a.HasHistory(h, proj); err != nil || !ok {
		t.Errorf("has history = %v %v", ok, err)
	}
	if ok, _ := a.HasHistory(h, filepath.Join(host.UserHome, "never")); ok {
		t.Error("no history expected")
	}
	if ss, _ := a.Sessions(h, agentio.ConversationQuery{Limit: 1}); len(ss) != 1 {
		t.Errorf("limit = %d", len(ss))
	}
	// A flat custom dir (PI_CODING_AGENT_SESSION_DIR) filters by header cwd.
	flat := filepath.Join(host.UserHome, "flat")
	b, _ := newAgent(t, SessionDirEnv+"="+flat)
	hdr, _ := json.Marshal(SessionHeader{Type: entrySession, Version: SessionVersion, ID: "f1", Cwd: proj})
	agenttest.WriteFile(t, flat, "2026-11-14T10-00-00-000Z_f1.jsonl", string(hdr)+"\n")
	hdr2, _ := json.Marshal(SessionHeader{Type: entrySession, Version: SessionVersion, ID: "f2", Cwd: filepath.Join(host.UserHome, "other")})
	agenttest.WriteFile(t, flat, "2026-11-14T10-00-00-000Z_f2.jsonl", string(hdr2)+"\n")
	st, err := b.SessionStore(b.DefaultHome())
	if err != nil || !st.Flat || st.Root != flat {
		t.Fatalf("store = %+v %v", st, err)
	}
	ss, err = b.Sessions(b.DefaultHome(), agentio.ConversationQuery{Dir: proj})
	if err != nil || len(ss) != 1 || ss[0].ID != "f1" {
		t.Fatalf("flat = %+v %v", ss, err)
	}
	c, _ := newAgent(t, SessionDirEnv+"=relative/dir")
	if _, err := c.SessionStore(c.DefaultHome()); err == nil {
		t.Error("a relative session dir must be refused")
	}
}

// TestSettingsRoundTrip: modelled keys are typed, unknown keys survive a
// round trip, a type change moves one key to Unknown with a warning.
func TestSettingsRoundTrip(t *testing.T) {
	t.Parallel()
	in := `{"$schema":"https://pi.dev/schemas/settings.schema.json","defaultProvider":"anthropic","defaultModel":"claude-sonnet","defaultThinkingLevel":"high","defaultTools":["read","bash"],"compaction":{"enabled":false},"packages":["npm:x",{"source":"git:y"}],"enableInstallTelemetry":"no"}`
	s, warns, err := ParseSettings("settings.json", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if s.DefaultProvider.Or("") != "anthropic" || s.DefaultThinkingLevel.Or("") != ThinkingHigh || !slices.Equal(s.DefaultTools, []string{"read", "bash"}) {
		t.Errorf("typed = %+v", s)
	}
	if !warns.Has(agentio.WarnTypeChanged) || s.EnableInstallTelemetry.Set {
		t.Errorf("type change: %v %+v", warns, s.EnableInstallTelemetry)
	}
	if !slices.Equal(s.Unknown.Keys(), []string{"$schema", "compaction", "enableInstallTelemetry"}) {
		t.Errorf("unknown = %v", s.Unknown.Keys())
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var a, b map[string]json.RawMessage
	if err := json.Unmarshal([]byte(in), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &b); err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("round trip lost keys: %s", out)
	}
	for k, v := range a {
		var x, y any2
		_ = json.Unmarshal(v, &x)
		_ = json.Unmarshal(b[k], &y)
		if string(x) != string(y) {
			t.Errorf("%s: %s != %s", k, v, b[k])
		}
	}
	if !slices.Contains(SettingsKeys(), "defaultModel") || slices.Contains(SettingsKeys(), "Unknown") {
		t.Errorf("keys = %v", SettingsKeys())
	}
}

// any2 compacts a JSON value for comparison (named so it never shadows
// the banned builtin).
type any2 []byte

// UnmarshalJSON keeps the compacted bytes.
func (c *any2) UnmarshalJSON(b []byte) error {
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return err
	}
	*c = buf.Bytes()
	return nil
}

func TestConfigTrustDefaults(t *testing.T) {
	t.Parallel()
	a, host := newAgent(t)
	h := a.DefaultHome()
	proj := filepath.Join(host.UserHome, "proj")
	agenttest.WriteFile(t, h.Dir(), "settings.json", `{"defaultProvider":"openai","defaultModel":"gpt-x"}`)
	agenttest.WriteFile(t, proj, ".pi/settings.json", `{"defaultModel":"gpt-y"}`)
	if tr, err := a.ProjectTrust(h, proj); err != nil || tr != agentio.TrustUnknown {
		t.Fatalf("trust = %v %v", tr, err)
	}
	if d, err := a.Defaults(h, proj); err != nil || d.Model != "openai/gpt-x" {
		t.Errorf("untrusted defaults = %+v %v", d, err)
	}
	real, err := filepath.EvalSymlinks(host.UserHome)
	if err != nil {
		t.Fatal(err)
	}
	td, _ := json.Marshal(map[string]bool{real: true})
	agenttest.WriteFile(t, h.Dir(), "trust.json", string(td))
	if tr, err := a.ProjectTrust(h, proj); err != nil || tr != agentio.TrustTrusted {
		t.Fatalf("trusted parent = %v %v", tr, err)
	}
	if d, err := a.Defaults(h, proj); err != nil || d.Model != "openai/gpt-y" {
		t.Errorf("trusted defaults = %+v %v", d, err)
	}
	layers, err := a.ConfigLayers(h, proj)
	if err != nil || len(layers) != 2 || layers[0].Scope != agentio.ConfigProject || !layers[0].Exists || layers[1].Scope != agentio.ConfigUser {
		t.Errorf("layers = %+v %v", layers, err)
	}
}

func TestResources(t *testing.T) {
	t.Parallel()
	a, host := newAgent(t)
	h := a.DefaultHome()
	proj := filepath.Join(host.UserHome, "repo", "proj")
	agenttest.WriteFile(t, h.Dir(), "AGENTS.md", "global")
	agenttest.WriteFile(t, h.Dir(), "SYSTEM.md", "system")
	agenttest.WriteFile(t, filepath.Join(host.UserHome, "repo"), "CLAUDE.md", "repo")
	agenttest.WriteFile(t, proj, "AGENTS.md", "proj")
	agenttest.WriteFile(t, proj, "CLAUDE.md", "shadowed by AGENTS.md")
	agenttest.WriteFile(t, proj, ".pi/SYSTEM.md", "untrusted project system")
	files, err := a.InstructionFiles(h, proj)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, f := range files {
		kinds = append(kinds, f.Kind+"@"+filepath.Base(filepath.Dir(f.Path)))
	}
	if want := []string{"AGENTS.md@agent", "CLAUDE.md@repo", "AGENTS.md@proj", "SYSTEM.md@agent"}; !slices.Equal(kinds, want) {
		t.Errorf("instructions = %v, want %v", kinds, want)
	}
	agenttest.WriteFile(t, h.Dir(), "mcp.json", `{"mcpServers":{"fs":{"command":"npx","args":["-y","srv"],"env":{"TOKEN":"fixture-secret"}},"docs":{"url":"https://example.com/mcp","enabled":false}}}`)
	ms, err := a.MCPServers(h, "")
	if err != nil || len(ms) != 2 || ms[0].Name != "docs" || ms[0].Transport != agentio.MCPHTTP || ms[1].Transport != agentio.MCPStdio || !slices.Equal(ms[1].EnvNames, []string{"TOKEN"}) {
		t.Fatalf("mcp = %+v %v", ms, err)
	}
	if strings.Contains(strings.Join(ms[1].EnvNames, ""), "fixture-secret") {
		t.Error("env value leaked")
	}
	agenttest.WriteFile(t, h.Dir(), "skills/review/SKILL.md", "---\nname: code-review\ndescription: Reviews code\n---\nBody\n")
	agenttest.WriteFile(t, h.Dir(), "skills/nested/deep/SKILL.md", "no frontmatter\n")
	sk, err := a.Skills(h, proj)
	if err != nil || len(sk) != 2 {
		t.Fatalf("skills = %+v %v", sk, err)
	}
	names := []string{sk[0].Name, sk[1].Name}
	slices.Sort(names)
	if !slices.Equal(names, []string{"code-review", "deep"}) {
		t.Errorf("skill names = %v", names)
	}
	agenttest.WriteFile(t, h.Dir(), "prompts/fix.md", "---\ndescription: Fix it\n---\nFix $1\n")
	ps, err := a.PromptTemplates(h, "")
	if err != nil || len(ps) != 1 || ps[0].Name != "fix" || ps[0].Description != "Fix it" {
		t.Errorf("prompts = %+v %v", ps, err)
	}
}

func TestLoginsAndDoctor(t *testing.T) {
	t.Parallel()
	a, _ := newAgent(t)
	h := a.DefaultHome()
	if id, err := a.Identity(h); err != nil || id.LoggedIn {
		t.Fatalf("fresh identity = %+v %v", id, err)
	}
	// auth.json shape is source-derived (no logged-in recording yet).
	agenttest.WriteFile(t, h.Dir(), "auth.json", `{"anthropic":{"type":"oauth","access":"fixture-access","refresh":"fixture-refresh","expires":1794650400000},"deepseek":{"type":"api_key","key":"!print-key"},"cloudflare-workers-ai":{"type":"api_key","key":"fixture-key","env":{"CLOUDFLARE_ACCOUNT_ID":"x"}}}`)
	ls, err := a.Logins(h)
	if err != nil || len(ls) != 3 {
		t.Fatalf("logins = %+v %v", ls, err)
	}
	if ls[0].Provider != "anthropic" || ls[0].Type != LoginOAuth || ls[0].ExpiresAt == nil || !ls[2].FromCommand || !slices.Equal(ls[1].EnvNames, []string{"CLOUDFLARE_ACCOUNT_ID"}) {
		t.Errorf("logins = %+v", ls)
	}
	j, _ := json.Marshal(ls)
	if strings.Contains(string(j), "fixture-") {
		t.Errorf("secret leaked: %s", j)
	}
	if id, _ := a.Identity(h); !id.LoggedIn || id.Credential != agentio.CredentialFile {
		t.Errorf("identity = %+v", id)
	}
	checks := a.Checks(context.Background(), h)
	ids := map[agentio.CheckID]agentio.CheckStatus{}
	for _, c := range checks {
		ids[c.ID] = c.Status
	}
	if ids[agentio.CheckLogin] != agentio.CheckOK || ids[CheckSettingsParse] != agentio.CheckOK || ids[agentio.CheckVersionTested] != agentio.CheckWarn || ids[CheckCacheWarming] != agentio.CheckInfo {
		t.Errorf("checks = %+v", checks)
	}
	b, _ := newAgent(t, "GEMINI_API_KEY=x")
	if id, _ := b.Identity(b.DefaultHome()); !id.LoggedIn || id.Credential != agentio.CredentialEnv {
		t.Errorf("env identity = %+v", id)
	}
}

func TestInstallAndProcess(t *testing.T) {
	t.Parallel()
	host := agenttest.NewHost(t)
	pkg := filepath.Join(host.UserHome, "npm", "lib", "node_modules", "@earendil-works", "pi-coding-agent")
	agenttest.WriteFile(t, pkg, "package.json", `{"name":"@earendil-works/pi-coding-agent","version":"1.5.2","piConfig":{"configDir":".pi"}}`)
	cli := agenttest.WriteFile(t, pkg, "dist/bundle/cli.js", "#!/usr/bin/env node\n")
	bin := filepath.Join(host.UserHome, "npm", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	agenttest.SymlinkOrSkip(t, cli, filepath.Join(bin, agenttest.ExeName(Binary)))
	if err := os.Chmod(cli, 0o755); err != nil {
		t.Fatal(err)
	}
	host.Env = []string{"HOME=" + host.UserHome, "PATH=" + bin}
	in, err := New(host).Install()
	if err != nil || in.Version.String() != "1.5.2" || in.Method != agentio.InstallNPM {
		t.Fatalf("install = %+v %v", in, err)
	}
	n := New(host).Neutral().(agentio.ProcessClassifier)
	// pi sets process.title = "pi": argv0 is "pi", exe is node.
	if info, ok := n.ClassifyProcess(agentio.Process{PID: 1, Exe: "/usr/bin/node", Argv: []string{"pi"}}); !ok || info.Agent != ID {
		t.Errorf("pi row = %v %v", info, ok)
	}
	if _, ok := n.ClassifyProcess(agentio.Process{PID: 2, Exe: "/usr/bin/node", Argv: []string{"node", "server.js"}}); ok {
		t.Error("plain node matched")
	}
}

// TestSharedConformance runs the shared agenttest checks, token login
// included: TokenProviders names the provider the credential needs.
func TestSharedConformance(t *testing.T) {
	a, _ := newAgent(t)
	agenttest.Conformance(t, a.Neutral(), a.DefaultHome().Neutral())
}
