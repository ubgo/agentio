package agenttest

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
)

// Conformance probe values. The secret is distinctive so a leak is
// unambiguous in any formatted output.
const (
	conformanceSecret   = "sk_agenttest-CONFORMANCE-SECRET-7f3a"
	conformanceResumeID = "agenttest-resume-0001"
	conformanceDir      = "/agenttest/project"
)

// Conformance runs the shared suite (SPEC §10) on a, whose Host must come
// from NewHost (or carry FailingHTTP and FailExec bound to t), and
// fixtureHome, a home of a with fixture files. It checks:
//
//   - the default home launches with Spec.HomeEnv absent, a non-default
//     home with it set exactly once to the home dir (invariant 2);
//   - ResumeID lands in Args (when the agent supports resume);
//   - Supported matches the accessors and every listed capability has a cost;
//   - every read accessor runs on fixtureHome without reaching Host.Exec
//     or Host.HTTP (the fakes fail the test on use);
//   - a token login keeps the token out of Args, errors, String(), fmt
//     verbs and JSON, and only Command.Env holds it (invariant 8).
//
// Unknown-key preservation is format-specific and stays in each vendor's
// reader tests.
func Conformance(t *testing.T, a agentio.Agent, fixtureHome agentio.Home) {
	t.Helper()
	t.Run("home-env", func(t *testing.T) { conformHomeEnv(t, a, fixtureHome) })
	t.Run("resume", func(t *testing.T) { conformResume(t, a) })
	t.Run("supported", func(t *testing.T) { conformSupported(t, a) })
	t.Run("read-accessors", func(t *testing.T) { conformReads(t, a, fixtureHome) })
	t.Run("token-redaction", func(t *testing.T) { conformToken(t, a) })
}

// conformHomeEnv checks invariant 2 on the default and a non-default home.
func conformHomeEnv(t *testing.T, a agentio.Agent, fixture agentio.Home) {
	spec := a.Spec()
	def := a.DefaultHome()
	if def.Agent() != a.ID() || !def.IsDefault() {
		t.Fatalf("DefaultHome() = %v, want a default home of %s", def, a.ID())
	}
	if spec.HomeEnv == "" {
		return
	}
	defStale := def.Target()
	defStale.ExtraEnv = []agentio.EnvChange{agentio.EnvSet(spec.HomeEnv, "/stale")}
	cmd, err := a.Launch(defStale, agentio.LaunchRequest{})
	if err != nil {
		t.Fatalf("Launch(default): %v", err)
	}
	if vals := envValues(cmd.Env, spec.HomeEnv); len(vals) != 0 {
		t.Errorf("default home: %s present %v, want absent", spec.HomeEnv, vals)
	}
	other := fixture
	if other.IsDefault() {
		other = agentio.NewHome(a.ID(), def.Dir()+"-agenttest", false, "agenttest")
	}
	cmd, err = a.Launch(other.Target(), agentio.LaunchRequest{})
	if err != nil {
		t.Fatalf("Launch(non-default): %v", err)
	}
	if vals := envValues(cmd.Env, spec.HomeEnv); len(vals) != 1 || vals[0] != other.Dir() {
		t.Errorf("non-default home: %s = %v, want [%s]", spec.HomeEnv, vals, other.Dir())
	}
	foreign := agentio.NewHome("agenttest-foreign", "/x", false, "x")
	if _, err := a.Launch(foreign.Target(), agentio.LaunchRequest{}); err == nil {
		t.Errorf("Launch(foreign home) = nil error, want ErrForeignHome")
	}
}

// envValues returns every value of key in env.
func envValues(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			out = append(out, v)
		}
	}
	return out
}

// conformResume checks the resume id is placed in argv.
func conformResume(t *testing.T, a agentio.Agent) {
	cmd, err := a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{ResumeID: conformanceResumeID})
	if err != nil {
		t.Skipf("resume not supported: %v", err)
	}
	if !slices.ContainsFunc(cmd.Args, func(s string) bool { return strings.Contains(s, conformanceResumeID) }) {
		t.Errorf("resume id missing from args %v", cmd.Args)
	}
}

// conformSupported checks Supported against Unsupported and costs.
func conformSupported(t *testing.T, a agentio.Agent) {
	sup := agentio.Supported(a)
	for _, c := range agentio.CapabilityValues() {
		has := slices.Contains(sup, c)
		if reason, declared := agentio.Unsupported(a, c); has && declared {
			t.Errorf("%s: supported but declares unsupported reason %q", c, reason)
		}
	}
	for _, c := range sup {
		if c != agentio.CapProcessClassifier && c != agentio.CapTokenLogin && c != agentio.CapLoginCommander && c != agentio.CapRunEvents && c.Cost() == (agentio.Cost{}) {
			t.Errorf("%s: no cost row", c)
		}
	}
}

