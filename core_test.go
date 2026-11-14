package agentcli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agentcli"
	"github.com/khanakia/agentcli/agenttest"
)

// sampleSpec is a config-only agent shaped like a real CLI.
func sampleSpec() agentcli.Spec {
	return agentcli.Spec{
		ID: "toy", Name: "Toy Agent", Binary: "toy",
		HomeEnv: "TOY_HOME", DefaultHome: ".toy", ExtraHomes: ".toy-*",
		HomeMarkers: []string{"config.json"},
		NewArgs:     []string{"{args}"}, ResumeArgs: []string{"--resume", "{id}", "{args}"},
		ModelArgs: []string{"--model", "{model}"}, AddDirArgs: []string{"--add-dir={dir}"},
		PromptArgs:  []string{"{prompt}"},
		VersionArgs: []string{"--version"}, LoginArgs: []string{"login"}, LogoutArgs: []string{"logout"},
		SharedHomeFiles: []string{"AGENTS.md", "settings.json"},
		CredentialEnv:   []string{"TOY_API_KEY", "TOY_TOKEN"}, APIBillingEnv: []string{"TOY_API_KEY"},
		Process: agentcli.ProcessMatch{Basenames: []string{"toy"}, ArgvContains: []string{"@toy/cli"}, Exclude: []string{"Toy.app"}},
	}
}

// newToy builds the sample agent on a fixture host with a default and a
// work home on disk.
func newToy(t *testing.T, opts ...agenttest.HostOption) (agentcli.Agent, agentcli.Host) {
	t.Helper()
	host := agenttest.NewHost(t, opts...)
	agenttest.WriteFile(t, host.UserHome, ".toy/config.json", "{}")
	agenttest.WriteFile(t, host.UserHome, ".toy-work/config.json", "{}")
	agenttest.WriteFile(t, host.UserHome, ".toy-empty/other", "")
	a, err := agentcli.FromSpec(sampleSpec(), host)
	if err != nil {
		t.Fatal(err)
	}
	return a, host
}

func TestFromSpec_Conformance(t *testing.T) {
	a, host := newToy(t)
	agenttest.Conformance(t, a, agentcli.NewHome("toy", filepath.Join(host.UserHome, ".toy-work"), false, "work"))
}

func TestFromSpec_Validation(t *testing.T) {
	t.Parallel()
	host := agenttest.NewHost(t)
	for name, mut := range map[string]func(*agentcli.Spec){
		"bad id":           func(s *agentcli.Spec) { s.ID = "Toy" },
		"no binary":        func(s *agentcli.Spec) { s.Binary = "" },
		"env without home": func(s *agentcli.Spec) { s.DefaultHome = "" },
		"absolute home":    func(s *agentcli.Spec) { s.DefaultHome = "/etc/toy" },
		"escaping home":    func(s *agentcli.Spec) { s.DefaultHome = "../toy" },
		"resume no id":     func(s *agentcli.Spec) { s.ResumeArgs = []string{"--resume"} },
		"model no slot":    func(s *agentcli.Spec) { s.ModelArgs = []string{"--model"} },
		"bad glob":         func(s *agentcli.Spec) { s.ExtraHomes = "[" },
	} {
		s := sampleSpec()
		mut(&s)
		if _, err := agentcli.FromSpec(s, host); !errors.Is(err, agentcli.ErrInvalidSpec) {
			t.Errorf("%s: err = %v, want ErrInvalidSpec", name, err)
		}
	}
}

