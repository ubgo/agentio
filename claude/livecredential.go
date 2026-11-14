package claude

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ubgo/agentio"
)

// Live credential: Claude Code re-runs the settings apiKeyHelper command
// to get its key, on start, every CLAUDE_CODE_API_KEY_HELPER_TTL_MS and on
// a 401 (Claude Code docs, "apiKeyHelper"; behaviour tested by hand on
// 2026-11-13). A launcher that owns the command can switch a running
// session's login by changing what the command prints.
const (
	// APIKeyHelperTTLEnv is the helper's refresh interval in milliseconds.
	APIKeyHelperTTLEnv = "CLAUDE_CODE_API_KEY_HELPER_TTL_MS"
	// keyAPIKeyHelper is the settings key holding the helper command.
	keyAPIKeyHelper = "apiKeyHelper"
	// DefaultHelperRefresh is the interval the plan uses when the caller
	// gives none. Claude's own default is five minutes; a switchable
	// session wants the new login within seconds, as tested by hand.
	DefaultHelperRefresh = 10 * time.Second
	// minHelperRefresh keeps a typo (1 ms) from making Claude run the
	// helper in a loop.
	minHelperRefresh = time.Second
)

// LiveCredential plans a launch whose key comes from command through
// apiKeyHelper: the settings key to merge into the launch's settings, the
// refresh interval as process env, and the variables to scrub because
// they outrank apiKeyHelper (ANTHROPIC_AUTH_TOKEN, ANTHROPIC_API_KEY; the
// OAuth token variable too, so only one login is in play). Claude runs
// the command itself; nothing here runs it or sees a key. cost: Pure.
func (a *Agent) LiveCredential(command string, refresh time.Duration) (agentio.LiveCredentialPlan, error) {
	if command == "" {
		return agentio.LiveCredentialPlan{}, fmt.Errorf("claude: live credential needs a helper command")
	}
	if refresh == 0 {
		refresh = DefaultHelperRefresh
	}
	if refresh < minHelperRefresh {
		return agentio.LiveCredentialPlan{}, fmt.Errorf("claude: helper refresh %s is below %s", refresh, minHelperRefresh)
	}
	settings, err := helperSettingsJSON(command)
	if err != nil {
		return agentio.LiveCredentialPlan{}, err
	}
	return agentio.LiveCredentialPlan{
		Settings: settings,
		Env:      []agentio.EnvChange{agentio.EnvSet(APIKeyHelperTTLEnv, fmt.Sprint(refresh.Milliseconds()))},
		Scrub:    credentialEnv(),
		Refresh:  refresh,
	}, nil
}

// helperSettingsJSON is {"apiKeyHelper": command}. cost: Pure.
func helperSettingsJSON(command string) (agentio.Raw, error) {
	return json.Marshal(map[string]string{keyAPIKeyHelper: command})
}
