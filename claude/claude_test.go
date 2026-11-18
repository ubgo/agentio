package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// fixtureVersions lists every recorded fixture home; each reader test runs
// over all of them (SPEC §10).
func fixtureVersions(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("testdata", "homes"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		t.Fatal("no fixture homes")
	}
	return out
}

// forEachFixture runs fn once per fixture version as a subtest, handing it
// the version's user-home tree, so an older fixture keeps guarding renamed
// and removed keys (SPEC §10).
func forEachFixture(t *testing.T, fn func(t *testing.T, fixture string)) {
	t.Helper()
	for _, v := range fixtureVersions(t) {
		t.Run(v, func(t *testing.T) { fn(t, filepath.Join("testdata", "homes", v, "userhome")) })
	}
}

// fixtureAgent copies a fixture user home to a temp dir and builds an agent
// on it with fakes for every host input.
func fixtureAgent(t *testing.T, version string, opts ...agenttest.HostOption) (*Agent, string) {
	t.Helper()
	dir := t.TempDir()
	agenttest.CopyFixture(t, filepath.Join("testdata", "homes", version, "userhome"), dir)
	base := []agenttest.HostOption{agenttest.WithUserHome(dir), agenttest.WithEnv("PATH=" + filepath.Join(dir, "bin"))}
	h := agenttest.NewHost(t, append(base, opts...)...)
	h.User = "user"
	return New(h), dir
}

func TestSpecValidates(t *testing.T) {
	if err := Spec().Validate(); err != nil {
		t.Fatal(err)
	}
	if New(agentio.Host{}).core == nil {
		t.Fatal("core not built")
	}
}

func TestCompatParses(t *testing.T) {
	c := Compat()
	if c.Min.IsZero() || c.Max.IsZero() || c.Min.Compare(c.Max) > 0 {
		t.Fatalf("bad compat %+v", c)
	}
	for _, v := range fixtureVersions(t) {
		if c.Check(agentio.MustVersion(v)) == agentio.CompatUnknown {
			t.Fatalf("fixture %s outside compat", v)
		}
	}
}

// TestFeaturesTableMatchesGenerated proves features_gen.go is the
// generation of features.tsv and every bound names a source (invariant 15).
func TestFeaturesTableMatchesGenerated(t *testing.T) {
	data, err := os.ReadFile("features.tsv")
	if err != nil {
		t.Fatal(err)
	}
	var rows [][]string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		l := sc.Text()
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Split(l, "\t")
		if len(f) != 5 {
			t.Fatalf("bad row %q", l)
		}
		rows = append(rows, f)
	}
	feats := Features()
	if len(feats) != len(rows) {
		t.Fatalf("generated %d features, tsv has %d: run go generate", len(feats), len(rows))
	}
	for i, r := range rows {
		f := feats[i]
		if f.String() != r[1] {
			t.Errorf("feature %d spelling %q, tsv %q", i, f.String(), r[1])
		}
		span := FeatureInfo(f)
		if span.Since.String() != agentio.MustVersion(r[2]).String() || span.Until.String() != agentio.MustVersion(r[3]).String() {
			t.Errorf("%s bounds %v, tsv %q %q", r[0], span, r[2], r[3])
		}
		if span.Known() && FeatureSource(f) == "" {
			t.Errorf("%s has a bound but no source", r[0])
		}
		if FeatureSource(f) != r[4] {
			t.Errorf("%s source mismatch", r[0])
		}
	}
}

func TestEncodePath(t *testing.T) {
	got := EncodePath("/space/projects/a_b.c d")
	if got != "-space-projects-a-b-c-d" {
		t.Fatal(got)
	}
}