func TestSpecLaunch_Matrix(t *testing.T) {
	t.Parallel()
	a, host := newToy(t)
	def := a.DefaultHome()
	work := agentcli.NewHome("toy", filepath.Join(host.UserHome, ".toy-work"), false, "work")
	tgt := work.Target()
	tgt.ExtraArgs = []string{"--profile-arg"}
	cmd, err := a.Launch(tgt, agentcli.LaunchRequest{Dir: "/p", ResumeID: "abc", Model: "m1", AddDirs: []string{"/a", "/b"}, Prompt: "hi there", Args: []string{"--last"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--resume", "abc", "--profile-arg", "--model", "m1", "--add-dir=/a", "--add-dir=/b", "--last", "hi there"}
	if !slices.Equal(cmd.Args, want) || cmd.Path != "toy" || cmd.Dir != "/p" {
		t.Errorf("cmd = %s %v dir %s, want %v", cmd.Path, cmd.Args, cmd.Dir, want)
	}
	for name, tc := range map[string]struct {
		t   agentcli.Target
		r   agentcli.LaunchRequest
		err error
	}{
		"full without allow": {def.Target(), agentcli.LaunchRequest{Autonomy: agentcli.AutonomyFull}, agentcli.ErrDangerousNotAllowed},
		"full with allow":    {agentcli.Target{Home: def, AllowDangerous: true}, agentcli.LaunchRequest{Autonomy: agentcli.AutonomyFull}, agentcli.ErrUnsupported},
		"read-only":          {def.Target(), agentcli.LaunchRequest{Autonomy: agentcli.AutonomyReadOnly}, agentcli.ErrUnsupported},
		"headless":           {def.Target(), agentcli.LaunchRequest{Headless: true}, agentcli.ErrUnsupported},
		"token":              {def.Target(), agentcli.LaunchRequest{Credential: &agentcli.TokenCredential{}}, agentcli.ErrUnsupported},
		"foreign home":       {agentcli.NewHome("other", "/x", false, "x").Target(), agentcli.LaunchRequest{}, agentcli.ErrForeignHome},
	} {
		if _, err := a.Launch(tc.t, tc.r); !errors.Is(err, tc.err) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.err)
		}
	}
	bare := sampleSpec()
	bare.ID, bare.ModelArgs, bare.PromptArgs, bare.ResumeArgs = "bare", nil, nil, nil
	b, err := agentcli.FromSpec(bare, host)
	if err != nil {
		t.Fatal(err)
	}
	for name, r := range map[string]agentcli.LaunchRequest{"model": {Model: "x"}, "prompt": {Prompt: "x"}, "resume": {ResumeID: "x"}} {
		if _, err := b.Launch(b.DefaultHome().Target(), r); !errors.Is(err, agentcli.ErrUnsupported) {
			t.Errorf("bare %s: err = %v, want ErrUnsupported", name, err)
		}
	}
}

// TestCoreEnvChanges_TokenScrubAndHomeRuleWin: ExtraEnv cannot override the
// home rule or reintroduce a scrubbed credential (invariants 2 and 9).
func TestCoreEnvChanges_TokenScrubAndHomeRuleWin(t *testing.T) {
	t.Parallel()
	host := agenttest.NewHost(t, agenttest.WithEnv("PATH=/bin", "TOY_TOKEN=inherited", "TOY_HOME=/stale"))
	core, err := agentcli.NewCore(sampleSpec(), host)
	if err != nil {
		t.Fatal(err)
	}
	tgt := core.DefaultHome().Target()
	tgt.ExtraEnv = []agentcli.EnvChange{agentcli.EnvSet("TOY_HOME", "/override"), agentcli.EnvSet("TOY_API_KEY", "profile-key"), agentcli.EnvSet("KEEP", "1")}
	tok := agentcli.NewToken("tok-1")
	cmd, err := core.Command(tgt, "", nil, []agentcli.EnvChange{agentcli.EnvSetSecret("TOY_TOKEN", tok)})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, k := range []string{"TOY_HOME", "TOY_API_KEY", "TOY_TOKEN", "KEEP", "PATH"} {
		got[k] = envValsExt(cmd.Env, k)
	}
	if len(got["TOY_HOME"]) != 0 || len(got["TOY_API_KEY"]) != 0 || !slices.Equal(got["TOY_TOKEN"], []string{"tok-1"}) || !slices.Equal(got["KEEP"], []string{"1"}) || !slices.Equal(got["PATH"], []string{"/bin"}) {
		t.Errorf("env = %v", got)
	}
	if !slices.Contains(cmd.SecretEnvNames(), "TOY_TOKEN") || slices.Contains(cmd.Redacted().Env, "TOY_TOKEN=tok-1") {
		t.Error("token env must be marked secret and redacted")
	}
	// Without a token login, inherited credentials stay (the home login may rely on them).
	cmd, _ = core.Command(core.DefaultHome().Target(), "", nil, nil)
	if !slices.Equal(envValsExt(cmd.Env, "TOY_TOKEN"), []string{"inherited"}) {
		t.Error("no token login: credential env untouched")
	}
}

func envValsExt(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			out = append(out, v)
		}
	}
	return out
}

