package agentio_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// geminiSpecJSON is the agx v2 spec §3.3 block for Gemini CLI, decoded the
// way agx decodes an agents.<id>.spec block.
const geminiSpecJSON = `{
  "id": "gem", "name": "Gemini CLI", "binary": "gemini",
  "default_home": ".gemini", "home_env": "GEMINI_CLI_HOME", "home_env_names_parent": true,
  "home_markers": ["settings.json"],
  "new_args": ["{args}"], "resume_args": ["--resume", "{id}", "{args}"],
  "model_args": ["--model", "{model}"], "prompt_args": ["--prompt", "{prompt}"],
  "credential_env": ["GOOGLE_CLOUD_ACCESS_TOKEN"],
  "autonomy_args": {"read-only": ["--approval-mode", "plan"], "full": ["--approval-mode", "yolo"]},
  "headless_args": ["--output-format", "stream-json"],
  "token_env": [{"kind": "api-key", "env": "GEMINI_API_KEY"}],
  "process": {"basenames": ["gemini"], "argv_contains": ["@google/gemini-cli"]}
}`

// piSpecJSON is the pi block: provider-keyed token variables, only "full".
const piSpecJSON = `{
  "id": "pix", "name": "pi", "binary": "pi", "default_home": ".pi/agent", "home_env": "PI_CODING_AGENT_DIR",
  "new_args": ["{args}"], "resume_args": ["--session", "{id}", "{args}"],
  "token_env": [
    {"kind": "api-key", "provider": "anthropic", "env": "ANTHROPIC_API_KEY"},
    {"kind": "api-key", "provider": "openai", "env": "OPENAI_API_KEY"}
  ],
  "autonomy_args": {"full": []}
}`

func decodeSpec(t *testing.T, js string) agentio.Spec {
	t.Helper()
	var s agentio.Spec
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestSpecJSONRoundTrip: a Spec survives encode/decode with enum keys and
// kinds spelled by name.
func TestSpecJSONRoundTrip(t *testing.T) {
	t.Parallel()
	s := decodeSpec(t, geminiSpecJSON)
	if !s.HomeEnvNamesParent || len(s.AutonomyArgs[agentio.AutonomyFull]) != 2 || s.TokenEnv[0].Kind != agentio.TokenAPIKey {
		t.Fatalf("decoded %+v", s)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back agentio.Spec
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.AutonomyArgs[agentio.AutonomyReadOnly][1] != "plan" || back.Process.ArgvContains[0] != "@google/gemini-cli" {
		t.Fatalf("round trip %s", b)
	}
}

// TestSpecLaunchTemplates: autonomy, headless and token templates are used
// when set and refused when absent (G4, G5).
func TestSpecLaunchTemplates(t *testing.T) {
	t.Parallel()
	host := agenttest.NewHost(t, agenttest.WithEnv("GOOGLE_CLOUD_ACCESS_TOKEN=x", "GEMINI_API_KEY=user-key"))
	a, err := agentio.FromSpec(decodeSpec(t, geminiSpecJSON), host)
	if err != nil {
		t.Fatal(err)
	}
	def := a.DefaultHome().Target()
	cmd, err := a.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyReadOnly, Headless: true, Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--approval-mode", "plan", "--output-format", "stream-json", "--prompt", "hi"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("args = %v, want %v", cmd.Args, want)
	}
	// Home login: the override variable is scrubbed, the user's own key kept (G12).
	if len(envValsExt(cmd.Env, "GOOGLE_CLOUD_ACCESS_TOKEN")) != 0 || len(envValsExt(cmd.Env, "GEMINI_API_KEY")) != 1 {
		t.Errorf("home login env = %v", cmd.Env)
	}
	if _, err := a.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyAsk}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("ask without a template = %v", err)
	}
	full := def
	full.AllowDangerous = true
	if _, err := a.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}); !errors.Is(err, agentio.ErrDangerousNotAllowed) {
		t.Errorf("full without AllowDangerous = %v", err)
	}
	cmd, err = a.Launch(full, agentio.LaunchRequest{Autonomy: agentio.AutonomyFull, Credential: &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken("k2")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := envValsExt(cmd.Env, "GEMINI_API_KEY"); !slices.Equal(got, []string{"k2"}) {
		t.Errorf("token env = %v", got)
	}
	if _, err := a.Launch(def, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("o")}}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("oauth token without an entry = %v", err)
	}
}

// TestSpecProviderTokens: pi's variable depends on the provider; a request
// without one is ambiguous; the other provider's key is never scrubbed.
func TestSpecProviderTokens(t *testing.T) {
	t.Parallel()
	host := agenttest.NewHost(t, agenttest.WithEnv("OPENAI_API_KEY=user-openai"))
	a, err := agentio.FromSpec(decodeSpec(t, piSpecJSON), host)
	if err != nil {
		t.Fatal(err)
	}
	def := a.DefaultHome().Target()
	cmd, err := a.Launch(def, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Provider: "anthropic", Token: agentio.NewToken("a1")}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(envValsExt(cmd.Env, "ANTHROPIC_API_KEY"), []string{"a1"}) || !slices.Equal(envValsExt(cmd.Env, "OPENAI_API_KEY"), []string{"user-openai"}) {
		t.Errorf("env = %v", cmd.Env)
	}
	if _, err := a.Launch(def, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken("a1")}}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("no provider among two = %v", err)
	}
	if _, err := a.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyAcceptEdits}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("accept-edits on pi = %v", err)
	}
}

// TestSpecHomeEnvNamesParent: GEMINI_CLI_HOME names the dir that contains
// .gemini, both ways (G6).
func TestSpecHomeEnvNamesParent(t *testing.T) {
	t.Parallel()
	host := agenttest.NewHost(t)
	parent := filepath.Join(host.UserHome, "gemini-work")
	agenttest.WriteFile(t, parent, ".gemini/settings.json", "{}")
	host.Env = append(host.Env, "GEMINI_CLI_HOME="+parent)
	a, err := agentio.FromSpec(decodeSpec(t, geminiSpecJSON), host)
	if err != nil {
		t.Fatal(err)
	}
	h, ok := a.HomeFromEnv()
	if !ok || h.Dir() != filepath.Join(parent, ".gemini") || h.Name() != "gemini-work" {
		t.Fatalf("home from env = %v %v", h, ok)
	}
	cmd, err := a.Launch(h.Target(), agentio.LaunchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := envValsExt(cmd.Env, "GEMINI_CLI_HOME"); !slices.Equal(got, []string{parent}) {
		t.Errorf("GEMINI_CLI_HOME = %v, want the parent %s", got, parent)
	}
	cmd, err = a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{})
	if err != nil || len(envValsExt(cmd.Env, "GEMINI_CLI_HOME")) != 0 {
		t.Errorf("default home env = %v %v", cmd.Env, err)
	}
	odd := agentio.NewHome("gem", filepath.Join(host.UserHome, "elsewhere"), false, "elsewhere")
	if _, err := a.Launch(odd.Target(), agentio.LaunchRequest{}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("a home not named .gemini = %v", err)
	}
}