func TestHomes(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		a, dir := fixtureAgent(t, v)
		if err := os.MkdirAll(filepath.Join(dir, ".claude-worktrees"), 0o700); err != nil {
			t.Fatal(err)
		}
		hs, err := a.Homes()
		if err != nil {
			t.Fatal(err)
		}
		if len(hs) != 2 || !hs[0].IsDefault() || hs[1].Name() != "work" {
			t.Fatalf("homes %v", hs)
		}
		if _, err := a.HomeOf(agentio.NewHome("codex", dir, true, "x")); !errors.Is(err, agentio.ErrForeignHome) {
			t.Fatal("foreign home accepted")
		}
	}
}

func TestGlobalConfigAndIdentity(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		secrets := agenttest.NewFakeSecrets()
		a, _ := fixtureAgent(t, v, agenttest.WithSecrets(secrets), agenttest.WithNow(agenttest.FixedNow))
		def := a.DefaultHome()
		g, err := a.GlobalConfig(def)
		if err != nil {
			t.Fatal(err)
		}
		if g.OAuthAccount == nil || g.OAuthAccount.EmailAddress.Reveal() != "user@example.com" {
			t.Fatal("oauthAccount not decoded")
		}
		if len(g.Unknown) == 0 || len(g.Projects) == 0 || g.CachedUsage == nil {
			t.Fatalf("unknown=%d projects=%d cached=%v", len(g.Unknown), len(g.Projects), g.CachedUsage != nil)
		}
		if g.MCPServers["fixture-stdio"].Env["FIXTURE_TOKEN"].String() == "fixture-secret" {
			t.Fatal("env value printed")
		}
		id, err := a.Identity(def)
		if err != nil {
			t.Fatal(err)
		}
		if !id.LoggedIn || id.PlanLabel != "Max (20x)" || id.Identity.Credential != agentio.CredentialFile {
			t.Fatalf("identity %+v", id.Identity)
		}
		if strings.Contains(id.Email.String(), "@") {
			t.Fatal("email not redacted")
		}
		work, _ := a.Homes()
		wid, err := a.Identity(work[1])
		if err != nil || wid.LoggedIn {
			t.Fatalf("work home should be logged out: %+v %v", wid, err)
		}
	}
}

// TestKeychainPrefersAccount guards invariant 3: the named-account entry
// wins over a stale same-service entry.
func TestKeychainPrefersAccount(t *testing.T) {
	secrets := agenttest.NewFakeSecrets()
	a, dir := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithSecrets(secrets), agenttest.WithNow(agenttest.FixedNow))
	if err := os.Remove(filepath.Join(dir, ".claude", credentialsFile)); err != nil {
		t.Fatal(err)
	}
	h := a.DefaultHome()
	svc := KeychainServices(h.Dir(), true)[0]
	secrets.Put(svc, "", `{"claudeAiOauth":{"accessToken":"stale","expiresAt":0}}`)
	secrets.Put(svc, "user", `{"claudeAiOauth":{"accessToken":"good","expiresAt":1893456000000,"subscriptionType":"pro"}}`)
	c, err := a.Credential(h)
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken.Reveal() != "good" || c.Source != SourceKeychain {
		t.Fatalf("got %s from %s", c.AccessToken, c.Source)
	}
}

