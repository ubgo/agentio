package codex

// Rule: NO BARE STRINGS for Codex wire values. Every vendor enum is an open
// string type with named constants and IsKnown(): Codex ships weekly, so an
// unknown value is kept verbatim (SPEC §5.5, invariant 13), never rejected.
// Values are verbatim from upstream rust-v0.162.1 (config.schema.json,
// protocol/src/config_types.rs, protocol/src/protocol.rs).

// ApprovalPolicy is `approval_policy` (AskForApproval upstream, kebab-case).
// "untrusted" and "on-failure" come from older versions and stay known so
// old rollouts decode without drift noise.
type ApprovalPolicy string

// ApprovalPolicy values.
const (
	ApprovalOnRequest ApprovalPolicy = "on-request"
	ApprovalGranular  ApprovalPolicy = "granular"
	ApprovalNever     ApprovalPolicy = "never"
	ApprovalUntrusted ApprovalPolicy = "untrusted"
	ApprovalOnFailure ApprovalPolicy = "on-failure"
)

// IsKnown reports whether p is a value this library was written against. Pure.
func (p ApprovalPolicy) IsKnown() bool {
	switch p {
	case ApprovalOnRequest, ApprovalGranular, ApprovalNever, ApprovalUntrusted, ApprovalOnFailure:
		return true
	}
	return false
}

// SandboxMode is `sandbox_mode` / `-s`.
type SandboxMode string

// SandboxMode values.
const (
	ModeReadOnly         SandboxMode = "read-only"
	ModeWorkspaceWrite   SandboxMode = "workspace-write"
	ModeDangerFullAccess SandboxMode = "danger-full-access"
	// ModeExternalSandbox appears only inside a rollout's sandbox_policy
	// ("external-sandbox"), never as a config value.
	ModeExternalSandbox SandboxMode = "external-sandbox"
)

// IsKnown reports whether m is a known sandbox mode. Pure.
func (m SandboxMode) IsKnown() bool {
	switch m {
	case ModeReadOnly, ModeWorkspaceWrite, ModeDangerFullAccess, ModeExternalSandbox:
		return true
	}
	return false
}

// ReasoningEffort is free-form upstream ("advertised by the model"); the
// constants are the ones openai_models.rs names.
type ReasoningEffort string

// ReasoningEffort values.
const (
	EffortNone       ReasoningEffort = "none"
	EffortMinimal    ReasoningEffort = "minimal"
	EffortLow        ReasoningEffort = "low"
	EffortMedium     ReasoningEffort = "medium"
	EffortHigh       ReasoningEffort = "high"
	EffortXHigh      ReasoningEffort = "xhigh"
	EffortMax        ReasoningEffort = "max"
	EffortUltra      ReasoningEffort = "ultra"
	EffortPersistent ReasoningEffort = "persistent"
)

// IsKnown reports whether e is a named effort. Pure.
func (e ReasoningEffort) IsKnown() bool {
	switch e {
	case EffortNone, EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax, EffortUltra, EffortPersistent:
		return true
	}
	return false
}

// ReasoningSummary is `model_reasoning_summary`.
type ReasoningSummary string

// ReasoningSummary values.
const (
	SummaryAuto     ReasoningSummary = "auto"
	SummaryConcise  ReasoningSummary = "concise"
	SummaryDetailed ReasoningSummary = "detailed"
	SummaryNone     ReasoningSummary = "none"
)

// IsKnown reports whether s is a known summary mode. Pure.
func (s ReasoningSummary) IsKnown() bool {
	switch s {
	case SummaryAuto, SummaryConcise, SummaryDetailed, SummaryNone:
		return true
	}
	return false
}

// Verbosity is `model_verbosity`.
type Verbosity string

// Verbosity values.
const (
	VerbosityLow    Verbosity = "low"
	VerbosityMedium Verbosity = "medium"
	VerbosityHigh   Verbosity = "high"
)

// IsKnown reports whether v is a known verbosity. Pure.
func (v Verbosity) IsKnown() bool {
	return v == VerbosityLow || v == VerbosityMedium || v == VerbosityHigh
}

// WebSearchMode is `web_search`.
type WebSearchMode string

// WebSearchMode values.
const (
	WebSearchDisabled WebSearchMode = "disabled"
	WebSearchCached   WebSearchMode = "cached"
	WebSearchIndexed  WebSearchMode = "indexed"
	WebSearchLive     WebSearchMode = "live"
)

// IsKnown reports whether m is a known web-search mode. Pure.
func (m WebSearchMode) IsKnown() bool {
	switch m {
	case WebSearchDisabled, WebSearchCached, WebSearchIndexed, WebSearchLive:
		return true
	}
	return false
}

