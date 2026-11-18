package claude

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// launchConfigurer is the claude agent's launch configurer on a fixture
// host, with a target at a known version.
func launchConfigurer(t *testing.T, version string) (agentio.Agent, agentio.LaunchConfigurer, agentio.Target) {
	t.Helper()
	a := New(agenttest.NewHost(t, agenttest.WithEnv("PATH=/bin", APIKeyEnv+"=inherited")))
	n := a.Neutral()
	lc, ok := agentio.LaunchConfigOf(n)
	if !ok {
		t.Fatal("claude must have a launch configurer")
	}
	tg := a.DefaultHome().Target()
	if version != "" {
		tg.Version = agentio.MustVersion(version)
	}
	return n, lc, tg
}

// testVersion is a Claude Code version every flag below exists in.
const testVersion = "2.1.323"

// TestLaunchConfiguredEmptyEqualsLaunch: an empty config builds exactly
// what the neutral Launch builds, for every request shape.
func TestLaunchConfiguredEmptyEqualsLaunch(t *testing.T) {
	t.Parallel()
	n, lc, tg := launchConfigurer(t, testVersion)
	dangerous := tg
	dangerous.AllowDangerous = true
	cases := []struct {
		name string
		t    agentio.Target
		r    agentio.LaunchRequest
	}{
		{"empty", tg, agentio.LaunchRequest{}},
		{"prompt", tg, agentio.LaunchRequest{Prompt: "-hi", Dir: "/work"}},
		{"model+dirs", tg, agentio.LaunchRequest{Model: "opus", AddDirs: []string{"/a", "/b"}}},
		{"read-only", tg, agentio.LaunchRequest{Autonomy: agentio.AutonomyReadOnly}},
		{"accept-edits", tg, agentio.LaunchRequest{Autonomy: agentio.AutonomyAcceptEdits}},
		{"full", dangerous, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}},
		{"headless resume", tg, agentio.LaunchRequest{Headless: true, ResumeID: "abc", Prompt: "go"}},
		{"raw args", tg, agentio.LaunchRequest{Args: []string{"--effort", "max"}, Prompt: "x"}},
		{"oauth token", tg, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("fake-token-x")}}},
		{"unknown version", n.DefaultHome().Target(), agentio.LaunchRequest{Model: "opus"}},
	}
	for _, c := range cases {
		want, err := n.Launch(c.t, c.r)
		if err != nil {
			t.Fatalf("%s: launch: %v", c.name, err)
		}
		got, err := lc.LaunchConfigured(c.t, c.r, agentio.LaunchConfig{})
		if err != nil {
			t.Fatalf("%s: configured: %v", c.name, err)
		}
		if !slices.Equal(got.Args, want.Args) || !slices.Equal(got.Env, want.Env) || got.Dir != want.Dir || !slices.Equal(got.Warnings, want.Warnings) || got.Path != want.Path {
			t.Errorf("%s:\n got %v %v\nwant %v %v", c.name, got.Args, got.Warnings, want.Args, want.Warnings)
		}
	}
}