func TestCachedUsage(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		a, _ := fixtureAgent(t, v)
		u, found, err := a.CachedUsage(a.DefaultHome())
		if err != nil || !found {
			t.Fatal(err, found)
		}
		if u.Source != agentio.UsageCache || u.ObservedAt.IsZero() || len(u.Windows) < 2 {
			t.Fatalf("usage %+v", u.Usage)
		}
		if u.Windows[0].Length != 5*time.Hour || u.Windows[0].Class() != agentio.WindowShort {
			t.Fatalf("window %+v", u.Windows[0])
		}
		if _, err := ExtraUsageOf(u.Neutral()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUsageAndProfileHTTP(t *testing.T) {
	usageBody := `{"limits":[{"kind":"session","group":"session","percent":42,"severity":"normal","resets_at":"2026-11-13T15:00:00Z","scope":null,"is_active":true}],"extra_usage":{"is_enabled":true,"utilization":12.5,"currency":"USD"}}`
	profileBody := `{"organization":{"organization_type":"claude_max","rate_limit_tier":"default_claude_max_20x","billing_type":"stripe_subscription","subscription_status":"active"}}`
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case usagePath:
			_, _ = w.Write([]byte(usageBody))
		case profilePath:
			_, _ = w.Write([]byte(profileBody))
		default:
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer srv.Close()
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithHTTP(srv.Client()), agenttest.WithNow(agenttest.FixedNow))
	a = a.WithBaseURL(srv.URL)
	u, err := a.Usage(context.Background(), a.DefaultHome())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(gotAuth, "Bearer sk-ant-oat01.FIXTURE") || len(u.Windows) != 2 || !u.Extra.IsEnabled || u.Source != agentio.UsageServer {
		t.Fatalf("usage %+v", u)
	}
	s, err := a.Subscription(context.Background(), a.DefaultHome())
	if err != nil || s.Plan != "Max (20x)" || s.Status != "active" {
		t.Fatalf("sub %+v %v", s, err)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{
		{http.StatusTooManyRequests, `{}`, agentio.ErrRateLimited},
		{http.StatusUnauthorized, `{"error":{"message":"bad sk-ant-oat01.FIXTURE-NOT-A-TOKEN"}}`, agentio.ErrUnauthorized},
		{http.StatusForbidden, `{"error":{"details":{"error_code":"oauth_scope_insufficient"}}}`, agentio.ErrScopeLimited},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithHTTP(srv.Client()), agenttest.WithNow(agenttest.FixedNow))
		_, err := a.WithBaseURL(srv.URL).Usage(context.Background(), a.DefaultHome())
		srv.Close()
		if !errors.Is(err, tc.want) {
			t.Fatalf("status %d: %v", tc.status, err)
		}
		if strings.Contains(err.Error(), "FIXTURE-NOT-A-TOKEN") {
			t.Fatal("token in error")
		}
	}
}

func TestProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		w.Header().Set("anthropic-ratelimit-unified-7d-utilization", "1.0")
		w.Header().Set("anthropic-ratelimit-unified-7d-reset", "1794700000")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithHTTP(srv.Client()))
	u, err := a.WithBaseURL(srv.URL).Prober().TokenUsage(context.Background(), agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("sk-ant-oat01-x")})
	if err != nil || len(u.Windows) != 1 || u.Windows[0].Percent != 100 {
		t.Fatalf("%+v %v", u, err)
	}
}

func TestSettingsMerge(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		a, dir := fixtureAgent(t, v)
		p := Project{Dir: filepath.Join(dir, "project")}
		eff, err := a.Settings(a.DefaultHome(), &p, nil)
		if err != nil {
			t.Fatal(err)
		}
		if eff.Merged.Model != "sonnet" || eff.Origin["model"] != ScopeLocal {
			t.Fatalf("model %q from %s", eff.Merged.Model, eff.Origin["model"])
		}
		if eff.Merged.Permissions == nil || eff.Merged.Permissions.DefaultMode != ModeBypassPermissions || len(eff.Merged.Permissions.Allow) != 1 {
			t.Fatalf("permissions %+v", eff.Merged.Permissions)
		}
		if len(eff.Merged.Hooks[HookStop]) != 2 {
			t.Fatalf("stop hooks %d", len(eff.Merged.Hooks[HookStop]))
		}
		if _, ok := eff.Merged.Unknown["futureKey"]; !ok {
			t.Fatal("unknown key dropped")
		}
		if eff.Merged.Hooks[HookPreToolUse][0].Hooks[0].Timeout <= 0 {
			t.Fatal("timeout not decoded")
		}
	}
}