// ApprovalsReviewer is `approvals_reviewer`; "guardian_subagent" is the
// upstream alias of auto_review.
type ApprovalsReviewer string

// ApprovalsReviewer values.
const (
	ReviewerUser             ApprovalsReviewer = "user"
	ReviewerAutoReview       ApprovalsReviewer = "auto_review"
	ReviewerGuardianSubagent ApprovalsReviewer = "guardian_subagent"
)

// IsKnown reports whether r is a known reviewer. Pure.
func (r ApprovalsReviewer) IsKnown() bool {
	return r == ReviewerUser || r == ReviewerAutoReview || r == ReviewerGuardianSubagent
}

// TrustLevel is `[projects."<dir>"] trust_level`.
type TrustLevel string

// TrustLevel values.
const (
	TrustTrusted   TrustLevel = "trusted"
	TrustUntrusted TrustLevel = "untrusted"
)

// IsKnown reports whether t is a known trust level. Pure.
func (t TrustLevel) IsKnown() bool { return t == TrustTrusted || t == TrustUntrusted }

// LoginMethod is `forced_login_method`.
type LoginMethod string

// LoginMethod values.
const (
	LoginChatGPT LoginMethod = "chatgpt"
	LoginAPI     LoginMethod = "api"
)

// IsKnown reports whether m is a known login method. Pure.
func (m LoginMethod) IsKnown() bool { return m == LoginChatGPT || m == LoginAPI }

// CredentialStore is `cli_auth_credentials_store`.
type CredentialStore string

// CredentialStore values.
const (
	StoreFile      CredentialStore = "file"
	StoreKeyring   CredentialStore = "keyring"
	StoreAuto      CredentialStore = "auto"
	StoreEphemeral CredentialStore = "ephemeral"
)

// IsKnown reports whether s is a known store. Pure.
func (s CredentialStore) IsKnown() bool {
	switch s {
	case StoreFile, StoreKeyring, StoreAuto, StoreEphemeral:
		return true
	}
	return false
}

// OAuthStore is `mcp_oauth_credentials_store`.
type OAuthStore string

// OAuthStore values.
const (
	OAuthStoreAuto    OAuthStore = "auto"
	OAuthStoreFile    OAuthStore = "file"
	OAuthStoreKeyring OAuthStore = "keyring"
)

// IsKnown reports whether s is a known OAuth store. Pure.
func (s OAuthStore) IsKnown() bool {
	return s == OAuthStoreAuto || s == OAuthStoreFile || s == OAuthStoreKeyring
}

// Personality is `personality` (deprecated upstream, still read).
type Personality string

// Personality values.
const (
	PersonalityNone      Personality = "none"
	PersonalityFriendly  Personality = "friendly"
	PersonalityPragmatic Personality = "pragmatic"
)

// IsKnown reports whether p is a known personality. Pure.
func (p Personality) IsKnown() bool {
	return p == PersonalityNone || p == PersonalityFriendly || p == PersonalityPragmatic
}

// FileOpener is `file_opener`.
type FileOpener string

// FileOpener values.
const (
	OpenerVSCode         FileOpener = "vscode"
	OpenerVSCodeInsiders FileOpener = "vscode-insiders"
	OpenerWindsurf       FileOpener = "windsurf"
	OpenerCursor         FileOpener = "cursor"
	OpenerNone           FileOpener = "none"
)

// IsKnown reports whether o is a known opener. Pure.
func (o FileOpener) IsKnown() bool {
	switch o {
	case OpenerVSCode, OpenerVSCodeInsiders, OpenerWindsurf, OpenerCursor, OpenerNone:
		return true
	}
	return false
}

// WireAPI is `model_providers.<x>.wire_api`; only "responses" at 0.162.1.
type WireAPI string

// WireResponses is the only wire API at 0.162.1.
const WireResponses WireAPI = "responses"

// IsKnown reports whether w is a known wire API. Pure.
func (w WireAPI) IsKnown() bool { return w == WireResponses }

// HistoryPersistence is `[history] persistence`.
type HistoryPersistence string

// HistoryPersistence values.
const (
	HistorySaveAll HistoryPersistence = "save-all"
	HistoryNone    HistoryPersistence = "none"
)

// IsKnown reports whether p is a known persistence mode. Pure.
func (p HistoryPersistence) IsKnown() bool { return p == HistorySaveAll || p == HistoryNone }

// EnvInherit is `[shell_environment_policy] inherit`.
type EnvInherit string

// EnvInherit values.
const (
	InheritCore EnvInherit = "core"
	InheritAll  EnvInherit = "all"
	InheritNone EnvInherit = "none"
)