func TestHomesDiscovery(t *testing.T) {
	t.Parallel()
	a, host := newToy(t, agenttest.WithEnv("TOY_HOME=/nonexistent"))
	homes, err := a.Homes()
	if err != nil {
		t.Fatal(err)
	}
	if len(homes) != 2 || !homes[0].IsDefault() || homes[1].Name() != "work" || homes[1].Dir() != filepath.Join(host.UserHome, ".toy-work") {
		t.Errorf("homes = %v", homes)
	}
	h, ok := a.HomeFromEnv()
	if !ok || h.Dir() != "/nonexistent" || h.IsDefault() {
		t.Errorf("HomeFromEnv = %v,%v", h, ok)
	}
	b, _ := newToy(t, agenttest.WithEnv("TOY_HOME="))
	if _, ok := b.HomeFromEnv(); ok {
		t.Error("empty home variable counts as absent")
	}
	if ok, err := a.IsHome(filepath.Join(host.UserHome, ".toy-empty")); ok || err != nil {
		t.Errorf("dir without marker: %v %v", ok, err)
	}
}

func TestSupportedAndGates(t *testing.T) {
	t.Parallel()
	a, _ := newToy(t)
	got := agentcli.Supported(a)
	want := []agentcli.Capability{agentcli.CapBinaryVersion, agentcli.CapLoginCommander, agentcli.CapProcessClassifier, agentcli.CapHomeLinker, agentcli.CapDoctor}
	if !slices.Equal(got, want) {
		t.Errorf("Supported = %v, want %v", got, want)
	}
	if r, ok := agentcli.Unsupported(a, agentcli.CapUsage); !ok || r == "" {
		t.Error("config-only agent must declare why usage is missing")
	}
	s := sampleSpec()
	s.ID, s.LoginArgs, s.SharedHomeFiles, s.VersionArgs, s.Process = "min", nil, nil, nil, agentcli.ProcessMatch{}
	m, err := agentcli.FromSpec(s, agenttest.NewHost(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := agentcli.Supported(m); !slices.Equal(got, []agentcli.Capability{agentcli.CapDoctor}) {
		t.Errorf("minimal Supported = %v", got)
	}
	// Every capability has a non-zero cost row except the pure ones.
	for _, c := range agentcli.CapabilityValues() {
		if c.Cost().Class == agentcli.CostPure && c != agentcli.CapTokenLogin && c != agentcli.CapLoginCommander && c != agentcli.CapProcessClassifier && c != agentcli.CapRunEvents {
			t.Errorf("%s: missing cost row", c)
		}
	}
	if !agentcli.CapBinaryVersion.Cost().MayTouchLogin || agentcli.CapHistoryMover.Cost().Access != agentcli.Write {
		t.Error("cost table")
	}
}

func TestClassifyProcess(t *testing.T) {
	t.Parallel()
	a, _ := newToy(t)
	agents := []agentcli.Agent{a}
	for name, tc := range map[string]struct {
		p  agentcli.Process
		ok bool
	}{
		"argv0":          {agentcli.Process{Argv: []string{"/usr/local/bin/toy", "--x"}}, true},
		"exe":            {agentcli.Process{Exe: "/opt/toy"}, true},
		"npm launcher":   {agentcli.Process{Argv: []string{"node", "/lib/node_modules/@toy/cli/bin.js"}}, true},
		"excluded app":   {agentcli.Process{Exe: "/Applications/Toy.app/Contents/MacOS/toy"}, false},
		"other":          {agentcli.Process{Argv: []string{"vim", "toy.txt"}}, false},
		"prefix not hit": {agentcli.Process{Argv: []string{"toybox"}}, false},
	} {
		info, ok := agentcli.ClassifyProcess(agents, tc.p)
		if ok != tc.ok || (ok && (info.Agent != "toy" || info.Kind != agentcli.ProcessCLI)) {
			t.Errorf("%s: %+v,%v want ok=%v", name, info, ok, tc.ok)
		}
	}
}

// liveAgent is a minimal agent with live sessions, for FindLiveSession.
type liveAgent struct {
	*agentcli.Core
	sessions map[string][]agentcli.LiveSession
}

func (l liveAgent) LiveSessions(h agentcli.Home) ([]agentcli.LiveSession, error) {
	return l.sessions[h.Dir()], nil
}

func (l liveAgent) IsLive(agentcli.Home, string) (bool, error) { return false, nil }

func TestFindLiveSession(t *testing.T) {
	t.Parallel()
	_, host := newToy(t)
	core, err := agentcli.NewCore(sampleSpec(), host)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(host.UserHome, ".toy-work")
	la := liveAgent{Core: core, sessions: map[string][]agentcli.LiveSession{work: {{PID: 42, ConversationID: "c1"}}}}
	a, s, ok, err := agentcli.FindLiveSession([]agentcli.Agent{la}, 42)
	if err != nil || !ok || s.ConversationID != "c1" || a.ID() != "toy" {
		t.Errorf("FindLiveSession = %v %+v %v %v", a, s, ok, err)
	}
	if _, _, ok, _ := agentcli.FindLiveSession([]agentcli.Agent{la}, 7); ok {
		t.Error("unknown pid found")
	}
}

func TestHomeLinks(t *testing.T) {
	t.Parallel()
	a, host := newToy(t)
	agenttest.WriteFile(t, host.UserHome, ".toy/AGENTS.md", "rules")
	agenttest.WriteFile(t, host.UserHome, ".toy/settings.json", "{}")
	agenttest.WriteFile(t, host.UserHome, ".toy-new/settings.json", "mine")
	linker, ok := agentcli.HomeLinkerOf(a)
	if !ok {
		t.Fatal("no home linker")
	}
	h := agentcli.NewHome("toy", filepath.Join(host.UserHome, ".toy-new"), false, "new")
	links, err := linker.PlanHomeLinks(a.DefaultHome(), h)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].Exists || !links[1].Exists {
		t.Fatalf("plan = %+v", links)
	}
	if err := linker.ApplyHomeLinks(links); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(h.Path("AGENTS.md")); string(b) != "rules" {
		t.Error("AGENTS.md not linked")
	}
	if b, _ := os.ReadFile(h.Path("settings.json")); string(b) != "mine" {
		t.Error("existing file overwritten")
	}
	if _, err := linker.PlanHomeLinks(a.DefaultHome(), agentcli.NewHome("x", "/x", false, "")); !errors.Is(err, agentcli.ErrForeignHome) {
		t.Errorf("foreign: %v", err)
	}
}

func TestRunnerVersion(t *testing.T) {
	t.Parallel()
	fx := &agenttest.FakeExec{Results: map[string]agentcli.ProcessResult{"toy --version": {Stdout: []byte("toy-cli 1.4.2\n")}}}
	a, _ := newToy(t, agenttest.WithExec(fx))
	bv, ok := agentcli.BinaryVersionOf(a)
	if !ok {
		t.Fatal("no binary version")
	}
	v, err := bv.BinaryVersion(context.Background())
	if err != nil || v.String() != "1.4.2" {
		t.Errorf("version = %v, %v", v, err)
	}
	if len(fx.Calls) != 1 || len(envValsExt(fx.Calls[0].Env, "TOY_HOME")) != 0 {
		t.Errorf("calls = %v", fx.Calls)
	}
}

func TestLookPath(t *testing.T) {
	t.Parallel()
	dir1, dir2 := t.TempDir(), t.TempDir()
	for _, d := range []string{dir1, dir2} {
		p := filepath.Join(d, "toy")
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir1, "noexec"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	list := strings.Join([]string{"", "relative", dir1, dir2}, string(os.PathListSeparator))
	all := agentcli.LookPathAll(list, "toy")
	if len(all) != 2 || all[0] != filepath.Join(dir1, "toy") {
		t.Errorf("LookPathAll = %v", all)
	}
	if _, err := agentcli.LookPath(list, "noexec"); !errors.Is(err, agentcli.ErrNotInstalled) {
		t.Errorf("non-executable: %v", err)
	}
	a, _ := newToy(t, agenttest.WithEnv("PATH="+list))
	if p, err := a.LookPath(); err != nil || p != all[0] {
		t.Errorf("agent LookPath = %s, %v", p, err)
	}
}

func TestDoctorL1(t *testing.T) {
	t.Parallel()
	a, _ := newToy(t, agenttest.WithEnv("PATH=/none", "TOY_API_KEY=x", "TOY_HOME=/y"))
	d, _ := agentcli.DoctorOf(a)
	got := map[agentcli.CheckID]agentcli.CheckStatus{}
	for _, c := range d.Checks(context.Background(), a.DefaultHome()) {
		got[c.ID] = c.Status
		if strings.Contains(c.Message, "=x") {
			t.Errorf("check leaks a value: %s", c.Message)
		}
	}
	if got[agentcli.CheckBinary] != agentcli.CheckFail || got[agentcli.CheckHome] != agentcli.CheckOK || got[agentcli.CheckHomeEnv] != agentcli.CheckInfo || got[agentcli.CheckAPIBillingEnv] != agentcli.CheckWarn {
		t.Errorf("checks = %v", got)
	}
	if c := agentcli.VersionCheck(agentcli.Compat{Min: agentcli.MustVersion("1.0.0"), Max: agentcli.MustVersion("1.0.0")}, agentcli.MustVersion("1.1.0")); c.Status != agentcli.CheckWarn {
		t.Errorf("VersionCheck newer = %v", c)
	}
}

// TestHostFromOS_ReadsProcessTable is a read-only smoke test of the real
// host on linux: the test process itself must be in the table.
func TestHostFromOS_ReadsProcessTable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux /proc reader")
	}
	h, err := agentcli.HostFromOS()
	if err != nil {
		t.Fatal(err)
	}
	procs, err := h.Processes.Processes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	self := os.Getpid()
	i := slices.IndexFunc(procs, func(p agentcli.Process) bool { return p.PID == self })
	if i < 0 || procs[i].Start.IsZero() || procs[i].PPID == 0 || len(procs[i].Argv) == 0 {
		t.Fatalf("self row = %+v (found %v)", procs, i >= 0)
	}
	cwd, _ := os.Getwd()
	if got, err := h.Processes.Cwd(context.Background(), self); err != nil || got != cwd {
		t.Errorf("Cwd = %s, %v", got, err)
	}
	// No keychain reader off darwin: "cannot look" must not read as "no entry".
	if _, found, err := h.Secrets.Get(context.Background(), "agentcli-test-none", ""); found || !errors.Is(err, agentcli.ErrUnsupported) {
		t.Errorf("linux secrets: %v %v", found, err)
	}
}