func TestSettingsTypeChangeDegrades(t *testing.T) {
	s, err := ParseSettings("x", []byte(`{"model": 7, "theme": "dark"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Theme != "dark" || !s.Warnings.Has(agentio.WarnTypeChanged) || s.Unknown["model"] == nil {
		t.Fatalf("%+v", s)
	}
}

func TestMCPServers(t *testing.T) {
	a, dir := fixtureAgent(t, fixtureVersions(t)[0])
	p := Project{Dir: filepath.Join(dir, "project")}
	ss, err := a.MCPServers(a.DefaultHome(), &p)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]MCPServer{}
	for _, s := range ss {
		byName[s.Name] = s
	}
	if byName["proj-db"].Approval != ApprovalApproved || byName["proj-db"].Transport != TransportStdio {
		t.Fatalf("proj-db %+v", byName["proj-db"])
	}
	if byName["fixture-http"].Transport != TransportHTTP || byName["fixture-http"].Scope != MCPScopeUser {
		t.Fatalf("http %+v", byName["fixture-http"])
	}
	out, _ := json.Marshal(byName["proj-db"].Env)
	if strings.Contains(string(out), "fixture-secret") {
		t.Fatal("secret in JSON")
	}
}

func TestConversations(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		a, _ := fixtureAgent(t, v)
		cs, err := a.Conversations(context.Background(), a.DefaultHome(), agentio.ConversationQuery{Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		if len(cs) == 0 {
			t.Fatal("no conversations")
		}
		for _, c := range cs {
			if c.Dir == "" || c.ID == "" || c.Title == "" {
				t.Fatalf("conversation %+v", c)
			}
		}
		one, err := a.Conversation(a.DefaultHome(), cs[0].ID)
		if err != nil || one.ID != cs[0].ID {
			t.Fatal(err)
		}
		has, err := a.HasHistory(a.DefaultHome(), cs[0].Dir)
		if err != nil || !has {
			t.Fatal("history not found", err)
		}
		hs, _ := a.Homes()
		wc, err := a.Conversations(context.Background(), hs[1], agentio.ConversationQuery{Dir: "/work/proj99"})
		if err != nil || len(wc) != 1 || wc[0].Title != "Work home fixture" || wc[0].TitleSource != TitleAI {
			t.Fatalf("work conversations %+v %v", wc, err)
		}
	}
}

func TestTurnsOffsetResume(t *testing.T) {
	a, dir := fixtureAgent(t, fixtureVersions(t)[0])
	hs, _ := a.Homes()
	id := "0b5e3a8e-0000-4000-8000-000000000099"
	var turns []agentio.Turn
	off, err := a.Turns(hs[1], id, 0, func(tn agentio.Turn) error { turns = append(turns, tn); return nil })
	if err != nil || len(turns) != 2 || turns[1].Tokens == nil || turns[1].Tokens.CachedInput != 300 {
		t.Fatalf("turns %+v %v", turns, err)
	}
	// Append a partial line: the offset must not move past it.
	p := filepath.Join(dir, ".claude-work", "projects", "-work-proj99", id+".jsonl")
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"type":"user","message":{"content":"partial`)
	_ = f.Close()
	off2, err := a.Turns(hs[1], id, off, func(agentio.Turn) error { t.Fatal("partial line read"); return nil })
	if err != nil || off2 != off {
		t.Fatalf("offset moved %d -> %d (%v)", off, off2, err)
	}
	last, ok, err := a.LastAssistant(hs[1], id)
	if err != nil || !ok || last.Content.Text() != "fixture text 2" || last.Usage.CacheRead != 300 {
		t.Fatalf("last assistant %+v %v %v", last, ok, err)
	}
}

func TestPromptHistory(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		a, _ := fixtureAgent(t, v)
		es, err := a.PromptHistory(a.DefaultHome(), HistoryQuery{Limit: 3})
		if err != nil || len(es) != 3 {
			t.Fatalf("%d %v", len(es), err)
		}
		if es[0].Time.Before(es[1].Time) {
			t.Fatal("not newest first")
		}
	}
	if p := parsePasted(json.RawMessage(`{"1":{"id":1,"type":"text","content":"x"}}`)); len(p) != 1 || p[0].Content != "x" {
		t.Fatal("pasted object form")
	}
	if p := parsePasted(json.RawMessage(`[]`)); p != nil {
		t.Fatal("pasted empty form")
	}
}

