package codex

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// codexConfigurer is the codex launch configurer on a fixture home, with
// a target at the fixture's version.
func codexConfigurer(t *testing.T) (agentio.Agent, agentio.LaunchConfigurer, agentio.Target) {
	t.Helper()
	a, h := setup(t, agenttest.WithEnv("PATH=/usr/bin"))
	n := a.Neutral()
	lc, ok := agentio.LaunchConfigOf(n)
	if !ok {
		t.Fatal("codex must have a launch configurer")
	}
	tg := h.Target()
	tg.Version = agentio.MustVersion(fixtureVersion)
	return n, lc, tg
}

// TestCodexLaunchConfiguredEmptyEqualsLaunch: an empty config builds
// exactly what the neutral Launch builds, for every request shape.
func TestCodexLaunchConfiguredEmptyEqualsLaunch(t *testing.T) {
	t.Parallel()
	n, lc, tg := codexConfigurer(t)
	dangerous := tg
	dangerous.AllowDangerous = true
	cases := []struct {
		name string
		t    agentio.Target
		r    agentio.LaunchRequest
	}{
		{"empty", tg, agentio.LaunchRequest{}},
		{"prompt+model+dirs", tg, agentio.LaunchRequest{Prompt: "hi", Model: "o3", AddDirs: []string{"/x"}, Dir: "/w"}},
		{"read-only", tg, agentio.LaunchRequest{Autonomy: agentio.AutonomyReadOnly}},
		{"ask", tg, agentio.LaunchRequest{Autonomy: agentio.AutonomyAsk}},
		{"full", dangerous, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}},
		{"resume", tg, agentio.LaunchRequest{ResumeID: convNew, Prompt: "go"}},
		{"headless", tg, agentio.LaunchRequest{Headless: true, Autonomy: agentio.AutonomyAsk, Prompt: "x"}},
		{"headless resume", tg, agentio.LaunchRequest{Headless: true, ResumeID: convNew}},
		{"raw args", tg, agentio.LaunchRequest{Args: []string{"--search"}, Prompt: "x"}},
		{"unknown version", n.DefaultHome().Target(), agentio.LaunchRequest{Model: "o3"}},
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
		if !slices.Equal(got.Args, want.Args) || !slices.Equal(got.Env, want.Env) || got.Dir != want.Dir || !slices.Equal(got.Warnings, want.Warnings) {
			t.Errorf("%s:\n got %v\nwant %v", c.name, got.Args, want.Args)
		}
	}
	if _, err := lc.LaunchConfigured(tg, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenAPIKey}}, agentio.LaunchConfig{}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("token login: %v", err)
	}
}