// TestLaunchConfiguredOptions: options overlay the request mapping; an
// unknown key, a wrong type and a key the request already set are
// refused; a version-gated flag is refused on an older Claude.
func TestLaunchConfiguredOptions(t *testing.T) {
	t.Parallel()
	_, lc, tg := launchConfigurer(t, testVersion)
	r := agentio.LaunchRequest{Model: "opus", Prompt: "hi", Args: []string{"--raw"}}
	cmd, err := lc.LaunchConfigured(tg, r, agentio.LaunchConfig{Options: agentio.Raw(`{"effort": "high", "mcp_configs": ["/m.json"]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cmd.Args, " "); got != "--model opus --effort high --mcp-config /m.json --raw -- hi" {
		t.Errorf("args = %s", got)
	}
	// extra_args next to raw args: both kept, extra_args first, then the
	// request's raw args, all before the prompt's "--".
	cmd, err = lc.LaunchConfigured(tg, r, agentio.LaunchConfig{Options: agentio.Raw(`{"extra_args": ["--debug", "api"]}`)})
	if err != nil {
		t.Fatalf("extra_args with raw args: %v", err)
	}
	if got := strings.Join(cmd.Args, " "); got != "--model opus --debug api --raw -- hi" {
		t.Errorf("extra_args with raw args: args = %s", got)
	}
	cases := []struct {
		name, options, want string
	}{
		{"unknown key", `{"efort": "high"}`, `unknown key "efort"`},
		{"wrong type", `{"effort": 3}`, "effort: expected a string"},
		{"bad enum", `{"effort": "turbo"}`, "is not one of"},
		{"model conflict", `{"model": "sonnet"}`, `option "model" conflicts with the request's model`},
		{"prompt conflict", `{"prompt": "other"}`, `option "prompt" conflicts with the request's prompt`},
		{"not an object", `["effort"]`, "options must be a JSON object"},
	}
	for _, c := range cases {
		_, err := lc.LaunchConfigured(tg, r, agentio.LaunchConfig{Options: agentio.Raw(c.options)})
		if !errors.Is(err, agentio.ErrInvalidOption) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
		if strings.Contains(err.Error(), "LaunchRequest") {
			t.Errorf("%s: the message names an internal type: %v", c.name, err)
		}
		if c.name != "model conflict" && c.name != "prompt conflict" {
			if err := lc.CheckOptions(agentio.Raw(c.options)); !errors.Is(err, agentio.ErrInvalidOption) {
				t.Errorf("%s: CheckOptions accepted it", c.name)
			}
		}
	}
	_, err = lc.LaunchConfigured(tg, agentio.LaunchRequest{Autonomy: agentio.AutonomyReadOnly}, agentio.LaunchConfig{Options: agentio.Raw(`{"permission_mode": "auto"}`)})
	if err == nil || !strings.Contains(err.Error(), "conflicts with the request's autonomy level") {
		t.Errorf("autonomy conflict: %v", err)
	}
	old := tg
	old.Version = agentio.MustVersion("1.0.100")
	_, err = lc.LaunchConfigured(old, agentio.LaunchRequest{}, agentio.LaunchConfig{Options: agentio.Raw(`{"agents": {"x": {"description": "d", "prompt": "p"}}}`)})
	var fe *FeatureUnavailableError
	if !errors.As(err, &fe) || fe.Feature != FeatureFlagAgents {
		t.Errorf("version gate: %v", err)
	}
	if err := lc.CheckOptions(nil); err != nil {
		t.Errorf("empty options: %v", err)
	}
}

// TestLaunchConfiguredSettings: settings go to a 0600 file or inline;
// unknown keys warn with a suggestion; type errors and a settings option
// competing with them are refused, and a refused launch writes no file.
func TestLaunchConfiguredSettings(t *testing.T) {
	t.Parallel()
	_, lc, tg := launchConfigurer(t, testVersion)
	runDir := filepath.Join(t.TempDir(), "run")
	settings := agentio.Raw(`{"model": "opus", "cleanupPeriodDays": 30, "cleanupPeriodDay": 7}`)
	cmd, err := lc.LaunchConfigured(tg, agentio.LaunchRequest{Args: []string{"--raw"}}, agentio.LaunchConfig{Settings: settings, RunDir: runDir, RunID: "run1"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir, "settings-run1.json")
	if !slices.Equal(cmd.Args, []string{flagSettings, path, "--raw"}) {
		t.Errorf("args = %v", cmd.Args)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no unix mode bits to check.
	if runtime.GOOS != agentio.OSWindows && fi.Mode().Perm() != launchSettingsFileMode {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
	if body, _ := os.ReadFile(path); string(body) != string(settings) {
		t.Errorf("file = %s", body)
	}
	if len(cmd.Warnings) != 1 || cmd.Warnings[0].Kind != agentio.WarnUnknownKey || cmd.Warnings[0].Key != "cleanupPeriodDay" || !strings.Contains(cmd.Warnings[0].Detail, "did you mean cleanupPeriodDays?") {
		t.Errorf("warnings = %v", cmd.Warnings)
	}
	inline, err := lc.LaunchConfigured(tg, agentio.LaunchRequest{}, agentio.LaunchConfig{Settings: settings, Delivery: agentio.SettingsInline})
	if err != nil || !slices.Equal(inline.Args, []string{flagSettings, string(settings)}) {
		t.Errorf("inline: %v %v", err, inline.Args)
	}
	refused := filepath.Join(t.TempDir(), "refused")
	cases := []struct {
		name string
		r    agentio.LaunchRequest
		c    agentio.LaunchConfig
		want string
	}{
		{"type error", agentio.LaunchRequest{}, agentio.LaunchConfig{Settings: agentio.Raw(`{"cleanupPeriodDays": "thirty"}`), RunDir: refused, RunID: "r"}, "cleanupPeriodDays"},
		{"settings option", agentio.LaunchRequest{}, agentio.LaunchConfig{Options: agentio.Raw(`{"settings": "/s.json"}`), Settings: settings, RunDir: refused, RunID: "r"}, `option "settings" conflicts with LaunchConfig.Settings`},
		{"no run dir", agentio.LaunchRequest{}, agentio.LaunchConfig{Settings: settings}, "RunDir"},
		{"not an object", agentio.LaunchRequest{}, agentio.LaunchConfig{Settings: agentio.Raw(`[1]`), RunDir: refused, RunID: "r"}, ""},
	}
	for _, c := range cases {
		_, err := lc.LaunchConfigured(tg, c.r, c.c)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	old := tg
	old.Version = agentio.MustVersion("1.0.50")
	if _, err := lc.LaunchConfigured(old, agentio.LaunchRequest{}, agentio.LaunchConfig{Settings: settings, RunDir: refused, RunID: "r"}); !errors.Is(err, agentio.ErrFeatureUnavailable) {
		t.Errorf("--settings on 1.0.50: %v", err)
	}
	if _, err := os.Stat(refused); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused launch wrote %s", refused)
	}
}

// TestLaunchConfiguredHelperLogin: an api-key-helper login merges into
// the settings object and is delivered with it (a file by default), and
// a settings apiKeyHelper competing with it is refused.
func TestLaunchConfiguredHelperLogin(t *testing.T) {
	t.Parallel()
	_, lc, tg := launchConfigurer(t, testVersion)
	cred := &agentio.TokenCredential{Kind: agentio.TokenAPIKeyHelper, Token: agentio.NewToken("~/bin/helper.sh")}
	runDir := t.TempDir()
	cmd, err := lc.LaunchConfigured(tg, agentio.LaunchRequest{Credential: cred}, agentio.LaunchConfig{Settings: agentio.Raw(`{"model": "opus"}`), RunDir: runDir, RunID: "h1"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir, "settings-h1.json")
	if !slices.Equal(cmd.Args, []string{flagSettings, path}) {
		t.Errorf("args = %v", cmd.Args)
	}
	body, _ := os.ReadFile(path)
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil || got[keyAPIKeyHelper] != "~/bin/helper.sh" || got["model"] != "opus" {
		t.Errorf("file = %s", body)
	}
	if _, ok := agentio.EnvLookup(cmd.Env, APIKeyEnv); ok {
		t.Error("an outranking variable was kept")
	}
	_, err = lc.LaunchConfigured(tg, agentio.LaunchRequest{Credential: cred}, agentio.LaunchConfig{Settings: agentio.Raw(`{"apiKeyHelper": "other"}`), Delivery: agentio.SettingsInline})
	if !errors.Is(err, agentio.ErrInvalidOption) || !strings.Contains(err.Error(), "apiKeyHelper") {
		t.Errorf("competing helper: %v", err)
	}
	inline, err := lc.LaunchConfigured(tg, agentio.LaunchRequest{Credential: cred}, agentio.LaunchConfig{Delivery: agentio.SettingsInline})
	if err != nil || !slices.Equal(inline.Args, []string{flagSettings, `{"apiKeyHelper":"~/bin/helper.sh"}`}) {
		t.Errorf("helper inline: %v %v", err, inline.Args)
	}
}

// TestCheckSettingsAndConcerns: CheckSettings reports type errors,
// unknown keys with suggestions and the keys that overlap a neutral
// concern; the option registry carries each key's concern.
func TestCheckSettingsAndConcerns(t *testing.T) {
	t.Parallel()
	_, lc, _ := launchConfigurer(t, "")
	c, err := lc.CheckSettings(agentio.Raw(`{"model": "opus", "apiKeyHelper": "h", "cleanupPeriodDays": "x", "permissions": {"defaultMode": "plan", "alow": []}, "env": {"ANTHROPIC_API_KEY": "k", "DEBUG": "1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.OK() || len(c.TypeErrors) != 1 || c.TypeErrors[0].Key != "cleanupPeriodDays" || strings.Contains(c.TypeErrors[0].Detail, `"x"`) {
		t.Errorf("type errors = %+v", c.TypeErrors)
	}
	if len(c.Unknown) != 1 || c.Unknown[0].Key != "permissions.alow" || c.Unknown[0].Suggestion != "permissions.allow" {
		t.Errorf("unknown = %+v", c.Unknown)
	}
	want := map[string]agentio.Concern{"model": agentio.ConcernModel, "apiKeyHelper": agentio.ConcernCredential, "permissions.defaultMode": agentio.ConcernAutonomy, "env.ANTHROPIC_API_KEY": agentio.ConcernCredential}
	if len(c.Concerns) != len(want) {
		t.Errorf("concerns = %v", c.Concerns)
	}
	for k, v := range want {
		if c.Concerns[k] != v {
			t.Errorf("concern %s = %s, want %s", k, c.Concerns[k], v)
		}
	}
	if _, err := lc.CheckSettings(agentio.Raw(`"x"`)); err == nil {
		t.Error("a non-object settings body was accepted")
	}
	keys, err := lc.OptionKeys()
	if err != nil {
		t.Fatal(err)
	}
	for key, concern := range map[string]agentio.Concern{"model": agentio.ConcernModel, "permission_mode": agentio.ConcernAutonomy, "settings": agentio.ConcernSettings, "effort": agentio.ConcernNone, "allow_dangerously_skip_permissions": agentio.ConcernNone} {
		k, err := agentio.FindOptionKey(keys, key)
		if err != nil || k.Concern != concern {
			t.Errorf("%s concern = %s (%v), want %s", key, k.Concern, err, concern)
		}
	}
	if s, err := lc.OptionsSchema(); err != nil || s.Properties["effort"].Enum == nil {
		t.Errorf("schema: %v", err)
	}
	if !lc.RawSettingsArg([]string{"--x", "--settings", "{}"}) || !lc.RawSettingsArg([]string{"--settings=/f"}) || lc.RawSettingsArg([]string{"--setting-sources", "user"}) {
		t.Error("raw --settings detection")
	}
}