func TestLaunchHomeRuleAndFeatures(t *testing.T) {
	a, dir := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithEnv("PATH=/bin", ConfigDirEnv+"=/somewhere"))
	def := a.DefaultHome()
	cmd, err := a.Launch(def.Target(), LaunchOptions{Model: "opus", PermissionMode: ModePlan, Prompt: "-hi"})
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, ConfigDirEnv+"=") {
			t.Fatal("default home must remove CLAUDE_CONFIG_DIR")
		}
	}
	if got := strings.Join(cmd.Args, " "); got != "--model opus --permission-mode plan -- -hi" {
		t.Fatal(got)
	}
	if !cmd.Warnings.Has(agentio.WarnVersionUnknown) {
		t.Fatal("unknown version not warned")
	}
	work := a.HomeAt(filepath.Join(dir, ".claude-work"))
	wt := work.Target()
	wt.Version = agentio.MustVersion("2.1.40")
	name := ""
	_, err = a.Launch(wt, LaunchOptions{Worktree: &name})
	var fe *FeatureUnavailableError
	if !errors.As(err, &fe) || !errors.Is(err, agentio.ErrFeatureUnavailable) || fe.Feature != FeatureFlagWorktree {
		t.Fatalf("worktree on 2.1.40: %v", err)
	}
	wt.Version = agentio.MustVersion("2.1.400")
	cmd, err = a.Launch(wt, LaunchOptions{Resume: "abc"})
	if err != nil || !cmd.Warnings.Has(agentio.WarnUntestedVersion) {
		t.Fatalf("newer version: %v %v", err, cmd.Warnings)
	}
	if v, _ := agentio.EnvLookup(cmd.Env, ConfigDirEnv); v != work.Dir() {
		t.Fatalf("non-default home env %q", v)
	}
	if _, err := (LaunchOptions{OutputFormat: OutputJSON}).Args(); !errors.Is(err, ErrInvalidLaunch) {
		t.Fatal("output-format without -p accepted")
	}
}

func TestNeutralLaunchToken(t *testing.T) {
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithEnv("PATH=/bin", APIKeyEnv+"=old"))
	tok := agentio.NewToken("sk-ant-oat01-secret")
	cmd, err := a.launchRequest(a.DefaultHome().Target(), agentio.LaunchRequest{Headless: true, Prompt: "x", Autonomy: agentio.AutonomyAcceptEdits, Credential: &agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: tok}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(cmd.Args, " "), "secret") || strings.Contains(cmd.Redacted().String(), "secret") {
		t.Fatal("token leaked")
	}
	if v, ok := agentio.EnvLookup(cmd.Env, APIKeyEnv); ok {
		t.Fatalf("api key not scrubbed: %q", v)
	}
	if _, err := a.launchRequest(a.DefaultHome().Target(), agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}); !errors.Is(err, agentio.ErrDangerousNotAllowed) {
		t.Fatal("full autonomy without AllowDangerous")
	}
}

func TestWriterHooks(t *testing.T) {
	a, dir := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithNow(agenttest.FixedNow))
	h := a.DefaultHome()
	w := a.Writer()
	b, err := w.AddHook(h, nil, ScopeUser, HookStop, HookMatcher{Hooks: []HookHandler{{Type: HookTypeCommand, Command: "ailog hook", Timeout: 5 * time.Second}}})
	if err != nil || b.IsEmpty() {
		t.Fatalf("add: %v %+v", err, b)
	}
	l, _ := a.UserSettings(h)
	if len(l.Settings.Hooks[HookStop]) != 2 || l.Settings.Theme != "dark" {
		t.Fatalf("after add %+v", l.Settings.Hooks[HookStop])
	}
	n, b2, err := w.RemoveHooks(h, nil, ScopeUser, func(_ HookEvent, hh HookHandler) bool { return hh.Command != "ailog hook" })
	if err != nil || n != 1 || b2.IsEmpty() || b2.Entries[0].Copy == b.Entries[0].Copy {
		t.Fatalf("remove: %d %v %+v", n, err, b2)
	}
	if _, err := os.Stat(b.Entries[0].Copy); err != nil {
		t.Fatal("backup missing")
	}
	_ = dir
}

