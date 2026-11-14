package claude

import (
	"context"
	"errors"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestNilHTTPIsUnsupported guards "no hidden state": every Claude network
// path on a Host without an HTTP client returns ErrUnsupported instead of
// building a private client that would reach the real network.
func TestNilHTTPIsUnsupported(t *testing.T) {
	t.Parallel()
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithHTTP(nil))
	ctx := context.Background()
	calls := map[string]func() error{
		"usage": func() error { _, err := a.Usage(ctx, a.DefaultHome()); return err },
		"subscription": func() error {
			_, err := a.Subscription(ctx, a.DefaultHome())
			return err
		},
		"probe": func() error {
			_, err := a.Prober().TokenUsage(ctx, agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("sk-ant-oat01-x")})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, agentio.ErrUnsupported) {
			t.Errorf("%s with nil Host.HTTP: %v, want ErrUnsupported", name, err)
		}
	}
}

// TestHomeLoginScrubsCredentialEnv guards invariant 9 for a home login: an
// inherited ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN or
// CLAUDE_CODE_OAUTH_TOKEN would silently override the home's stored login,
// so Launch and LoginCommand remove every one of them.
func TestHomeLoginScrubsCredentialEnv(t *testing.T) {
	t.Parallel()
	env := []string{"PATH=/bin", "KEEP=1"}
	for _, name := range credentialEnv() {
		env = append(env, name+"=inherited")
	}
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithEnv(env...))
	cmd, err := a.Launch(a.DefaultHome().Target(), LaunchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	login := a.LoginCommand(a.DefaultHome(), LoginClaudeAI, "")
	for label, c := range map[string]agentio.Command{"launch": cmd, "login": login} {
		for _, name := range []string{APIKeyEnv, AuthTokenEnv, OAuthTokenEnv} {
			if v, ok := agentio.EnvLookup(c.Env, name); ok {
				t.Errorf("%s kept inherited %s=%q", label, name, v)
			}
		}
		if v, _ := agentio.EnvLookup(c.Env, "KEEP"); v != "1" {
			t.Errorf("%s dropped an unrelated variable", label)
		}
	}
}
