package claude

import (
	"sort"

	"github.com/ubgo/agentio"
)

// EnvGroup groups the variables Claude Code reads.
type EnvGroup string

// Env groups (CLAUDE_API §15).
const (
	EnvGroupHome        EnvGroup = "home"
	EnvGroupCredentials EnvGroup = "credentials"
	EnvGroupProvider    EnvGroup = "provider"
	EnvGroupModel       EnvGroup = "model"
	EnvGroupMarker      EnvGroup = "marker"
	EnvGroupPrivacy     EnvGroup = "privacy"
	EnvGroupFeature     EnvGroup = "feature"
)

// EnvVar is one variable Claude Code reads.
type EnvVar struct {
	Name       string
	Group      EnvGroup
	Secret     bool
	Deprecated bool
}

// envRow builds a table row.
func envRow(g EnvGroup, secret bool, names ...string) []EnvVar {
	out := make([]EnvVar, len(names))
	for i, n := range names {
		out[i] = EnvVar{Name: n, Group: g, Secret: secret}
	}
	return out
}

// KnownEnv lists the variables the library knows (the subset of the ~250
// documented ones, hand-picked because a table generated from the docs
// pages does not exist yet). cost: Pure.
func KnownEnv() []EnvVar {
	var out []EnvVar
	out = append(out, envRow(EnvGroupHome, false, ConfigDirEnv)...)
	out = append(out, envRow(EnvGroupCredentials, true, OAuthTokenEnv, APIKeyEnv, AuthTokenEnv, "ANTHROPIC_FOUNDRY_API_KEY",
		"ANTHROPIC_FOUNDRY_AUTH_TOKEN", "AWS_BEARER_TOKEN_BEDROCK", "ANTHROPIC_AWS_API_KEY")...)
	out = append(out, envRow(EnvGroupCredentials, false, "ANTHROPIC_PROFILE")...)
	out = append(out, envRow(EnvGroupProvider, false, UseBedrockEnv, UseVertexEnv, UseFoundryEnv, BaseURLEnv,
		"ANTHROPIC_BEDROCK_BASE_URL", "ANTHROPIC_VERTEX_BASE_URL", "ANTHROPIC_VERTEX_PROJECT_ID", "ANTHROPIC_FOUNDRY_BASE_URL",
		"ANTHROPIC_FOUNDRY_RESOURCE")...)
	out = append(out, envRow(EnvGroupModel, false, "ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_FABLE_MODEL",
		"CLAUDE_CODE_SUBAGENT_MODEL", "CLAUDE_CODE_EFFORT_LEVEL", "MAX_THINKING_TOKENS")...)
	out = append(out, EnvVar{Name: "ANTHROPIC_SMALL_FAST_MODEL", Group: EnvGroupModel, Deprecated: true})
	out = append(out, envRow(EnvGroupMarker, false, "CLAUDECODE", "CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_BRIDGE_SESSION_ID",
		"CLAUDE_CODE_MESSAGING_SOCKET", "CLAUDE_CODE_REMOTE_SESSION_ID", "CLAUDE_CODE_SIMPLE", "CLAUDE_CODE_SAFE_MODE")...)
	out = append(out, envRow(EnvGroupMarker, true, "CLAUDE_CODE_MESSAGING_TOKEN")...)
	out = append(out, envRow(EnvGroupPrivacy, false, "CLAUDE_CODE_SKIP_PROMPT_HISTORY", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC",
		"DISABLE_TELEMETRY", "DISABLE_ERROR_REPORTING", "DO_NOT_TRACK", "CLAUDE_CODE_ENABLE_TELEMETRY",
		"CLAUDE_CODE_DEBUG_LOGS_DIR", "CLAUDE_CODE_TMPDIR")...)
	out = append(out, envRow(EnvGroupFeature, false, "CLAUDE_CODE_DISABLE_AUTO_MEMORY", "CLAUDE_CODE_DISABLE_CLAUDE_MDS",
		"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "CLAUDE_CODE_DISABLE_BACKGROUND_TASKS", "CLAUDE_CODE_DISABLE_AGENT_VIEW",
		"CLAUDE_CODE_DISABLE_FILE_CHECKPOINTING", "CLAUDE_CODE_MAX_OUTPUT_TOKENS", "API_TIMEOUT_MS", "MCP_TIMEOUT",
		"MCP_TOOL_TIMEOUT", "BASH_DEFAULT_TIMEOUT_MS", "BASH_MAX_TIMEOUT_MS", "CLAUDE_CODE_DISABLE_DEFERRED_TOOL_RESUME",
		"CLAUDE_CODE_FULL_SIGN_IN_LINK")...)
	return out
}

// EnvFromProcess marks an EnvSetting that comes from Host.Env rather than a
// settings layer.
const EnvFromProcess SettingsScope = "process"

// EnvSetting is one variable in effect and where it came from. Value is an
// agentio.EnvValue: it prints redacted.
type EnvSetting struct {
	Name   string
	Value  agentio.EnvValue
	From   SettingsScope
	Secret bool
}

// EnvInEffect lists known variables set in Host.Env plus every settings
// `env` entry of each layer (highest first), so a caller sees both and
// which layer set it. cost: Disk · Read.
func (a *Agent) EnvInEffect(h Home, p *Project) ([]EnvSetting, error) {
	known := KnownEnv()
	secret := map[string]bool{}
	var out []EnvSetting
	for _, k := range known {
		secret[k.Name] = k.Secret
		if v, ok := a.host.Getenv(k.Name); ok {
			out = append(out, EnvSetting{Name: k.Name, Value: agentio.NewEnvValue(v), From: EnvFromProcess, Secret: k.Secret})
		}
	}
	eff, err := a.Settings(h, p, nil)
	if err != nil {
		return out, err
	}
	for _, l := range eff.Layers {
		names := make([]string, 0, len(l.Settings.Env))
		for n := range l.Settings.Env {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			out = append(out, EnvSetting{Name: n, Value: agentio.NewEnvValue(l.Settings.Env[n]), From: l.Scope, Secret: secret[n]})
		}
	}
	return out, nil
}