// tokenAgent is a spec agent with a token login, so the conformance
// suite's token-redaction branch runs on the root alone.
type tokenAgent struct{ *agentcli.Core }

func (tokenAgent) TokenKinds() []agentcli.TokenKind {
	return []agentcli.TokenKind{agentcli.TokenAPIKey}
}

func (tokenAgent) DetectTokenKind(agentcli.Token) (agentcli.TokenKind, bool) {
	return agentcli.TokenAPIKey, true
}

func (tokenAgent) TokenEnv(c agentcli.TokenCredential) ([]agentcli.EnvChange, error) {
	return []agentcli.EnvChange{agentcli.EnvSetSecret("TOY_API_KEY", c.Token)}, nil
}

func (tokenAgent) TokenSetupCommand(agentcli.Home) (agentcli.Command, bool) {
	return agentcli.Command{}, false
}

// Launch builds argv from the spec but routes a credential through TokenEnv.
func (a tokenAgent) Launch(t agentcli.Target, r agentcli.LaunchRequest) (agentcli.Command, error) {
	cred := r.Credential
	r.Credential = nil
	cmd, err := a.Core.Launch(t, r)
	if err != nil || cred == nil {
		return cmd, err
	}
	env, err := a.TokenEnv(*cred)
	if err != nil {
		return agentcli.Command{}, err
	}
	return a.Command(t, cmd.Dir, cmd.Args, env)
}

func TestConformance_TokenAgent(t *testing.T) {
	_, host := newToy(t)
	core, err := agentcli.NewCore(sampleSpec(), host)
	if err != nil {
		t.Fatal(err)
	}
	a := tokenAgent{Core: core}
	if _, ok := agentcli.TokenLoginOf(a); !ok {
		t.Fatal("token login not detected")
	}
	agenttest.Conformance(t, a, a.DefaultHome())
}