// conformReads calls every read accessor on the fixture home; FailExec and
// FailingHTTP in the Host report any process run or network call.
func conformReads(t *testing.T, a agentio.Agent, h agentio.Home) {
	ctx := context.Background()
	var outputs []string
	keep := func(name string, v fmt.Stringer, err error) {
		if err != nil {
			outputs = append(outputs, name+": "+err.Error())
		}
		if v != nil {
			outputs = append(outputs, v.String())
		}
	}
	if x, ok := agentio.InstallerOf(a); ok {
		v, err := x.Install()
		keep("install", dump(v), err)
	}
	if x, ok := agentio.IdentityOf(a); ok {
		v, err := x.Identity(h)
		keep("identity", dump(v), err)
	}
	if x, ok := agentio.CachedUsageOf(a); ok {
		v, _, err := x.CachedUsage(h)
		keep("cached usage", dump(v), err)
	}
	if x, ok := agentio.ConversationsOf(a); ok {
		v, err := x.Conversations(ctx, h, agentio.ConversationQuery{Limit: 5})
		keep("conversations", dump(v), err)
	}
	if x, ok := agentio.HistoryCheckerOf(a); ok {
		_, err := x.HasHistory(h, conformanceDir)
		keep("history", nil, err)
	}
	if x, ok := agentio.PromptHistoryOf(a); ok {
		v, err := x.PromptHistory(h, agentio.PromptQuery{Limit: 5})
		keep("prompts", dump(v), err)
	}
	if x, ok := agentio.LiveSessionsOf(a); ok {
		v, err := x.LiveSessions(h)
		keep("live", dump(v), err)
	}
	if x, ok := agentio.InstructionFilesOf(a); ok {
		v, err := x.InstructionFiles(h, conformanceDir)
		keep("instructions", dump(v), err)
	}
	if x, ok := agentio.MCPServersOf(a); ok {
		v, err := x.MCPServers(h, conformanceDir)
		keep("mcp", dump(v), err)
	}
	if x, ok := agentio.SkillsOf(a); ok {
		v, err := x.Skills(h, conformanceDir)
		keep("skills", dump(v), err)
	}
	if x, ok := agentio.PluginsOf(a); ok {
		v, err := x.Plugins(h, conformanceDir)
		keep("plugins", dump(v), err)
	}
	if x, ok := agentio.ConfigOf(a); ok {
		v, err := x.ConfigLayers(h, conformanceDir)
		keep("config", dump(v), err)
		_, err = x.ProjectTrust(h, conformanceDir)
		keep("trust", nil, err)
		_, err = x.Defaults(h, conformanceDir)
		keep("defaults", nil, err)
	}
	if x, ok := agentio.ModelsOf(a); ok {
		v, err := x.Models(h)
		keep("models", dump(v), err)
	}
	if x, ok := agentio.DoctorOf(a); ok {
		keep("doctor", dump(x.Checks(ctx, h)), nil)
	}
	if _, err := a.Homes(); err != nil {
		keep("homes", nil, err)
	}
	for _, o := range outputs {
		if strings.Contains(o, conformanceSecret) {
			t.Errorf("read output leaks the probe secret: %s", o)
		}
	}
}

// formatted renders a value through every fmt form a caller might log.
type formatted string

// String returns the joined forms.
func (f formatted) String() string { return string(f) }

// dump renders %v, %+v, %#v and JSON of v together. Values are rendered
// through the concrete types the suite passes, so this needs no `any`.
func dump[T agentio.Token | agentio.Install | agentio.Identity | agentio.Usage | []agentio.Conversation | []agentio.PromptEntry | []agentio.LiveSession | []agentio.InstructionFile | []agentio.MCPServer | []agentio.Skill | []agentio.Plugin | []agentio.ConfigLayer | []agentio.Model | []agentio.Check | agentio.Command](v T) fmt.Stringer {
	j, err := json.Marshal(v)
	if err != nil {
		j = []byte(err.Error())
	}
	return formatted(fmt.Sprintf("%v\n%+v\n%#v\n%s", v, v, v, j))
}

// conformToken checks invariant 8 on a token launch.
func conformToken(t *testing.T, a agentio.Agent) {
	tl, ok := agentio.TokenLoginOf(a)
	if !ok {
		return
	}
	kinds := tl.TokenKinds()
	if len(kinds) == 0 {
		t.Fatal("TokenKinds() is empty")
	}
	cred := &agentio.TokenCredential{Kind: kinds[0], Token: agentio.NewToken(conformanceSecret)}
	cmd, err := a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{Credential: cred})
	if err != nil {
		if strings.Contains(err.Error(), conformanceSecret) {
			t.Errorf("launch error leaks the token")
		}
		t.Fatalf("Launch(token): %v", err)
	}
	if slices.ContainsFunc(cmd.Args, func(s string) bool { return strings.Contains(s, conformanceSecret) }) {
		t.Errorf("token in argv %v", cmd.Redacted().Args)
	}
	if !slices.ContainsFunc(cmd.Env, func(s string) bool { return strings.HasSuffix(s, "="+conformanceSecret) }) {
		t.Errorf("token not in Command.Env")
	}
	// Command.Env must hold the token, so the raw Command is checked only
	// through its safe renderings: Redacted, String and JSON.
	j, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}
	if out := dump(cmd.Redacted()).String() + cmd.String() + string(j); strings.Contains(out, conformanceSecret) {
		t.Errorf("formatted command leaks the token")
	}
	if out := dump(cred.Token).String(); strings.Contains(out, conformanceSecret) {
		t.Errorf("formatted token leaks its value")
	}
	changes, err := tl.TokenEnv(*cred)
	if err != nil {
		t.Fatalf("TokenEnv: %v", err)
	}
	for _, c := range changes {
		j, _ := json.Marshal(c)
		if strings.Contains(c.String()+fmt.Sprintf("%v%+v", c, c)+string(j), conformanceSecret) {
			t.Errorf("env change %s leaks the token", c.Name())
		}
	}
}
