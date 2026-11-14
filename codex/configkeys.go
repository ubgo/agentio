package codex

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// Config key table for `-c path=value` overrides: every dotted config.toml
// path this library decodes into a typed field, with its value type and,
// for enums, the documented values. A launcher (agx --setting) uses it to
// type-check an override before Codex sees it, to list keys, and to build
// a schema. "*" in a path stands for any table key (a profile, MCP server
// or provider name).
//
// The table is written once here; a test walks the decoder's switch cases
// (go/ast over config.go) so a key the decoder learns fails until it is
// listed, and probes every entry through ParseConfig so a wrong kind
// fails too. CheckOverride itself decodes through the real decoder.

// ConfigKey is one typed config.toml path.
type ConfigKey struct {
	// Path is dotted, "*" for any table key: "mcp_servers.*.command".
	Path string `json:"path"`
	// Kind is the value type: string, enum, bool, number (an integer),
	// list (an array of strings).
	Kind agentio.OptionKind `json:"kind"`
	// Values are an enum's documented values. Codex keeps unknown values,
	// so an unlisted value is a warning, not a refusal.
	Values []string `json:"values,omitempty"`
}

// Config key kinds, named after agentio.OptionKind.
const (
	kStr  = agentio.OptionString
	kEnum = agentio.OptionEnum
	kBool = agentio.OptionBool
	kInt  = agentio.OptionNumber
	kList = agentio.OptionList
)

// profileScalarKeys are the keys valid both at the root and inside a
// legacy [profiles.<name>] table (decodeProfileScalar).
func profileScalarKeys() []ConfigKey {
	return []ConfigKey{
		{"model", kStr, nil},
		{"service_tier", kStr, nil},
		{"model_provider", kStr, nil},
		{keyApprovalPolicy, kEnum, valuesOf(ApprovalOnRequest, ApprovalNever, ApprovalUntrusted, ApprovalOnFailure)},
		{"approvals_reviewer", kEnum, valuesOf(ReviewerUser, ReviewerAutoReview, ReviewerGuardianSubagent)},
		{"sandbox_mode", kEnum, valuesOf(ModeReadOnly, ModeWorkspaceWrite, ModeDangerFullAccess)},
		{"model_reasoning_effort", kEnum, effortValues()},
		{"plan_mode_reasoning_effort", kEnum, effortValues()},
		{"model_reasoning_summary", kEnum, valuesOf(SummaryAuto, SummaryConcise, SummaryDetailed, SummaryNone)},
		{"model_verbosity", kEnum, valuesOf(VerbosityLow, VerbosityMedium, VerbosityHigh)},
		{"model_catalog_json", kStr, nil},
		{"personality", kEnum, valuesOf(PersonalityNone, PersonalityFriendly, PersonalityPragmatic)},
		{"chatgpt_base_url", kStr, nil},
		{"model_instructions_file", kStr, nil},
		{"web_search", kEnum, valuesOf(WebSearchDisabled, WebSearchCached, WebSearchIndexed, WebSearchLive)},
		{"oss_provider", kStr, nil},
	}
}

// effortValues are the reasoning effort levels.
func effortValues() []string {
	return valuesOf(EffortNone, EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax, EffortUltra, EffortPersistent)
}

// valuesOf spells enum constants. cost: Pure.
func valuesOf[S ~string](vs ...S) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}

