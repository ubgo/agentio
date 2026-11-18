package agentio_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// tokenSpecJSON is a config-defined agent with three token logins: an
// api-key per provider (the variable depends on the provider, like pi) and
// one oauth-token with an explicit scrub list.
const tokenSpecJSON = `{
  "id": "tok", "name": "Token CLI", "binary": "tokcli",
  "default_home": ".tokcli", "home_env": "TOKCLI_HOME",
  "new_args": ["{args}"],
  "credential_env": ["TOKCLI_API_KEY", "TOKCLI_AUTH_TOKEN", "TOKCLI_OAUTH_TOKEN"],
  "token_env": [
    {"kind": "api-key", "provider": "anthropic", "env": "ANTHROPIC_API_KEY"},
    {"kind": "oauth-token", "env": "TOKCLI_OAUTH_TOKEN", "scrub": ["TOKCLI_AUTH_TOKEN"]},
    {"kind": "api-key", "provider": "openai", "env": "OPENAI_API_KEY"}
  ]
}`

// TestSpecTokenLogin_CapabilityFollowsTokenEnv: a config-defined agent has
// the token-login capability exactly when its spec has token_env, and says
// why it is missing otherwise (it was missing for every spec agent while
// Launch already honoured token_env).
func TestSpecTokenLogin_CapabilityFollowsTokenEnv(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		js   string
		want bool
	}{
		"with token_env":            {geminiSpecJSON, true},
		"with provider token_env":   {piSpecJSON, true},
		"with mixed token_env":      {tokenSpecJSON, true},
		"without token_env":         {`{"id": "bare", "binary": "bare", "new_args": ["{args}"]}`, false},
		"with an empty token_env":   {`{"id": "bare", "binary": "bare", "token_env": []}`, false},
		"credential_env, no tokens": {`{"id": "bare", "binary": "bare", "credential_env": ["BARE_KEY"]}`, false},
	} {
		a, err := agentio.FromSpec(decodeSpec(t, tc.js), agenttest.NewHost(t))
		if err != nil {
			t.Fatal(err)
		}
		_, got := agentio.TokenLoginOf(a)
		if got != tc.want || slices.Contains(agentio.Supported(a), agentio.CapTokenLogin) != tc.want {
			t.Errorf("%s: TokenLoginOf = %v, Supported = %v, want %v", name, got, agentio.Supported(a), tc.want)
		}
		reason, declared := agentio.Unsupported(a, agentio.CapTokenLogin)
		if tc.want == declared || (!tc.want && reason != "spec has no token_env") {
			t.Errorf("%s: Unsupported = %q, %v", name, reason, declared)
		}
	}
}

// TestSpecTokenLogin_KindsAndDetection: TokenKinds lists exactly the kinds
// token_env names, in spec order, each once (the first is the default a
// caller offers); a spec describes no token shapes, so detection never
// guesses.
func TestSpecTokenLogin_KindsAndDetection(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		js   string
		want []agentio.TokenKind
	}{
		"one kind":         {geminiSpecJSON, []agentio.TokenKind{agentio.TokenAPIKey}},
		"one kind, twice":  {piSpecJSON, []agentio.TokenKind{agentio.TokenAPIKey}},
		"two kinds, mixed": {tokenSpecJSON, []agentio.TokenKind{agentio.TokenAPIKey, agentio.TokenOAuthToken}},
	} {
		tl := specTokenLogin(t, tc.js)
		if got := tl.TokenKinds(); !slices.Equal(got, tc.want) {
			t.Errorf("%s: TokenKinds = %v, want %v", name, got, tc.want)
		}
		if k, ok := tl.DetectTokenKind(agentio.NewToken("key-shaped-value")); ok || k != agentio.TokenKindUnknown {
			t.Errorf("%s: DetectTokenKind = %v, %v; a spec has no shapes to detect", name, k, ok)
		}
		if cmd, ok := tl.TokenSetupCommand(agentio.NewHome("tok", "/h", true, "default")); ok || cmd.Path != "" {
			t.Errorf("%s: TokenSetupCommand = %v, %v; a spec has no setup command", name, cmd, ok)
		}
	}
}

// TestSpecTokenLogin_EnvPerKindAndProvider: TokenEnv and Launch put the
// token in the variable token_env names for (kind, provider), refuse a
// login token_env does not describe with ErrUnsupported, and agree.
func TestSpecTokenLogin_EnvPerKindAndProvider(t *testing.T) {
	t.Parallel()
	a, err := agentio.FromSpec(decodeSpec(t, tokenSpecJSON), agenttest.NewHost(t))
	if err != nil {
		t.Fatal(err)
	}
	tl, ok := agentio.TokenLoginOf(a)
	if !ok {
		t.Fatal("token login missing")
	}
	for name, tc := range map[string]struct {
		kind     agentio.TokenKind
		provider string
		want     string // the variable; "" = refused
	}{
		"api-key anthropic":        {agentio.TokenAPIKey, "anthropic", "ANTHROPIC_API_KEY"},
		"api-key openai":           {agentio.TokenAPIKey, "openai", "OPENAI_API_KEY"},
		"api-key, no provider":     {agentio.TokenAPIKey, "", ""},
		"api-key, unknown":         {agentio.TokenAPIKey, "mistral", ""},
		"oauth-token":              {agentio.TokenOAuthToken, "", "TOKCLI_OAUTH_TOKEN"},
		"oauth-token, a provider":  {agentio.TokenOAuthToken, "anthropic", ""},
		"api-key-helper, no entry": {agentio.TokenAPIKeyHelper, "", ""},
	} {
		cred := agentio.TokenCredential{Kind: tc.kind, Provider: tc.provider, Token: agentio.NewToken("v-" + name)}
		changes, envErr := tl.TokenEnv(cred)
		cmd, launchErr := a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{Credential: &cred})
		if tc.want == "" {
			if !errors.Is(envErr, agentio.ErrUnsupported) || !errors.Is(launchErr, agentio.ErrUnsupported) || envErr.Error() != launchErr.Error() {
				t.Errorf("%s: TokenEnv err = %v, Launch err = %v, want the same ErrUnsupported", name, envErr, launchErr)
			}
			continue
		}
		if envErr != nil || launchErr != nil {
			t.Fatalf("%s: TokenEnv err = %v, Launch err = %v", name, envErr, launchErr)
		}
		if len(changes) != 1 || changes[0].Name() != tc.want || changes[0].Op() != agentio.EnvSetSecretOp || !slices.Contains(cmd.SecretEnvNames(), tc.want) {
			t.Errorf("%s: TokenEnv = %v, secret names %v, want %s set as a secret", name, changes, cmd.SecretEnvNames(), tc.want)
		}
		if got := envValsExt(cmd.Env, tc.want); !slices.Equal(got, []string{"v-" + name}) {
			t.Errorf("%s: %s = %v", name, tc.want, got)
		}
	}
}

