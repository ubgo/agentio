package claude

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestLiveCredential: the plan names the helper, the refresh interval and
// the outranking variables; a refresh below a second is refused.
func TestLiveCredential(t *testing.T) {
	t.Parallel()
	a := New(agenttest.NewHost(t))
	lc, ok := agentio.LiveCredentialOf(a.Neutral())
	if !ok {
		t.Fatal("claude must have a live credential")
	}
	p, err := lc.LiveCredential("/usr/local/bin/agx credential", 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(p.Settings) != `{"apiKeyHelper":"/usr/local/bin/agx credential"}` || p.Refresh != DefaultHelperRefresh {
		t.Errorf("plan = %s %s", p.Settings, p.Refresh)
	}
	if len(p.Env) != 1 || p.Env[0].Name() != APIKeyHelperTTLEnv || p.Env[0].String() != APIKeyHelperTTLEnv+"=10000" {
		t.Errorf("env = %v", p.Env)
	}
	if !slices.Equal(p.Scrub, []string{OAuthTokenEnv, APIKeyEnv, AuthTokenEnv}) {
		t.Errorf("scrub = %v", p.Scrub)
	}
	if _, err := lc.LiveCredential("x", time.Millisecond); err == nil {
		t.Error("1ms refresh must be refused")
	}
	if _, err := lc.LiveCredential("", 0); err == nil {
		t.Error("an empty command must be refused")
	}
}

// TestHelperLogin: an api-key-helper login goes into --settings as
// apiKeyHelper and scrubs every variable that outranks it.
func TestHelperLogin(t *testing.T) {
	t.Parallel()
	a := New(agenttest.NewHost(t, agenttest.WithEnv("ANTHROPIC_API_KEY=user", "CLAUDE_CODE_OAUTH_TOKEN=o")))
	cred := &agentio.TokenCredential{Kind: agentio.TokenAPIKeyHelper, Token: agentio.NewToken("~/bin/helper.sh")}
	cmd, err := a.Neutral().Launch(a.DefaultHome().Neutral().Target(), agentio.LaunchRequest{Credential: cred})
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.Index(cmd.Args, flagSettings); i < 0 || cmd.Args[i+1] != `{"apiKeyHelper":"~/bin/helper.sh"}` {
		t.Errorf("args = %v", cmd.Args)
	}
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, APIKeyEnv+"=") || strings.HasPrefix(kv, OAuthTokenEnv+"=") {
			t.Errorf("outranking variable kept: %s", strings.SplitN(kv, "=", 2)[0])
		}
	}
	if _, err := HelperSettings(agentio.TokenCredential{Kind: agentio.TokenAPIKey}); err == nil {
		t.Error("an api key is not a helper login")
	}
	if !slices.Contains(a.TokenKinds(), agentio.TokenAPIKeyHelper) {
		t.Error("helper kind must be listed")
	}
}