// ConfigKeys lists every typed config path, sorted. cost: Pure.
func ConfigKeys() []ConfigKey {
	keys := profileScalarKeys()
	for _, k := range profileScalarKeys() {
		k.Path = keyProfiles + ".*." + k.Path
		keys = append(keys, k)
	}
	keys = append(keys,
		ConfigKey{"model_context_window", kInt, nil},
		ConfigKey{"model_auto_compact_token_limit", kInt, nil},
		ConfigKey{"review_model", kStr, nil},
		ConfigKey{"openai_base_url", kStr, nil},
		ConfigKey{"default_permissions", kStr, nil},
		ConfigKey{"allow_login_shell", kBool, nil},
		ConfigKey{"forced_login_method", kEnum, valuesOf(LoginChatGPT, LoginAPI)},
		ConfigKey{"forced_chatgpt_workspace_id", kList, nil},
		ConfigKey{"cli_auth_credentials_store", kEnum, valuesOf(StoreFile, StoreKeyring, StoreAuto, StoreEphemeral)},
		ConfigKey{"mcp_oauth_credentials_store", kEnum, valuesOf(OAuthStoreAuto, OAuthStoreFile, OAuthStoreKeyring)},
		ConfigKey{"profile", kStr, nil},
		ConfigKey{"instructions", kStr, nil},
		ConfigKey{"developer_instructions", kStr, nil},
		ConfigKey{"compact_prompt", kStr, nil},
		ConfigKey{"project_doc_max_bytes", kInt, nil},
		ConfigKey{"project_doc_fallback_filenames", kList, nil},
		ConfigKey{"project_root_markers", kList, nil},
		ConfigKey{"notify", kList, nil},
		ConfigKey{"file_opener", kEnum, valuesOf(OpenerVSCode, OpenerVSCodeInsiders, OpenerWindsurf, OpenerCursor, OpenerNone)},
		ConfigKey{"hide_agent_reasoning", kBool, nil},
		ConfigKey{"show_raw_agent_reasoning", kBool, nil},
		ConfigKey{"check_for_update_on_startup", kBool, nil},
		ConfigKey{"sqlite_home", kStr, nil},
		ConfigKey{"log_dir", kStr, nil},
		ConfigKey{"analytics.enabled", kBool, nil},
		ConfigKey{"feedback.enabled", kBool, nil},
		ConfigKey{"sandbox_workspace_write.writable_roots", kList, nil},
		ConfigKey{"sandbox_workspace_write.network_access", kBool, nil},
		ConfigKey{"sandbox_workspace_write.exclude_tmpdir_env_var", kBool, nil},
		ConfigKey{"sandbox_workspace_write.exclude_slash_tmp", kBool, nil},
		ConfigKey{"shell_environment_policy.inherit", kEnum, valuesOf(InheritCore, InheritAll, InheritNone)},
		ConfigKey{"shell_environment_policy.ignore_default_excludes", kBool, nil},
		ConfigKey{"shell_environment_policy.experimental_use_profile", kBool, nil},
		ConfigKey{"shell_environment_policy.exclude", kList, nil},
		ConfigKey{"shell_environment_policy.include_only", kList, nil},
		ConfigKey{"history.persistence", kEnum, valuesOf(HistorySaveAll, HistoryNone)},
		ConfigKey{"history.max_bytes", kInt, nil},
		ConfigKey{"goals.max_goal_token_budget", kInt, nil},
		ConfigKey{keyProjects + ".*.trust_level", kEnum, valuesOf(TrustTrusted, TrustUntrusted)},
		ConfigKey{keyPlugins + ".*." + keyEnabled, kBool, nil},
		ConfigKey{keyFeatures + ".*", kBool, nil},
		ConfigKey{"agents.enabled", kBool, nil},
		ConfigKey{"agents.max_concurrent_threads_per_session", kInt, nil},
		ConfigKey{"agents.max_depth", kInt, nil},
		ConfigKey{"agents.default_subagent_model", kStr, nil},
		ConfigKey{"agents.default_subagent_reasoning_effort", kEnum, effortValues()},
		ConfigKey{"agents.job_max_runtime_seconds", kInt, nil},
		ConfigKey{"agents.interrupt_message", kBool, nil},
		ConfigKey{"skills.bundled.enabled", kBool, nil},
		ConfigKey{"skills.include_instructions", kBool, nil},
		ConfigKey{"skills.max_context_tokens", kInt, nil},
		ConfigKey{keyMCPServers + ".*.command", kStr, nil},
		ConfigKey{keyMCPServers + ".*.args", kList, nil},
		ConfigKey{keyMCPServers + ".*.cwd", kStr, nil},
		ConfigKey{keyMCPServers + ".*.url", kStr, nil},
		ConfigKey{keyMCPServers + ".*.bearer_token_env_var", kStr, nil},
		ConfigKey{keyMCPServers + ".*.oauth_resource", kStr, nil},
		ConfigKey{keyMCPServers + ".*.scopes", kList, nil},
		ConfigKey{keyMCPServers + ".*." + keyEnabled, kBool, nil},
		ConfigKey{keyMCPServers + ".*.required", kBool, nil},
		ConfigKey{keyMCPServers + ".*.supports_parallel_tool_calls", kBool, nil},
		ConfigKey{keyMCPServers + ".*.startup_timeout_ms", kInt, nil},
		ConfigKey{keyMCPServers + ".*.enabled_tools", kList, nil},
		ConfigKey{keyMCPServers + ".*.disabled_tools", kList, nil},
		ConfigKey{keyMCPServers + ".*.default_tools_approval_mode", kEnum, valuesOf(ToolApprovalAuto, ToolApprovalPrompt, ToolApprovalWrites, ToolApprovalApprove)},
		ConfigKey{keyMCPServers + ".*.startup_readiness", kEnum, valuesOf(ReadinessConnection, ReadinessCatalog)},
		ConfigKey{keyModelProviders + ".*.name", kStr, nil},
		ConfigKey{keyModelProviders + ".*.base_url", kStr, nil},
		ConfigKey{keyModelProviders + ".*.env_key", kStr, nil},
		ConfigKey{keyModelProviders + ".*.env_key_instructions", kStr, nil},
		ConfigKey{keyModelProviders + ".*.model_catalog_url", kStr, nil},
		ConfigKey{keyModelProviders + ".*.wire_api", kEnum, valuesOf(WireResponses)},
		ConfigKey{keyModelProviders + ".*.request_max_retries", kInt, nil},
		ConfigKey{keyModelProviders + ".*.stream_max_retries", kInt, nil},
		ConfigKey{keyModelProviders + ".*.stream_idle_timeout_ms", kInt, nil},
		ConfigKey{keyModelProviders + ".*.websocket_connect_timeout_ms", kInt, nil},
		ConfigKey{keyModelProviders + ".*.requires_openai_auth", kBool, nil},
		ConfigKey{keyModelProviders + ".*.supports_websockets", kBool, nil},
		ConfigKey{keyModelProviders + ".*.supports_standalone_web_search", kBool, nil},
		ConfigKey{keyMarketplaces + ".*.source", kStr, nil},
		ConfigKey{keyMarketplaces + ".*.ref", kStr, nil},
		ConfigKey{keyMarketplaces + ".*.sparse_paths", kList, nil},
	)
	slices.SortFunc(keys, func(a, b ConfigKey) int { return strings.Compare(a.Path, b.Path) })
	return keys
}