func TestHookInputDecode(t *testing.T) {
	in, err := DecodeHookInput(strings.NewReader(`{"session_id":"s","transcript_path":"/t","cwd":"/c","hook_event_name":"Stop","permission_mode":"default","last_assistant_message":"done","stop_hook_active":false,"effort":{"level":"high"},"new_field":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if in.PermissionMode != ModeManual || in.LastAssistantMessage != "done" || in.Effort.Level != EffortHigh || in.Unknown["new_field"] == nil {
		t.Fatalf("%+v", in)
	}
}

func TestInstallNative(t *testing.T) {
	a, dir := fixtureAgent(t, fixtureVersions(t)[0])
	ver := filepath.Join(dir, ".local", "share", "claude", "versions", "2.1.323")
	bin := filepath.Join(dir, "bin")
	for _, d := range []string{filepath.Dir(ver), bin} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(ver, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	agenttest.SymlinkOrSkip(t, ver, filepath.Join(bin, agenttest.ExeName(Binary)))
	inst, err := a.Install()
	if err != nil {
		t.Fatal(err)
	}
	if inst.Method != agentio.InstallNative || inst.Version.String() != "2.1.323" || inst.RawMethod != "native" || inst.LastUpdate == nil {
		t.Fatalf("install %+v", inst)
	}
}

func TestConformance(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		a, _ := fixtureAgent(t, v)
		hs, err := a.Homes()
		if err != nil || len(hs) == 0 {
			t.Fatal(err)
		}
		agenttest.Conformance(t, a.Neutral(), hs[0].Neutral())
		if r, ok := agentio.Unsupported(a.Neutral(), agentio.CapModels); !ok || r == "" {
			t.Fatal("ModelLister reason not declared")
		}
	}
}

func TestDoctor(t *testing.T) {
	a, dir := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithNow(agenttest.FixedNow))
	p := Project{Dir: filepath.Join(dir, "project")}
	checks := a.Doctor(context.Background(), a.DefaultHome(), &p, DoctorOptions{})
	ids := map[agentio.CheckID]agentio.CheckStatus{}
	for _, c := range checks {
		ids[c.ID] = c.Status
		if strings.Contains(c.Message, "FIXTURE-NOT-A-TOKEN") || strings.Contains(c.Message, "user@example.com") {
			t.Fatalf("value in check %s", c.ID)
		}
	}
	for _, want := range []agentio.CheckID{agentio.CheckVersionTested, agentio.CheckLogin, CheckRetention, CheckHistorySize} {
		if _, ok := ids[want]; !ok {
			t.Errorf("missing check %s in %v", want, ids)
		}
	}
}

// TestRunnerBuiltNotRun drives the Runner door through a fake exec: the
// commands are built with the home env rule, and the login files of the
// fixture home are unchanged afterwards (bytes and mtime, SPEC §10).
func TestRunnerBuiltNotRun(t *testing.T) {
	fx := &agenttest.FakeExec{Results: map[string]agentio.ProcessResult{
		"claude --version":           {Stdout: []byte("2.1.323 (Claude Code)\n")},
		"claude auth status --json":  {Stdout: []byte(`{"loggedIn":true,"authMethod":"claude.ai","email":"user@example.com","planTier":"max_20x","newKey":1}`)},
		"claude plugin list --json":  {Stdout: []byte(`[{"id":"p@m","version":"1","scope":"user","enabled":true,"installPath":"/x"}]`)},
		"claude agents --json --all": {Stdout: []byte(`[{"id":"ab","pid":1,"sessionId":"s","kind":"bg","state":"running","startedAt":1794000000000,"newAgentKey":"x"}]`)},
		"claude auto-mode config":    {Stdout: []byte(`{}`)},
	}}
	a, dir := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithExec(fx))
	cred := filepath.Join(dir, ".claude", credentialsFile)
	before, _ := os.ReadFile(cred)
	st0, _ := os.Stat(cred)
	r := a.Runner()
	v, err := r.BinaryVersion(context.Background())
	if err != nil || v.String() != "2.1.323" {
		t.Fatal(v, err)
	}
	s, err := r.AuthStatus(context.Background(), a.DefaultHome())
	if err != nil || !s.LoggedIn || s.Unknown["newKey"] == nil || strings.Contains(s.Email.String(), "@") {
		t.Fatalf("%+v %v", s, err)
	}
	if ps, err := r.Plugins(context.Background(), a.DefaultHome(), false); err != nil || len(ps) != 1 {
		t.Fatal(ps, err)
	}
	if bg, err := r.BackgroundAgents(context.Background(), a.DefaultHome(), true); err != nil || len(bg) != 1 || bg[0].Unknown["newAgentKey"] == nil {
		t.Fatal(bg, err)
	}
	if _, err := r.AutoModeConfig(context.Background(), a.DefaultHome()); err != nil {
		t.Fatal(err)
	}
	for _, c := range fx.Calls {
		if _, ok := agentio.EnvLookup(c.Env, ConfigDirEnv); ok {
			t.Fatalf("default home run carries %s", ConfigDirEnv)
		}
	}
	after, _ := os.ReadFile(cred)
	st1, _ := os.Stat(cred)
	if !bytes.Equal(before, after) || !st0.ModTime().Equal(st1.ModTime()) {
		t.Fatal("login file changed")
	}
}

func TestStricterLowerValueWins(t *testing.T) {
	got := mergeKey("disableClaudeAiConnectors", json.RawMessage(`true`), json.RawMessage(`false`))
	if string(got) != "true" {
		t.Fatal(string(got))
	}
	got = mergeKey("maxEffortLevel", json.RawMessage(`"medium"`), json.RawMessage(`"max"`))
	if string(got) != `"medium"` {
		t.Fatal(string(got))
	}
	got = mergeKey(keyPermissions, json.RawMessage(`{"denyInsteadOfPrompting":true,"allow":["A"]}`), json.RawMessage(`{"denyInsteadOfPrompting":false,"allow":["B"]}`))
	var p Permissions
	if err := json.Unmarshal(got, &p); err != nil || p.DenyInsteadOfPrompting == nil || !*p.DenyInsteadOfPrompting || len(p.Allow) != 2 {
		t.Fatalf("%s", got)
	}
	got = mergeKey("model", json.RawMessage(`"a"`), json.RawMessage(`"b"`))
	if string(got) != `"b"` {
		t.Fatal(string(got))
	}
}

// TestMCPAddJSONStdinRow pins the 2.1.327 drift row: `claude mcp add-json
// <name> -` reads the server JSON from stdin since 2.1.327 (CHANGELOG
// "Added"), the vendor's way to keep env values and headers off argv.
func TestMCPAddJSONStdinRow(t *testing.T) {
	span := FeatureInfo(FeatureMCPAddJSONStdin)
	if span.Since.String() != "2.1.327" || !span.Until.IsZero() || FeatureSource(FeatureMCPAddJSONStdin) == "" {
		t.Fatalf("span %v source %q", span, FeatureSource(FeatureMCPAddJSONStdin))
	}
	if span.In(agentio.MustVersion("2.1.326")) || !span.In(agentio.MustVersion("2.1.327")) {
		t.Fatalf("span %v must start at 2.1.327", span)
	}
}