// TestSpecTokenLogin_ScrubRules: a token login removes only the inherited
// variables that would override it (token_env scrub, or credential_env
// minus its own variable), a home login removes all of credential_env, and
// a variable outside credential_env (another provider's key) survives.
func TestSpecTokenLogin_ScrubRules(t *testing.T) {
	t.Parallel()
	inherited := []string{"TOKCLI_API_KEY=i1", "TOKCLI_AUTH_TOKEN=i2", "TOKCLI_OAUTH_TOKEN=i3", "OPENAI_API_KEY=i4", "ANTHROPIC_API_KEY=i5"}
	a, err := agentio.FromSpec(decodeSpec(t, tokenSpecJSON), agenttest.NewHost(t, agenttest.WithEnv(inherited...)))
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		cred *agentio.TokenCredential
		kept []string // inherited variables still present with their value
	}{
		"home login": {nil, []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"}},
		// scrub nil: credential_env minus ANTHROPIC_API_KEY (not in it) = all three.
		"api-key anthropic": {&agentio.TokenCredential{Kind: agentio.TokenAPIKey, Provider: "anthropic", Token: agentio.NewToken("t")}, []string{"OPENAI_API_KEY"}},
		// scrub ["TOKCLI_AUTH_TOKEN"]: only that one goes; its own variable is replaced.
		"oauth-token": {&agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("t")}, []string{"TOKCLI_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"}},
	} {
		cmd, err := a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{Credential: tc.cred})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, kv := range inherited {
			k, v, _ := strings.Cut(kv, "=")
			got := envValsExt(cmd.Env, k)
			switch {
			case slices.Contains(tc.kept, k):
				if !slices.Equal(got, []string{v}) {
					t.Errorf("%s: %s = %v, want the inherited %s kept", name, k, got, v)
				}
			case tc.cred != nil && slices.Equal(got, []string{"t"}):
				// the token's own variable
			case len(got) != 0:
				t.Errorf("%s: %s = %v, want it scrubbed", name, k, got)
			}
		}
	}
}

// TestSpecTokenLogin_Providers: a spec whose token_env names a provider is
// a TokenProviderLister listing, per kind, the providers in spec order (""
// for an entry that names none); a spec naming no provider is not one, so
// a caller asks for no provider.
func TestSpecTokenLogin_Providers(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		js     string
		lister bool
		want   map[agentio.TokenKind][]string
	}{
		"no providers": {geminiSpecJSON, false, nil},
		"pi":           {piSpecJSON, true, map[agentio.TokenKind][]string{agentio.TokenAPIKey: {"anthropic", "openai"}, agentio.TokenOAuthToken: nil}},
		"mixed":        {tokenSpecJSON, true, map[agentio.TokenKind][]string{agentio.TokenAPIKey: {"anthropic", "openai"}, agentio.TokenOAuthToken: {""}, agentio.TokenAPIKeyHelper: nil}},
	} {
		tl := specTokenLogin(t, tc.js)
		pl, ok := tl.(agentio.TokenProviderLister)
		if ok != tc.lister {
			t.Errorf("%s: TokenProviderLister = %v, want %v", name, ok, tc.lister)
			continue
		}
		for k, want := range tc.want {
			if got := pl.TokenProviders(k); !slices.Equal(got, want) {
				t.Errorf("%s: TokenProviders(%s) = %q, want %q", name, k, got, want)
			}
		}
	}
}

// TestSpecTokenLogin_Conformance runs the shared suite, token-redaction
// branch included, on spec agents with token_env, with and without
// providers.
func TestSpecTokenLogin_Conformance(t *testing.T) {
	for name, js := range map[string]string{"gemini": geminiSpecJSON, "pi": piSpecJSON, "mixed": tokenSpecJSON} {
		t.Run(name, func(t *testing.T) {
			a, err := agentio.FromSpec(decodeSpec(t, js), agenttest.NewHost(t))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := agentio.TokenLoginOf(a); !ok {
				t.Fatal("token login missing, so the redaction branch would not run")
			}
			agenttest.Conformance(t, a, a.DefaultHome())
		})
	}
}

// specTokenLogin builds a spec agent from js and returns its TokenLogin.
func specTokenLogin(t *testing.T, js string) agentio.TokenLogin {
	t.Helper()
	a, err := agentio.FromSpec(decodeSpec(t, js), agenttest.NewHost(t))
	if err != nil {
		t.Fatal(err)
	}
	tl, ok := agentio.TokenLoginOf(a)
	if !ok {
		t.Fatal("token login missing")
	}
	return tl
}