// LookupConfigKey finds the table entry for a concrete dotted path
// ("mcp_servers.github.command" matches "mcp_servers.*.command"). Pure.
func LookupConfigKey(path string) (ConfigKey, bool) {
	segs := strings.Split(path, ".")
	for _, k := range ConfigKeys() {
		pat := strings.Split(k.Path, ".")
		if len(pat) != len(segs) {
			continue
		}
		match := true
		for i := range pat {
			if pat[i] != "*" && pat[i] != segs[i] {
				match = false
				break
			}
		}
		if match {
			return k, true
		}
	}
	return ConfigKey{}, false
}

// OverrideStatus is how CheckOverride classified an override.
type OverrideStatus string

// OverrideStatus values.
const (
	// OverrideTyped: a modelled key with a value of the right type.
	OverrideTyped OverrideStatus = "typed"
	// OverrideUnlistedValue: a modelled enum key with a value Codex may
	// not know (Codex keeps unknown values, so it is a warning).
	OverrideUnlistedValue OverrideStatus = "unlisted-value"
	// OverrideRaw: a section this library keeps raw ([tui], [permissions.*]);
	// passed through unchecked.
	OverrideRaw OverrideStatus = "raw"
	// OverrideUnknown: a key this library does not model; passed through
	// unchecked (Codex may know it).
	OverrideUnknown OverrideStatus = "unknown"
	// OverrideTypeError: a modelled key with a value of the wrong type;
	// Codex would ignore or reject it.
	OverrideTypeError OverrideStatus = "type-error"
)

// OverrideCheck is the result of CheckOverride.
type OverrideCheck struct {
	Status OverrideStatus
	// Key is the table entry for a typed, unlisted-value or type-error
	// result.
	Key ConfigKey
	// Detail says what was expected, never the value.
	Detail string
}

// CheckOverride type-checks one `-c path=value` by decoding it through the
// same decoder config.toml goes through, then checks an enum value against
// the table. The value stays as built (Override constructors do the
// quoting); nothing is written. A value that is not TOML is an error.
// cost: Pure.
func CheckOverride(o Override) (OverrideCheck, error) {
	src := o.Path + " = " + string(o.Value)
	c, warns, err := ParseConfig([]byte(src), "override")
	if err != nil {
		return OverrideCheck{}, fmt.Errorf("codex: override %s: %w", o.Path, err)
	}
	key, listed := LookupConfigKey(o.Path)
	counts := c.KeyCount()
	switch {
	case len(warns) > 0:
		return OverrideCheck{Status: OverrideTypeError, Key: key, Detail: warns[0].Detail}, nil
	case counts.Raw > 0:
		return OverrideCheck{Status: OverrideRaw}, nil
	case counts.Unknown > 0 || !listed:
		return OverrideCheck{Status: OverrideUnknown}, nil
	}
	if key.Kind == kEnum {
		var v string
		if _, err := fmt.Sscanf(string(o.Value), "%q", &v); err == nil && !slices.Contains(key.Values, v) {
			return OverrideCheck{Status: OverrideUnlistedValue, Key: key, Detail: "one of " + strings.Join(key.Values, ", ")}, nil
		}
	}
	return OverrideCheck{Status: OverrideTyped, Key: key}, nil
}