// IsKnown reports whether e is a known inherit mode. Pure.
func (e EnvInherit) IsKnown() bool { return e == InheritCore || e == InheritAll || e == InheritNone }

// MCPAuth is `mcp_servers.<x>.auth`.
type MCPAuth string

// MCPAuth values.
const (
	MCPAuthOAuth   MCPAuth = "oauth"
	MCPAuthChatGPT MCPAuth = "chatgpt"
	MCPAuthEMA     MCPAuth = "ema_auth"
)

// IsKnown reports whether a is a known MCP auth mode. Pure.
func (a MCPAuth) IsKnown() bool { return a == MCPAuthOAuth || a == MCPAuthChatGPT || a == MCPAuthEMA }

// ToolApproval is `mcp_servers.<x>.default_tools_approval_mode`.
type ToolApproval string

// ToolApproval values.
const (
	ToolApprovalAuto    ToolApproval = "auto"
	ToolApprovalPrompt  ToolApproval = "prompt"
	ToolApprovalWrites  ToolApproval = "writes"
	ToolApprovalApprove ToolApproval = "approve"
)

// IsKnown reports whether t is a known tool approval mode. Pure.
func (t ToolApproval) IsKnown() bool {
	switch t {
	case ToolApprovalAuto, ToolApprovalPrompt, ToolApprovalWrites, ToolApprovalApprove:
		return true
	}
	return false
}

// MCPReadiness is `mcp_servers.<x>.startup_readiness`.
type MCPReadiness string

// MCPReadiness values.
const (
	ReadinessConnection MCPReadiness = "connection"
	ReadinessCatalog    MCPReadiness = "catalog"
)

// IsKnown reports whether r is a known readiness mode. Pure.
func (r MCPReadiness) IsKnown() bool { return r == ReadinessConnection || r == ReadinessCatalog }

// HookEvent names a `[hooks]` event (same names as Claude Code hooks).
type HookEvent string

// HookEvent values (config_toml.rs hooks at 0.162.1).
const (
	HookSessionStart      HookEvent = "SessionStart"
	HookSessionEnd        HookEvent = "SessionEnd"
	HookUserPromptSubmit  HookEvent = "UserPromptSubmit"
	HookPreToolUse        HookEvent = "PreToolUse"
	HookPostToolUse       HookEvent = "PostToolUse"
	HookPermissionRequest HookEvent = "PermissionRequest"
	HookStop              HookEvent = "Stop"
	HookSubagentStart     HookEvent = "SubagentStart"
	HookSubagentStop      HookEvent = "SubagentStop"
	HookPreCompact        HookEvent = "PreCompact"
	HookPostCompact       HookEvent = "PostCompact"
	HookInterrupt         HookEvent = "Interrupt"
)

// IsKnown reports whether e is a known hook event. Pure.
func (e HookEvent) IsKnown() bool {
	switch e {
	case HookSessionStart, HookSessionEnd, HookUserPromptSubmit, HookPreToolUse, HookPostToolUse,
		HookPermissionRequest, HookStop, HookSubagentStart, HookSubagentStop, HookPreCompact,
		HookPostCompact, HookInterrupt:
		return true
	}
	return false
}

// SessionSource is a rollout's session_meta.source / threads.source.
// Upstream it is an enum whose Custom/Internal/SubAgent variants serialize as
// objects ({"subagent": {...}}); those decode to the variant name.
type SessionSource string

// SessionSource values.
const (
	SourceCLI      SessionSource = "cli"
	SourceVSCode   SessionSource = "vscode"
	SourceExec     SessionSource = "exec"
	SourceMCP      SessionSource = "mcp"
	SourceCustom   SessionSource = "custom"
	SourceInternal SessionSource = "internal"
	SourceSubagent SessionSource = "subagent"
	SourceUnknown  SessionSource = "unknown"
)

// IsKnown reports whether s is a known source. Pure.
func (s SessionSource) IsKnown() bool {
	switch s {
	case SourceCLI, SourceVSCode, SourceExec, SourceMCP, SourceCustom, SourceInternal, SourceSubagent, SourceUnknown:
		return true
	}
	return false
}

// AuthMode is auth.json `auth_mode`.
type AuthMode string

// AuthMode values; AuthNone is the absent/empty value.
const (
	AuthNone    AuthMode = ""
	AuthChatGPT AuthMode = "chatgpt"
	AuthAPIKey  AuthMode = "apikey"
)

// IsKnown reports whether m is a known auth mode. Pure.
func (m AuthMode) IsKnown() bool { return m == AuthNone || m == AuthChatGPT || m == AuthAPIKey }