// TestCodexLaunchConfiguredOptions: options overlay the mapping; a key
// the request set, an unknown key and a wrong type are refused; a gated
// flag is refused on an older Codex.
func TestCodexLaunchConfiguredOptions(t *testing.T) {
	t.Parallel()
	_, lc, tg := codexConfigurer(t)
	r := agentio.LaunchRequest{Model: "o3", Prompt: "hi", Args: []string{"--raw"}}
	cmd, err := lc.LaunchConfigured(tg, r, agentio.LaunchConfig{Options: agentio.Raw(`{"codex_profile": "deep", "search": true}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cmd.Args, " "); got != "-m o3 -p deep --search hi --raw" {
		t.Errorf("args = %s", got)
	}
	cases := []struct{ name, options, want string }{
		{"model conflict", `{"model": "gpt-5"}`, `option "model" conflicts with the request's model`},
		{"unknown key", `{"profile": "deep"}`, `unknown key "profile"`},
		{"wrong type", `{"search": "yes"}`, "search: expected true or false"},
		{"bad enum", `{"sandbox": "everything"}`, "is not one of"},
	}
	for _, c := range cases {
		_, err := lc.LaunchConfigured(tg, r, agentio.LaunchConfig{Options: agentio.Raw(c.options)})
		if !errors.Is(err, agentio.ErrInvalidOption) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	_, err = lc.LaunchConfigured(tg, agentio.LaunchRequest{Autonomy: agentio.AutonomyAsk}, agentio.LaunchConfig{Options: agentio.Raw(`{"sandbox": "read-only"}`)})
	if err == nil || !strings.Contains(err.Error(), "conflicts with the request's autonomy level") {
		t.Errorf("autonomy conflict: %v", err)
	}
	if _, err := lc.LaunchConfigured(tg, agentio.LaunchRequest{}, agentio.LaunchConfig{Options: agentio.Raw(`{"bypass_all": true}`)}); !errors.Is(err, agentio.ErrDangerousNotAllowed) {
		t.Errorf("bypass without AllowDangerous: %v", err)
	}
	old := tg
	old.Version = agentio.MustVersion("0.150.0")
	_, err = lc.LaunchConfigured(old, agentio.LaunchRequest{}, agentio.LaunchConfig{Options: agentio.Raw(`{"no_daemon": true}`)})
	var fe *FeatureUnavailableError
	if !errors.As(err, &fe) || fe.Feature != FeatureFlagNoDaemon {
		t.Errorf("version gate: %v", err)
	}
}

// TestCodexLaunchConfiguredSettings: settings leaves become quoted -c
// overrides before every other flag; unknown, raw and unlisted values
// warn; a type error is refused; values with no -c form are refused.
func TestCodexLaunchConfiguredSettings(t *testing.T) {
	t.Parallel()
	_, lc, tg := codexConfigurer(t)
	settings := agentio.Raw(`{"model_reasoning_effort": "high", "sandbox_workspace_write": {"network_access": true}, "notify": ["say", "a \"b\""], "model_context_window": 200000, "brand_new": 1.5, "tui": {"theme": "dark"}, "file_opener": "emacs"}`)
	cmd, err := lc.LaunchConfigured(tg, agentio.LaunchRequest{Model: "o3", Args: []string{"--raw"}}, agentio.LaunchConfig{Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-c", "brand_new=1.5",
		"-c", "file_opener=\"emacs\"",
		"-c", "model_context_window=200000",
		"-c", "model_reasoning_effort=\"high\"",
		"-c", "notify=[\"say\", \"a \\\"b\\\"\"]",
		"-c", "sandbox_workspace_write.network_access=true",
		"-c", "tui.theme=\"dark\"",
		"-m", "o3", "--raw",
	}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("args =\n%q\nwant\n%q", cmd.Args, want)
	}
	keys := []string{}
	for _, w := range cmd.Warnings {
		if w.Kind == agentio.WarnUnknownKey {
			keys = append(keys, w.Key)
		}
	}
	if !slices.Equal(keys, []string{"brand_new", "file_opener", "tui.theme"}) {
		t.Errorf("warnings = %v", cmd.Warnings)
	}
	refused := []struct{ name, settings, want string }{
		{"type error", `{"model_context_window": "big"}`, "model_context_window"},
		{"null", `{"model": null}`, "model: null"},
		{"mixed array", `{"notify": ["a", 1]}`, "array of non-strings"},
		{"dotted key", `{"a.b": 1}`, `"a.b"`},
		{"not an object", `[1]`, "JSON object"},
	}
	for _, c := range refused {
		_, err := lc.LaunchConfigured(tg, agentio.LaunchRequest{}, agentio.LaunchConfig{Settings: agentio.Raw(c.settings)})
		if !errors.Is(err, agentio.ErrInvalidOption) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if lc.RawSettingsArg([]string{"-c", "model=x"}) {
		t.Error("codex -c adds overrides; it does not replace settings")
	}
}

// TestCodexCheckSettings: findings, suggestions and concerns.
func TestCodexCheckSettings(t *testing.T) {
	t.Parallel()
	_, lc, _ := codexConfigurer(t)
	c, err := lc.CheckSettings(agentio.Raw(`{"model": "o3", "sandbox_mode": "read-only", "approval_policy": "never", "model_reasoning_efort": "high", "hide_agent_reasoning": "yes"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.TypeErrors) != 1 || c.TypeErrors[0].Key != "hide_agent_reasoning" {
		t.Errorf("type errors = %+v", c.TypeErrors)
	}
	if len(c.Unknown) != 1 || c.Unknown[0].Suggestion != "model_reasoning_effort" {
		t.Errorf("unknown = %+v", c.Unknown)
	}
	if c.Concerns["model"] != agentio.ConcernModel || c.Concerns["sandbox_mode"] != agentio.ConcernAutonomy || c.Concerns["approval_policy"] != agentio.ConcernAutonomy || len(c.Concerns) != 3 {
		t.Errorf("concerns = %v", c.Concerns)
	}
	keys, err := lc.OptionKeys()
	if err != nil {
		t.Fatal(err)
	}
	for key, concern := range map[string]agentio.Concern{"model": agentio.ConcernModel, "sandbox": agentio.ConcernAutonomy, "bypass_all": agentio.ConcernAutonomy, "search": agentio.ConcernNone} {
		if k, err := agentio.FindOptionKey(keys, key); err != nil || k.Concern != concern {
			t.Errorf("%s concern = %s", key, k.Concern)
		}
	}
}

// TestOverrideFloat: TOML floats keep a decimal point or exponent and
// spell the special values TOML's way.
func TestOverrideFloat(t *testing.T) {
	t.Parallel()
	for v, want := range map[float64]string{1.5: "1.5", 2: "2.0", 1e21: "1e+21", math.Inf(1): "inf", math.Inf(-1): "-inf"} {
		if got := string(OverrideFloat("k", v).Value); got != want {
			t.Errorf("%v = %s, want %s", v, got, want)
		}
	}
	if got := string(OverrideFloat("k", math.NaN()).Value); got != "nan" {
		t.Errorf("NaN = %s", got)
	}
}
