package gemini

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

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/gemini/internal/featuregen"
)

// fixtureRoot is the 0.68.0 fixture (userhome/.gemini and project/).
const fixtureRoot = "testdata/homes/0.68.0"

// Placeholders the fixture carries instead of machine paths.
const (
	phProject = "@PROJECT@"
	phHash    = "@PROJECT_HASH@"
)

// Session ids in the fixture chats.
const (
	sessFlaky     = "11111111-1111-4111-8111-111111111111"
	sessChangelog = "22222222-2222-4222-8222-222222222222"
	sessEmpty     = "33333333-3333-4333-8333-333333333333"
	sessLegacy    = "44444444-4444-4444-8444-444444444444"
	sessSubagent  = "55555555-5555-4555-8555-555555555555"
)

// fixture is a copied fixture: the agent, its default home and the
// project dir the placeholders now name.
type fixture struct {
	a       *Agent
	home    Home
	project string
	host    agentio.Host
}

// setup copies the fixture into temp dirs and replaces the placeholders:
// raw in .project_root, JSON-encoded inside JSON files (a Windows path
// pasted raw would be an invalid escape).
func setup(t *testing.T, opts ...agenttest.HostOption) fixture {
	t.Helper()
	host := agenttest.NewHost(t, opts...)
	agenttest.CopyFixture(t, filepath.Join(fixtureRoot, "userhome"), host.UserHome)
	project := filepath.Join(t.TempDir(), "demo")
	agenttest.CopyFixture(t, filepath.Join(fixtureRoot, "project"), project)
	if err := os.MkdirAll(filepath.Join(project, gitDir), 0o700); err != nil {
		t.Fatal(err)
	}
	quoted, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	gdir := filepath.Join(host.UserHome, defaultDirName)
	err = filepath.WalkDir(gdir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out := bytes.ReplaceAll(b, []byte(`"`+phProject+`"`), quoted)
		out = bytes.ReplaceAll(out, []byte(phProject), []byte(project))
		out = bytes.ReplaceAll(out, []byte(phHash), []byte(ProjectHash(project)))
		return os.WriteFile(p, out, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	a := New(host)
	return fixture{a: a, home: a.DefaultHome(), project: project, host: host}
}

func TestSpecAndDrift(t *testing.T) {
	if err := Spec().Validate(); err != nil {
		t.Fatalf("spec: %v", err)
	}
	if c := Compat(); c.Min.String() != TestedMin || c.Max.String() != TestedMax {
		t.Fatalf("compat %v", c)
	}
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
	if !bytes.Equal(want, got) {
		t.Fatal("features_gen.go is stale: run go generate ./gemini")
	}
	if FeatureInfo(FeatureFlagApprovalMode).Known() || FeatureSource(FeatureFlagApprovalMode) != "" {
		t.Fatal("no release note names a bound: every bound stays unknown")
	}
	if checkFeature(agentio.MustVersion("0.68.0"), FeatureFlagResumeUUID) != nil {
		t.Fatal("an unknown bound must pass")
	}
	err = (&FeatureUnavailableError{Feature: FeatureFlagResume, Installed: agentio.MustVersion("0.1.0"), Since: agentio.MustVersion("0.2.0")})
	if !errors.Is(err, agentio.ErrFeatureUnavailable) || !strings.Contains(err.Error(), "needs gemini >= 0.2.0") {
		t.Fatalf("feature error %v", err)
	}
}

func TestHomes(t *testing.T) {
	f := setup(t)
	if f.home.Dir() != filepath.Join(f.host.UserHome, defaultDirName) || f.home.Name() != agentio.DefaultHomeName {
		t.Fatalf("default home %v %s", f.home, f.home.Name())
	}
	parent := filepath.Join(f.host.UserHome, "gemini-work")
	agenttest.WriteFile(t, parent, ".gemini/projects.json", `{"projects":{}}`)
	f.host.Env = append(f.host.Env, HomeEnv+"="+parent)
	a := New(f.host)
	h, ok := a.HomeFromEnv()
	if !ok || h.Dir() != filepath.Join(parent, defaultDirName) || h.Name() != "gemini-work" {
		t.Fatalf("home from env %v %v", h, ok)
	}
	hs, err := a.Homes()
	if err != nil || len(hs) != 2 || !hs[0].IsDefault() {
		t.Fatalf("homes %v %v", hs, err)
	}
	if ok, err := a.IsHome(filepath.Join(f.host.UserHome, "nothing")); ok || err != nil {
		t.Fatalf("not a home %v %v", ok, err)
	}
	if _, err := a.HomeOf(agentio.NewHome("claude", "/x", false, "x")); !errors.Is(err, agentio.ErrForeignHome) {
		t.Fatalf("foreign %v", err)
	}
}

func TestInstallAndVersion(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "lib", "node_modules", "@google", "gemini-cli")
	agenttest.WriteFile(t, pkg, "package.json", `{"name":"@google/gemini-cli","version":"0.68.0"}`)
	script := agenttest.WriteFile(t, pkg, "bundle/gemini.js", "#!/usr/bin/env node\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	agenttest.SymlinkOrSkip(t, script, filepath.Join(bin, Binary))
	fx := &agenttest.FakeExec{Default: agentio.ProcessResult{Stdout: []byte("0.68.0\n")}}
	a := New(agenttest.NewHost(t, agenttest.WithEnv("PATH="+bin), agenttest.WithExec(fx)))
	in, err := a.Install()
	if err != nil || in.Version.String() != "0.68.0" || in.Method != agentio.InstallNPM {
		t.Fatalf("install %+v %v", in, err)
	}
	v, err := a.Runner().BinaryVersion(context.Background())
	if err != nil || v.String() != "0.68.0" {
		t.Fatalf("binary version %v %v", v, err)
	}
	if bv, ok := agentio.BinaryVersionOf(a.Neutral()); !ok || bv == nil {
		t.Fatal("binary version capability")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	f := setup(t)
	sf, err := ReadSettingsFile(agentio.ConfigUser, f.home.UserSettingsPath())
	if err != nil || !sf.Exists || len(sf.Warnings) != 0 {
		t.Fatalf("read %+v %v", sf, err)
	}
	s := sf.Settings
	if s.Security.Auth.SelectedType != AuthGoogleLogin || s.General.DefaultApprovalMode != ApprovalAutoEdit || s.Model.Name != "gemini-2.5-pro" {
		t.Fatalf("typed %+v", s)
	}
	if !slices.Equal(s.Unknown.Keys(), []string{"skills", "ui"}) || len(s.General.SessionRetention.Unknown) != 0 {
		t.Fatalf("unknown %v", s.Unknown.Keys())
	}
	if got := s.MCPServers["fixture-stdio"].Env["FIXTURE_TOKEN"].String(); strings.Contains(got, "fixture-secret") {
		t.Fatalf("env value printed: %s", got)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back Settings
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	b2, _ := json.Marshal(back)
	if !bytes.Equal(b, b2) || !bytes.Contains(b, []byte(`"hideBanner":true`)) {
		t.Fatalf("round trip:\n%s\n%s", b, b2)
	}
	// A nested unknown key and a type change at the top.
	odd, warns, err := DecodeSettings([]byte(`{"general":{"vimMode":true,"brandNew":1},"model":"wrong-type"}`))
	if err != nil || odd.General.Unknown["brandNew"] == nil || odd.Model != nil || len(warns) != 1 || odd.Unknown["model"] == nil {
		t.Fatalf("lenient %+v %v %v", odd, warns, err)
	}
	if out, _ := json.Marshal(odd); !bytes.Contains(out, []byte(`"brandNew":1`)) || !bytes.Contains(out, []byte(`"model":"wrong-type"`)) {
		t.Fatalf("lenient round trip %s", out)
	}
	var fl struct{ F StringList }
	if json.Unmarshal([]byte(`{"F":["A.md","B.md"]}`), &fl) != nil || len(fl.F) != 2 {
		t.Fatalf("string list %v", fl)
	}
}

func TestStripJSONComments(t *testing.T) {
	in := "{\n // c \"x\"\n \"a\": \"//not\", /* b\n */ \"c\": \"\\\"/*\"\n}"
	var m map[string]string
	if err := json.Unmarshal(StripJSONComments([]byte(in)), &m); err != nil || m["a"] != "//not" || m["c"] != `"/*` {
		t.Fatalf("%v %v", m, err)
	}
}

func TestSessions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ss, err := f.a.Sessions(ctx, f.home, "")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range ss {
		ids = append(ids, s.ID)
	}
	if !slices.Equal(ids, []string{sessChangelog, sessFlaky, sessLegacy}) {
		t.Fatalf("ids %v (empty %s and subagent %s must be skipped)", ids, sessEmpty, sessSubagent)
	}
	if ss[0].Title != "Changelog entry for the parser fix" || ss[1].Title != "fix the flaky test in parser_test.go" || ss[2].Title != "explain the build script" || !ss[2].Legacy {
		t.Fatalf("titles %q %q %q", ss[0].Title, ss[1].Title, ss[2].Title)
	}
	if ss[1].Updated.Format("15:04") != "10:05" || ss[1].Dir != f.project || ss[1].ProjectHash != ProjectHash(f.project) || ss[1].Slug != "demo" {
		t.Fatalf("session %+v", ss[1])
	}
	byDir, err := f.a.Sessions(ctx, f.home, f.project)
	if err != nil || len(byDir) != 3 {
		t.Fatalf("by dir %v %v", byDir, err)
	}
	if none, err := f.a.Sessions(ctx, f.home, t.TempDir()); err != nil || len(none) != 0 {
		t.Fatalf("other dir %v %v", none, err)
	}
	if _, err := f.a.Session(ctx, f.home, "nope"); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("missing %v", err)
	}
	if ok, err := f.a.HasHistory(f.home, f.project); !ok || err != nil {
		t.Fatalf("history %v %v", ok, err)
	}
	// Without the registry entry the .project_root markers still find it.
	if err := os.WriteFile(f.home.Path(projectsFile), []byte(`{"projects":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if slug, ok, err := f.a.ProjectSlug(f.home, f.project); slug != "demo" || !ok || err != nil {
		t.Fatalf("slug fallback %q %v %v", slug, ok, err)
	}
	conv, err := f.a.Neutral().(agentio.ConversationLister).Conversation(f.home.Neutral(), sessFlaky)
	if err != nil || conv.Dir != f.project || conv.Agent != ID {
		t.Fatalf("neutral conversation %+v %v", conv, err)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{"My Project": "my-project", "gemini_iwork.qepy": "gemini-iwork-qepy", "--A--b--": "a-b", "###": slugFallback, "": slugFallback} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	// sha256("/work/demo") computed with sha256sum.
	if got := ProjectHash("/work/demo"); got != "111b1182b4b056ca80f7335964bf62c7940d4990fccce4f5b91db3170297fb04" {
		t.Fatalf("hash %s", got)
	}
}

func TestInstructionFiles(t *testing.T) {
	f := setup(t)
	sub := filepath.Join(f.project, "sub")
	files, err := f.a.InstructionFiles(f.home, sub)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range files {
		rel, _ := filepath.Rel(filepath.Dir(f.project), x.Path)
		if x.Scope == agentio.InstructionUser {
			rel = "~" + string(filepath.Separator) + filepath.Base(x.Path)
		}
		got = append(got, x.Scope.String()+":"+filepath.ToSlash(rel))
	}
	want := []string{"user:~/GEMINI.md", "project:demo/GEMINI.md", "project:demo/sub/GEMINI.md"}
	if !slices.Equal(got, want) {
		t.Fatalf("files %v, want %v", got, want)
	}
	// A folder the trust file refuses loads no project file.
	trust, _ := json.Marshal(map[string]TrustLevel{f.project: DoNotTrust})
	if err := os.WriteFile(f.home.Path(trustedFoldersFile), trust, 0o600); err != nil {
		t.Fatal(err)
	}
	files, err = f.a.InstructionFiles(f.home, sub)
	if err != nil || len(files) != 1 {
		t.Fatalf("untrusted %v %v", files, err)
	}
}

func TestMCPServers(t *testing.T) {
	f := setup(t)
	ss, err := f.a.MCPServers(f.home, f.project)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]MCPServer{}
	for _, s := range ss {
		got[s.Name] = s
	}
	if len(got) != 5 {
		t.Fatalf("servers %v", got)
	}
	if s := got["fixture-stdio"].Neutral(); s.Transport != agentio.MCPStdio || s.Scope != agentio.ConfigUser || !slices.Equal(s.EnvNames, []string{"FIXTURE_TOKEN"}) {
		t.Fatalf("stdio %+v", s)
	}
	if got["fixture-http"].Config.Transport() != agentio.MCPHTTP || got["project-server"].Config.Transport() != agentio.MCPSSE || got["project-server"].Scope != agentio.ConfigProject {
		t.Fatal("transports or scope")
	}
	if e := got["disabled-one"].Enabled; e == nil || *e {
		t.Fatal("disabled-one must be disabled by mcp-server-enablement.json")
	}
	if s := got["ext-server"]; s.Extension != "demo-ext" || s.Scope != agentio.ConfigPlugin {
		t.Fatalf("extension server %+v", s)
	}
	out, _ := json.Marshal(ss)
	if bytes.Contains(out, []byte("fixture-secret")) {
		t.Fatal("an MCP env value leaked into JSON")
	}
}

func TestSkillsAndExtensions(t *testing.T) {
	f := setup(t)
	xs, err := f.a.Extensions(f.home, f.project)
	if err != nil || len(xs) != 1 || !xs[0].Enabled || xs[0].Manifest.Version != "1.2.0" || xs[0].Manifest.Unknown["settings"] == nil {
		t.Fatalf("extensions %+v %v", xs, err)
	}
	if enabledFor(extensionOverrides{Overrides: []string{"!/elsewhere/*"}}, filepath.FromSlash("/elsewhere/x")) {
		t.Fatal("an override rule must disable below its glob")
	}
	if !enabledFor(extensionOverrides{Overrides: []string{"/a/*", "!/a/*"}}, filepath.FromSlash("/b")) {
		t.Fatal("no match keeps the default")
	}
	ss, err := f.a.Skills(f.home, f.project)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]bool{}
	for _, s := range ss {
		state[s.Name] = s.Enabled
	}
	if !state["review"] || state["old-skill"] || !state["ext-skill"] || len(state) != 3 {
		t.Fatalf("skills %v", state)
	}
}

func TestConfigReader(t *testing.T) {
	f := setup(t)
	layers, err := f.a.ConfigLayers(f.home, f.project)
	if err != nil || len(layers) != 4 || layers[1].Scope != agentio.ConfigProject || !layers[1].Exists || layers[2].Scope != agentio.ConfigUser {
		t.Fatalf("layers %+v %v", layers, err)
	}
	d, err := f.a.Defaults(f.home, f.project)
	if err != nil || d.Model != "gemini-2.5-flash" || d.Autonomy != agentio.AutonomyAcceptEdits {
		t.Fatalf("defaults %+v %v", d, err)
	}
	if tr, err := f.a.ProjectTrust(f.home, filepath.Join(f.project, "sub")); tr != agentio.TrustTrusted || err != nil {
		t.Fatalf("trust %v %v", tr, err)
	}
	if tr, _ := f.a.ProjectTrust(f.home, t.TempDir()); tr != agentio.TrustUnknown {
		t.Fatalf("unknown trust %v", tr)
	}
	rules := map[string]TrustLevel{filepath.FromSlash("/w/a/b"): TrustParent}
	if TrustOf(rules, filepath.FromSlash("/w/a/c")) != agentio.TrustTrusted {
		t.Fatal("TRUST_PARENT trusts the parent")
	}
	// GEMINI_CLI_SYSTEM_SETTINGS_PATH moves the system layer.
	sys := agenttest.WriteFile(t, t.TempDir(), "system.json", `{"model":{"name":"forced"}}`)
	f.host.Env = append(f.host.Env, SystemSettingsEnv+"="+sys)
	if d, _ := New(f.host).Defaults(f.home, f.project); d.Model != "forced" {
		t.Fatalf("system wins %+v", d)
	}
}

func TestIdentity(t *testing.T) {
	f := setup(t)
	id, err := f.a.Identity(f.home)
	if err != nil || !id.LoggedIn || id.AuthType != AuthGoogleLogin || id.Credential != agentio.CredentialFile || id.ExpiresAt == nil {
		t.Fatalf("identity %+v %v", id, err)
	}
	if id.Email.String() == "user@example.com" {
		t.Fatal("email must redact")
	}
	agenttest.WriteFile(t, f.home.Dir(), settingsFile, `{"security":{"auth":{"selectedType":"gemini-api-key"}}}`)
	if id, _ := f.a.Identity(f.home); id.LoggedIn || id.Billing != agentio.BillingPerToken {
		t.Fatalf("api key home without env %+v", id)
	}
	f.host.Env = append(f.host.Env, APIKeyEnv+"=k")
	if id, _ := New(f.host).Identity(f.home); !id.LoggedIn || id.Credential != agentio.CredentialEnv {
		t.Fatalf("api key home %+v", id)
	}
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

func TestLaunch(t *testing.T) {
	f := setup(t, agenttest.WithEnv("GEMINI_API_KEY=user-key", "GOOGLE_CLOUD_ACCESS_TOKEN=x", "GOOGLE_GENAI_USE_GCA=true"))
	n := f.a.Neutral()
	def := f.home.Target()
	for lvl, want := range map[agentio.Autonomy][]string{
		agentio.AutonomyDefault:     nil,
		agentio.AutonomyReadOnly:    {flagApprovalMode, "plan"},
		agentio.AutonomyAsk:         {flagApprovalMode, "default"},
		agentio.AutonomyAcceptEdits: {flagApprovalMode, "auto_edit"},
	} {
		cmd, err := n.Launch(def, agentio.LaunchRequest{Autonomy: lvl})
		if err != nil || !slices.Equal(cmd.Args, want) {
			t.Errorf("%s: %v %v, want %v", lvl, cmd.Args, err, want)
		}
	}
	if _, err := n.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}); !errors.Is(err, agentio.ErrDangerousNotAllowed) {
		t.Fatalf("full needs AllowDangerous: %v", err)
	}
	full := def
	full.AllowDangerous = true
	cmd, err := n.Launch(full, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull, Headless: true, Prompt: "fix it", Model: "gemini-2.5-flash", AddDirs: []string{"../lib"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{flagApprovalMode, "yolo", flagOutputFormat, "stream-json", flagModel, "gemini-2.5-flash", flagIncludeDirs, "../lib", flagPrompt, "fix it"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("headless args %v, want %v", cmd.Args, want)
	}
	// Home login: the override variables go, the user's API key stays.
	if len(envVals(cmd.Env, CloudAccessTokenEnv)) != 0 || len(envVals(cmd.Env, UseGCAEnv)) != 0 || !slices.Equal(envVals(cmd.Env, APIKeyEnv), []string{"user-key"}) {
		t.Fatalf("env %v", cmd.Env)
	}
	cmd, err = n.Launch(def, agentio.LaunchRequest{Prompt: "hello", ResumeID: sessFlaky})
	if err != nil || !slices.Equal(cmd.Args, []string{flagResume, sessFlaky, flagPromptInteractive, "hello"}) {
		t.Fatalf("resume %v %v", cmd.Args, err)
	}
	// Token logins: Gemini key and Vertex express key.
	cmd, err = n.Launch(def, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken("k2")}})
	if err != nil || !slices.Equal(envVals(cmd.Env, APIKeyEnv), []string{"k2"}) {
		t.Fatalf("token %v %v", cmd.Env, err)
	}
	cmd, err = n.Launch(def, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Provider: ProviderVertex, Token: agentio.NewToken("v1")}})
	if err != nil || !slices.Equal(envVals(cmd.Env, GoogleAPIKeyEnv), []string{"v1"}) {
		t.Fatalf("vertex token %v %v", cmd.Env, err)
	}
	if _, err := n.Launch(def, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("o")}}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Fatalf("oauth token %v", err)
	}
	// Homes: the default removes GEMINI_CLI_HOME, another sets its parent.
	stale := def
	stale.ExtraEnv = []agentio.EnvChange{agentio.EnvSet(HomeEnv, "/stale")}
	if cmd, err := n.Launch(stale, agentio.LaunchRequest{}); err != nil || len(envVals(cmd.Env, HomeEnv)) != 0 {
		t.Fatalf("default home env %v %v", cmd.Env, err)
	}
	work := f.a.HomeAt(filepath.Join(f.host.UserHome, "gemini-work", defaultDirName))
	if cmd, err := n.Launch(work.Target(), agentio.LaunchRequest{}); err != nil || !slices.Equal(envVals(cmd.Env, HomeEnv), []string{filepath.Join(f.host.UserHome, "gemini-work")}) {
		t.Fatalf("work home env %v %v", cmd.Env, err)
	}
	if cmd.Path != Binary {
		t.Fatalf("path %s", cmd.Path)
	}
	// TokenLogin through the accessor.
	tl, ok := agentio.TokenLoginOf(n)
	if !ok {
		t.Fatal("token login")
	}
	if k, ok := tl.DetectTokenKind(agentio.NewToken("AIza-fake")); !ok || k != agentio.TokenAPIKey {
		t.Fatalf("detect %v %v", k, ok)
	}
	if _, err := tl.TokenEnv(agentio.TokenCredential{Kind: agentio.TokenAPIKey, Provider: "openai", Token: agentio.NewToken("x")}); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("unknown provider %v", err)
	}
}

func TestClassifyProcess(t *testing.T) {
	a := New(agenttest.NewHost(t))
	for _, tc := range []struct {
		argv []string
		kind agentio.ProcessKind
		ok   bool
	}{
		{[]string{"node", "/usr/local/bin/gemini"}, agentio.ProcessCLI, true},
		{[]string{"/opt/node/bin/node", "--max-old-space-size=8192", "/usr/local/bin/gemini", "-p", "hi"}, agentio.ProcessExec, true},
		{[]string{"node", "/x/lib/node_modules/@google/gemini-cli/bundle/gemini.js", "mcp", "list"}, agentio.ProcessLauncher, true},
		{[]string{`C:\Program Files\nodejs\node.exe`, `C:\Users\u\AppData\Roaming\npm\node_modules\@google\gemini-cli\bundle\gemini.js`}, agentio.ProcessCLI, true},
		{[]string{"gemini", "--resume", "latest"}, agentio.ProcessCLI, true},
		{[]string{"node", "/srv/app/server.js", "gemini"}, "", false},
		{[]string{"geminid"}, "", false},
	} {
		info, ok := a.ClassifyProcess(agentio.Process{Argv: tc.argv})
		if ok != tc.ok || info.Kind != tc.kind {
			t.Errorf("%v: %+v %v, want %s %v", tc.argv, info, ok, tc.kind, tc.ok)
		}
	}
}

func TestNeutralCapabilities(t *testing.T) {
	f := setup(t)
	n := f.a.Neutral()
	if got := agentio.Supported(n); !slices.Equal(got, supported()) {
		t.Fatalf("supported %v, want %v", got, supported())
	}
	for _, c := range agentio.CapabilityValues() {
		if slices.Contains(supported(), c) {
			continue
		}
		if r, ok := agentio.Unsupported(n, c); !ok || r == "" || r == "not modelled for Gemini CLI" {
			t.Errorf("%s: no specific reason (%q)", c, r)
		}
	}
	if _, ok := agentio.LoginCommanderOf(n); ok {
		t.Fatal("Gemini has no login command")
	}
	doc, _ := agentio.DoctorOf(n)
	checks := doc.Checks(context.Background(), f.home.Neutral())
	ids := map[agentio.CheckID]agentio.CheckStatus{}
	for _, c := range checks {
		ids[c.ID] = c.Status
	}
	if ids[agentio.CheckLogin] != agentio.CheckOK || ids[CheckSettingsParse] != agentio.CheckOK || ids[agentio.CheckVersionTested] != agentio.CheckWarn {
		t.Fatalf("checks %v", checks)
	}
	if _, ok := ids[CheckAutoUpdate]; ok {
		t.Fatal("auto update is off in the fixture")
	}
	cl, _ := agentio.ConversationsOf(n)
	convs, err := cl.Conversations(context.Background(), f.home.Neutral(), agentio.ConversationQuery{Limit: 2})
	if err != nil || len(convs) != 2 {
		t.Fatalf("limit %v %v", convs, err)
	}
	hl, _ := agentio.HomeLinkerOf(n)
	links, err := hl.PlanHomeLinks(f.home.Neutral(), f.a.HomeAt(filepath.Join(t.TempDir(), defaultDirName)).Neutral())
	if err != nil || len(links) != len(sharedHomeFiles()) {
		t.Fatalf("links %v %v", links, err)
	}
}
