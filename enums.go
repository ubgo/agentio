// Closed sets owned by the library (SPEC §5.5): uint8 values with String, MarshalText, UnmarshalText, a Values list and a Parse function each.

package agentio

// CostClass orders what a call may spend (SPEC §4). Library-owned closed set; a capability's cost is the worst case of its methods.
type CostClass uint8

// CostClass values. Order is the wire order of names; append only.
const (
	// CostPure: no I/O: parsing, building a Command.
	CostPure CostClass = iota
	// CostDisk: reads files through Host-derived paths.
	CostDisk
	// CostProcess: runs a local program once (vendor binary, `security`, `ps`).
	CostProcess
	// CostNetwork: one HTTP request to the vendor; may 429; cache, never poll.
	CostNetwork
	// CostQuota: spends model tokens of an account; never loop.
	CostQuota
)

// costClassKind labels CostClass in errors.
const costClassKind = "cost class"

// costClassNames is the wire spelling of each CostClass, indexed by value. Pure.
func costClassNames() []string {
	return []string{"pure", "disk", "process", "network", "quota"}
}

// String returns the wire spelling. Pure.
func (v CostClass) String() string { return enumString(v, costClassKind, costClassNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v CostClass) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *CostClass) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, costClassKind, costClassNames())
}

// CostClassValues lists every CostClass in order: the canonical iteration list. Pure.
func CostClassValues() []CostClass { return enumValues[CostClass](costClassNames()) }

// ParseCostClass parses a wire spelling. Pure.
func ParseCostClass(s string) (CostClass, bool) { return enumParse[CostClass](s, costClassNames()) }

// Access says whether a call only reads or also writes (SPEC §4).
type Access uint8

// Access values. Order is the wire order of names; append only.
const (
	// Read: reads only.
	Read Access = iota
	// Write: writes a file the vendor CLI owns; always backs it up first (invariant 11).
	Write
)

// accessKind labels Access in errors.
const accessKind = "access"

// accessNames is the wire spelling of each Access, indexed by value. Pure.
func accessNames() []string {
	return []string{"read", "write"}
}

// String returns the wire spelling. Pure.
func (v Access) String() string { return enumString(v, accessKind, accessNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v Access) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *Access) UnmarshalText(b []byte) error { return enumUnmarshal(v, b, accessKind, accessNames()) }

// AccessValues lists every Access in order: the canonical iteration list. Pure.
func AccessValues() []Access { return enumValues[Access](accessNames()) }

// ParseAccess parses a wire spelling. Pure.
func ParseAccess(s string) (Access, bool) { return enumParse[Access](s, accessNames()) }

// TokenKind is the kind of credential a token login carries. It decides which variable the vendor CLI reads it from and how the account is billed.
type TokenKind uint8

// TokenKind values. Order is the wire order of names; append only.
const (
	// TokenKindUnknown: zero value: kind not known; never launched with.
	TokenKindUnknown TokenKind = iota
	// TokenOAuthToken: long-lived plan token (Claude `setup-token`): spends the plan quota, never refreshed.
	TokenOAuthToken
	// TokenAPIKey: pay-per-token API key.
	TokenAPIKey
	// TokenAPIKeyHelper: a command the agent itself runs to get its key (Claude settings apiKeyHelper); the key never passes through the caller.
	TokenAPIKeyHelper
)

// tokenKindKind labels TokenKind in errors.
const tokenKindKind = "token kind"

// tokenKindNames is the wire spelling of each TokenKind, indexed by value. Pure.
func tokenKindNames() []string {
	return []string{"unknown", "oauth-token", "api-key", "api-key-helper"}
}

// String returns the wire spelling. Pure.
func (v TokenKind) String() string { return enumString(v, tokenKindKind, tokenKindNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v TokenKind) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *TokenKind) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, tokenKindKind, tokenKindNames())
}

// TokenKindValues lists every TokenKind in order: the canonical iteration list. Pure.
func TokenKindValues() []TokenKind { return enumValues[TokenKind](tokenKindNames()) }

// ParseTokenKind parses a wire spelling. Pure.
func ParseTokenKind(s string) (TokenKind, bool) { return enumParse[TokenKind](s, tokenKindNames()) }

// Billing says how an account or launch is paid for, which decides whether usage windows exist at all.
type Billing uint8

// Billing values. Order is the wire order of names; append only.
const (
	// BillingUnknown: not known.
	BillingUnknown Billing = iota
	// BillingSubscription: a plan with usage windows.
	BillingSubscription
	// BillingPerToken: billed per token: no windows exist (an answer, not a failure).
	BillingPerToken
)

// billingKind labels Billing in errors.
const billingKind = "billing"

// billingNames is the wire spelling of each Billing, indexed by value. Pure.
func billingNames() []string {
	return []string{"unknown", "subscription", "per-token"}
}

// String returns the wire spelling. Pure.
func (v Billing) String() string { return enumString(v, billingKind, billingNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v Billing) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *Billing) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, billingKind, billingNames())
}

// BillingValues lists every Billing in order: the canonical iteration list. Pure.
func BillingValues() []Billing { return enumValues[Billing](billingNames()) }

// ParseBilling parses a wire spelling. Pure.
func ParseBilling(s string) (Billing, bool) { return enumParse[Billing](s, billingNames()) }

// WindowClass buckets a usage window by its length so windows of different vendors sort together (a 3-hour window with a 5-hour one). Derived from Window.Length, never stored.
type WindowClass uint8

// WindowClass values. Order is the wire order of names; append only.
const (
	// WindowUnknown: length not given by the vendor.
	WindowUnknown WindowClass = iota
	// WindowShort: shorter than one day (Claude 5-hour session).
	WindowShort
	// WindowDay: one day up to a week.
	WindowDay
	// WindowWeek: one week up to a month.
	WindowWeek
	// WindowMonth: a month or longer.
	WindowMonth
)

// windowClassKind labels WindowClass in errors.
const windowClassKind = "window class"

// windowClassNames is the wire spelling of each WindowClass, indexed by value. Pure.
func windowClassNames() []string {
	return []string{"unknown", "short", "day", "week", "month"}
}

// String returns the wire spelling. Pure.
func (v WindowClass) String() string { return enumString(v, windowClassKind, windowClassNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v WindowClass) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *WindowClass) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, windowClassKind, windowClassNames())
}

// WindowClassValues lists every WindowClass in order: the canonical iteration list. Pure.
func WindowClassValues() []WindowClass { return enumValues[WindowClass](windowClassNames()) }

// ParseWindowClass parses a wire spelling. Pure.
func ParseWindowClass(s string) (WindowClass, bool) {
	return enumParse[WindowClass](s, windowClassNames())
}

// Autonomy is the neutral permission level of a launch (SPEC §5.4). A level with no exact vendor mode is refused, never mapped to a more permissive one; AutonomyFull needs Target.AllowDangerous.
type Autonomy uint8

// Autonomy values. Order is the wire order of names; append only.
const (
	// AutonomyDefault: no flag: the vendor's configured default.
	AutonomyDefault Autonomy = iota
	// AutonomyReadOnly: may read, never edit or run (Claude plan mode).
	AutonomyReadOnly
	// AutonomyAsk: asks before edits and commands.
	AutonomyAsk
	// AutonomyAcceptEdits: edits without asking, asks for commands.
	AutonomyAcceptEdits
	// AutonomyFull: no prompts, no sandbox; needs Target.AllowDangerous.
	AutonomyFull
)

// autonomyKind labels Autonomy in errors.
const autonomyKind = "autonomy"

// autonomyNames is the wire spelling of each Autonomy, indexed by value. Pure.
func autonomyNames() []string {
	return []string{"default", "read-only", "ask", "accept-edits", "full"}
}

// String returns the wire spelling. Pure.
func (v Autonomy) String() string { return enumString(v, autonomyKind, autonomyNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v Autonomy) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *Autonomy) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, autonomyKind, autonomyNames())
}

// AutonomyValues lists every Autonomy in order: the canonical iteration list. Pure.
func AutonomyValues() []Autonomy { return enumValues[Autonomy](autonomyNames()) }

// ParseAutonomy parses a wire spelling. Pure.
func ParseAutonomy(s string) (Autonomy, bool) { return enumParse[Autonomy](s, autonomyNames()) }

// UsageSource says where a Usage reading came from, so a caller can show its age and trust.
type UsageSource uint8

// UsageSource values. Order is the wire order of names; append only.
const (
	// UsageSourceUnknown: not known.
	UsageSourceUnknown UsageSource = iota
	// UsageServer: vendor usage endpoint (network).
	UsageServer
	// UsageCache: a vendor-written cache on disk (Claude cachedUsageUtilization).
	UsageCache
	// UsageTranscript: a record in a transcript (Codex rollout token_count).
	UsageTranscript
	// UsageProbe: headers of a quota-spending probe call.
	UsageProbe
)

// usageSourceKind labels UsageSource in errors.
const usageSourceKind = "usage source"

// usageSourceNames is the wire spelling of each UsageSource, indexed by value. Pure.
func usageSourceNames() []string {
	return []string{"unknown", "server", "cache", "transcript", "probe"}
}

// String returns the wire spelling. Pure.
func (v UsageSource) String() string { return enumString(v, usageSourceKind, usageSourceNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v UsageSource) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *UsageSource) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, usageSourceKind, usageSourceNames())
}

// UsageSourceValues lists every UsageSource in order: the canonical iteration list. Pure.
func UsageSourceValues() []UsageSource { return enumValues[UsageSource](usageSourceNames()) }

// ParseUsageSource parses a wire spelling. Pure.
func ParseUsageSource(s string) (UsageSource, bool) {
	return enumParse[UsageSource](s, usageSourceNames())
}

// RunEventType classifies one event of a headless run stream. Unknown vendor events are RunEventOther with Raw kept.
type RunEventType uint8

// RunEventType values. Order is the wire order of names; append only.
const (
	// RunEventOther: an event the decoder does not classify; Raw holds it.
	RunEventOther RunEventType = iota
	// RunEventStarted: the run or conversation started.
	RunEventStarted
	// RunEventMessage: assistant or user text.
	RunEventMessage
	// RunEventTool: a tool call or result.
	RunEventTool
	// RunEventUsage: token counts.
	RunEventUsage
	// RunEventDone: the run finished.
	RunEventDone
	// RunEventError: the vendor reported an error.
	RunEventError
)

// runEventTypeKind labels RunEventType in errors.
const runEventTypeKind = "run event type"

// runEventTypeNames is the wire spelling of each RunEventType, indexed by value. Pure.
func runEventTypeNames() []string {
	return []string{"other", "started", "message", "tool", "usage", "done", "error"}
}

// String returns the wire spelling. Pure.
func (v RunEventType) String() string { return enumString(v, runEventTypeKind, runEventTypeNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v RunEventType) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *RunEventType) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, runEventTypeKind, runEventTypeNames())
}

// RunEventTypeValues lists every RunEventType in order: the canonical iteration list. Pure.
func RunEventTypeValues() []RunEventType { return enumValues[RunEventType](runEventTypeNames()) }

// ParseRunEventType parses a wire spelling. Pure.
func ParseRunEventType(s string) (RunEventType, bool) {
	return enumParse[RunEventType](s, runEventTypeNames())
}

// CheckStatus is the outcome of one doctor check.
type CheckStatus uint8

// CheckStatus values. Order is the wire order of names; append only.
const (
	// CheckSkip: not applicable or not run (zero value, so a forgotten status never reads as ok).
	CheckSkip CheckStatus = iota
	// CheckOK: passed.
	CheckOK
	// CheckInfo: a fact worth showing, not a problem.
	CheckInfo
	// CheckWarn: works, but something is off (e.g. untested vendor version).
	CheckWarn
	// CheckFail: broken.
	CheckFail
)

// checkStatusKind labels CheckStatus in errors.
const checkStatusKind = "check status"

// checkStatusNames is the wire spelling of each CheckStatus, indexed by value. Pure.
func checkStatusNames() []string {
	return []string{"skip", "ok", "info", "warn", "fail"}
}

// String returns the wire spelling. Pure.
func (v CheckStatus) String() string { return enumString(v, checkStatusKind, checkStatusNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v CheckStatus) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *CheckStatus) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, checkStatusKind, checkStatusNames())
}

// CheckStatusValues lists every CheckStatus in order: the canonical iteration list. Pure.
func CheckStatusValues() []CheckStatus { return enumValues[CheckStatus](checkStatusNames()) }

// ParseCheckStatus parses a wire spelling. Pure.
func ParseCheckStatus(s string) (CheckStatus, bool) {
	return enumParse[CheckStatus](s, checkStatusNames())
}

// CompatStatus places an installed vendor version against the tested range (SPEC §5.8). Outside the range is a warning, never a refusal.
type CompatStatus uint8

// CompatStatus values. Order is the wire order of names; append only.
const (
	// CompatUnknown: version or range not known.
	CompatUnknown CompatStatus = iota
	// CompatInRange: within TestedMin..TestedMax.
	CompatInRange
	// CompatNewer: newer than TestedMax.
	CompatNewer
	// CompatOlder: older than TestedMin.
	CompatOlder
)

// compatStatusKind labels CompatStatus in errors.
const compatStatusKind = "compat status"

// compatStatusNames is the wire spelling of each CompatStatus, indexed by value. Pure.
func compatStatusNames() []string {
	return []string{"unknown", "in-range", "newer", "older"}
}

// String returns the wire spelling. Pure.
func (v CompatStatus) String() string { return enumString(v, compatStatusKind, compatStatusNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v CompatStatus) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *CompatStatus) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, compatStatusKind, compatStatusNames())
}

// CompatStatusValues lists every CompatStatus in order: the canonical iteration list. Pure.
func CompatStatusValues() []CompatStatus { return enumValues[CompatStatus](compatStatusNames()) }

// ParseCompatStatus parses a wire spelling. Pure.
func ParseCompatStatus(s string) (CompatStatus, bool) {
	return enumParse[CompatStatus](s, compatStatusNames())
}

// WarningKind classifies a drift warning a tolerant reader or builder returns instead of failing (SPEC §5.8).
type WarningKind uint8

// WarningKind values. Order is the wire order of names; append only.
const (
	// WarnUntestedVersion: installed vendor version is outside the tested range.
	WarnUntestedVersion WarningKind = iota
	// WarnVersionUnknown: installed vendor version is not known; built anyway.
	WarnVersionUnknown
	// WarnTypeChanged: a key's value has another type than modelled; raw kept in Unknown.
	WarnTypeChanged
	// WarnOldKeyName: a key was read under its old name.
	WarnOldKeyName
	// WarnUnknownRecord: a record type the reader does not model; kept as Raw.
	WarnUnknownRecord
	// WarnUnreadable: one entry could not be read or parsed; the rest of the result is valid.
	WarnUnreadable
	// WarnUnknownKey: a settings key or config override the library does not model; passed on unchecked.
	WarnUnknownKey
)

// warningKindKind labels WarningKind in errors.
const warningKindKind = "warning kind"

// warningKindNames is the wire spelling of each WarningKind, indexed by value. Pure.
func warningKindNames() []string {
	return []string{"untested-version", "version-unknown", "type-changed", "old-key-name", "unknown-record", "unreadable", "unknown-key"}
}

// String returns the wire spelling. Pure.
func (v WarningKind) String() string { return enumString(v, warningKindKind, warningKindNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v WarningKind) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *WarningKind) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, warningKindKind, warningKindNames())
}

// WarningKindValues lists every WarningKind in order: the canonical iteration list. Pure.
func WarningKindValues() []WarningKind { return enumValues[WarningKind](warningKindNames()) }

// ParseWarningKind parses a wire spelling. Pure.
func ParseWarningKind(s string) (WarningKind, bool) {
	return enumParse[WarningKind](s, warningKindNames())
}

// SessionStatus is a live session's activity as the vendor reports it.
type SessionStatus uint8

// SessionStatus values. Order is the wire order of names; append only.
const (
	// SessionStatusUnknown: vendor does not say.
	SessionStatusUnknown SessionStatus = iota
	// SessionIdle: waiting for input.
	SessionIdle
	// SessionBusy: working on a turn.
	SessionBusy
)

// sessionStatusKind labels SessionStatus in errors.
const sessionStatusKind = "session status"

// sessionStatusNames is the wire spelling of each SessionStatus, indexed by value. Pure.
func sessionStatusNames() []string {
	return []string{"unknown", "idle", "busy"}
}

// String returns the wire spelling. Pure.
func (v SessionStatus) String() string { return enumString(v, sessionStatusKind, sessionStatusNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v SessionStatus) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *SessionStatus) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, sessionStatusKind, sessionStatusNames())
}

// SessionStatusValues lists every SessionStatus in order: the canonical iteration list. Pure.
func SessionStatusValues() []SessionStatus { return enumValues[SessionStatus](sessionStatusNames()) }

// ParseSessionStatus parses a wire spelling. Pure.
func ParseSessionStatus(s string) (SessionStatus, bool) {
	return enumParse[SessionStatus](s, sessionStatusNames())
}

// Role is the side of a conversation turn.
type Role uint8

// Role values. Order is the wire order of names; append only.
const (
	// RoleUser: the user's turn.
	RoleUser Role = iota
	// RoleAssistant: the agent's turn.
	RoleAssistant
)

// roleKind labels Role in errors.
const roleKind = "role"

// roleNames is the wire spelling of each Role, indexed by value. Pure.
func roleNames() []string {
	return []string{"user", "assistant"}
}

// String returns the wire spelling. Pure.
func (v Role) String() string { return enumString(v, roleKind, roleNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v Role) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *Role) UnmarshalText(b []byte) error { return enumUnmarshal(v, b, roleKind, roleNames()) }

// RoleValues lists every Role in order: the canonical iteration list. Pure.
func RoleValues() []Role { return enumValues[Role](roleNames()) }

// ParseRole parses a wire spelling. Pure.
func ParseRole(s string) (Role, bool) { return enumParse[Role](s, roleNames()) }

// CredentialSource says where a home's login was read from.
type CredentialSource uint8

// CredentialSource values. Order is the wire order of names; append only.
const (
	// CredentialNone: no credential found.
	CredentialNone CredentialSource = iota
	// CredentialKeychain: macOS keychain.
	CredentialKeychain
	// CredentialFile: a file in the home (.credentials.json, auth.json).
	CredentialFile
	// CredentialKeyring: an OS keyring other than the macOS keychain.
	CredentialKeyring
	// CredentialEnv: an environment variable of the base environment.
	CredentialEnv
)

// credentialSourceKind labels CredentialSource in errors.
const credentialSourceKind = "credential source"

// credentialSourceNames is the wire spelling of each CredentialSource, indexed by value. Pure.
func credentialSourceNames() []string {
	return []string{"none", "keychain", "file", "keyring", "env"}
}

// String returns the wire spelling. Pure.
func (v CredentialSource) String() string {
	return enumString(v, credentialSourceKind, credentialSourceNames())
}

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v CredentialSource) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *CredentialSource) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, credentialSourceKind, credentialSourceNames())
}

// CredentialSourceValues lists every CredentialSource in order: the canonical iteration list. Pure.
func CredentialSourceValues() []CredentialSource {
	return enumValues[CredentialSource](credentialSourceNames())
}

// ParseCredentialSource parses a wire spelling. Pure.
func ParseCredentialSource(s string) (CredentialSource, bool) {
	return enumParse[CredentialSource](s, credentialSourceNames())
}

// LoginFlow is the kind of interactive login a LoginCommander builds. Secrets always go on stdin, never argv.
type LoginFlow uint8

// LoginFlow values. Order is the wire order of names; append only.
const (
	// LoginBrowser: browser OAuth flow.
	LoginBrowser LoginFlow = iota
	// LoginDevice: device-code flow.
	LoginDevice
	// LoginAPIKeyStdin: API key read from stdin.
	LoginAPIKeyStdin
	// LoginAccessTokenStdin: access token read from stdin.
	LoginAccessTokenStdin
)

// loginFlowKind labels LoginFlow in errors.
const loginFlowKind = "login flow"

// loginFlowNames is the wire spelling of each LoginFlow, indexed by value. Pure.
func loginFlowNames() []string {
	return []string{"browser", "device", "api-key-stdin", "access-token-stdin"}
}

// String returns the wire spelling. Pure.
func (v LoginFlow) String() string { return enumString(v, loginFlowKind, loginFlowNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v LoginFlow) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *LoginFlow) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, loginFlowKind, loginFlowNames())
}

// LoginFlowValues lists every LoginFlow in order: the canonical iteration list. Pure.
func LoginFlowValues() []LoginFlow { return enumValues[LoginFlow](loginFlowNames()) }

// ParseLoginFlow parses a wire spelling. Pure.
func ParseLoginFlow(s string) (LoginFlow, bool) { return enumParse[LoginFlow](s, loginFlowNames()) }

// InstructionScope is where an instruction file (CLAUDE.md, AGENTS.md) applies.
type InstructionScope uint8

// InstructionScope values. Order is the wire order of names; append only.
const (
	// InstructionManaged: machine policy.
	InstructionManaged InstructionScope = iota
	// InstructionUser: the home.
	InstructionUser
	// InstructionProject: checked into the project.
	InstructionProject
	// InstructionLocal: the project, not checked in.
	InstructionLocal
	// InstructionAuto: auto memory written by the agent.
	InstructionAuto
	// InstructionImported: pulled in by an @import.
	InstructionImported
)

// instructionScopeKind labels InstructionScope in errors.
const instructionScopeKind = "instruction scope"

// instructionScopeNames is the wire spelling of each InstructionScope, indexed by value. Pure.
func instructionScopeNames() []string {
	return []string{"managed", "user", "project", "local", "auto", "imported"}
}

// String returns the wire spelling. Pure.
func (v InstructionScope) String() string {
	return enumString(v, instructionScopeKind, instructionScopeNames())
}

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v InstructionScope) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *InstructionScope) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, instructionScopeKind, instructionScopeNames())
}

// InstructionScopeValues lists every InstructionScope in order: the canonical iteration list. Pure.
func InstructionScopeValues() []InstructionScope {
	return enumValues[InstructionScope](instructionScopeNames())
}

// ParseInstructionScope parses a wire spelling. Pure.
func ParseInstructionScope(s string) (InstructionScope, bool) {
	return enumParse[InstructionScope](s, instructionScopeNames())
}

// ConfigScope is the layer a config value, MCP server or setting comes from.
type ConfigScope uint8

// ConfigScope values. Order is the wire order of names; append only.
const (
	// ConfigScopeUnknown: not known.
	ConfigScopeUnknown ConfigScope = iota
	// ConfigManaged: machine policy.
	ConfigManaged
	// ConfigUser: the home.
	ConfigUser
	// ConfigProject: checked into the project.
	ConfigProject
	// ConfigLocal: per project, not checked in.
	ConfigLocal
	// ConfigProfile: a named vendor profile (Codex).
	ConfigProfile
	// ConfigCLI: command-line override.
	ConfigCLI
	// ConfigPlugin: contributed by a plugin.
	ConfigPlugin
)

// configScopeKind labels ConfigScope in errors.
const configScopeKind = "config scope"

// configScopeNames is the wire spelling of each ConfigScope, indexed by value. Pure.
func configScopeNames() []string {
	return []string{"unknown", "managed", "user", "project", "local", "profile", "cli", "plugin"}
}

// String returns the wire spelling. Pure.
func (v ConfigScope) String() string { return enumString(v, configScopeKind, configScopeNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v ConfigScope) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *ConfigScope) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, configScopeKind, configScopeNames())
}

// ConfigScopeValues lists every ConfigScope in order: the canonical iteration list. Pure.
func ConfigScopeValues() []ConfigScope { return enumValues[ConfigScope](configScopeNames()) }

// ParseConfigScope parses a wire spelling. Pure.
func ParseConfigScope(s string) (ConfigScope, bool) {
	return enumParse[ConfigScope](s, configScopeNames())
}

// Trust is whether the agent trusts a project folder.
type Trust uint8

// Trust values. Order is the wire order of names; append only.
const (
	// TrustUnknown: never asked or not recorded.
	TrustUnknown Trust = iota
	// TrustTrusted: trusted.
	TrustTrusted
	// TrustUntrusted: explicitly untrusted.
	TrustUntrusted
)

// trustKind labels Trust in errors.
const trustKind = "trust"

// trustNames is the wire spelling of each Trust, indexed by value. Pure.
func trustNames() []string {
	return []string{"unknown", "trusted", "untrusted"}
}

// String returns the wire spelling. Pure.
func (v Trust) String() string { return enumString(v, trustKind, trustNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v Trust) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *Trust) UnmarshalText(b []byte) error { return enumUnmarshal(v, b, trustKind, trustNames()) }

// TrustValues lists every Trust in order: the canonical iteration list. Pure.
func TrustValues() []Trust { return enumValues[Trust](trustNames()) }

// ParseTrust parses a wire spelling. Pure.
func ParseTrust(s string) (Trust, bool) { return enumParse[Trust](s, trustNames()) }

// Capability names one shared L2 capability (SPEC §6.2). Every constant has an accessor and a row in the Supported table; a test fails otherwise.
type Capability uint8

// Capability values. Order is the wire order of names; append only.
const (
	// CapInstaller: Installer · InstallerOf.
	CapInstaller Capability = iota
	// CapBinaryVersion: BinaryVersion · BinaryVersionOf (runs the binary).
	CapBinaryVersion
	// CapIdentity: IdentityReader · IdentityOf.
	CapIdentity
	// CapUsage: UsageReader · UsageOf (network).
	CapUsage
	// CapCachedUsage: CachedUsageReader · CachedUsageOf.
	CapCachedUsage
	// CapSubscription: SubscriptionReader · SubscriptionOf (network).
	CapSubscription
	// CapTokenLogin: TokenLogin · TokenLoginOf.
	CapTokenLogin
	// CapTokenProber: TokenProber · TokenProberOf (quota).
	CapTokenProber
	// CapLoginCommander: LoginCommander · LoginCommanderOf.
	CapLoginCommander
	// CapConversations: ConversationLister · ConversationsOf.
	CapConversations
	// CapHistoryChecker: HistoryChecker · HistoryCheckerOf.
	CapHistoryChecker
	// CapHistoryMover: HistoryMover · HistoryMoverOf (write).
	CapHistoryMover
	// CapConversationMover: ConversationMover · ConversationMoverOf (write).
	CapConversationMover
	// CapPromptHistory: PromptHistoryReader · PromptHistoryOf.
	CapPromptHistory
	// CapLiveSessions: LiveSessionLister · LiveSessionsOf.
	CapLiveSessions
	// CapProcessClassifier: ProcessClassifier · ProcessClassifierOf.
	CapProcessClassifier
	// CapTurns: TurnReader · TurnsOf.
	CapTurns
	// CapHomeLinker: HomeLinker · HomeLinkerOf (write).
	CapHomeLinker
	// CapInstructionFiles: InstructionFiles · InstructionFilesOf.
	CapInstructionFiles
	// CapMCPServers: MCPLister · MCPServersOf.
	CapMCPServers
	// CapSkills: SkillLister · SkillsOf.
	CapSkills
	// CapPlugins: PluginLister · PluginsOf.
	CapPlugins
	// CapConfig: ConfigReader · ConfigOf.
	CapConfig
	// CapModels: ModelLister · ModelsOf.
	CapModels
	// CapRunEvents: RunEvents · RunEventsOf.
	CapRunEvents
	// CapDoctor: Doctor · DoctorOf.
	CapDoctor
	// CapLiveCredential: LiveCredentialer · LiveCredentialOf.
	CapLiveCredential
	// CapLaunchConfig: LaunchConfigurer · LaunchConfigOf (writes a settings file when asked).
	CapLaunchConfig
)

// capabilityKind labels Capability in errors.
const capabilityKind = "capability"

// capabilityNames is the wire spelling of each Capability, indexed by value. Pure.
func capabilityNames() []string {
	return []string{"installer", "binary-version", "identity", "usage", "cached-usage", "subscription", "token-login", "token-prober", "login-commander", "conversations", "history-checker", "history-mover", "conversation-mover", "prompt-history", "live-sessions", "process-classifier", "turns", "home-linker", "instruction-files", "mcp-servers", "skills", "plugins", "config", "models", "run-events", "doctor", "live-credential", "launch-config"}
}

// String returns the wire spelling. Pure.
func (v Capability) String() string { return enumString(v, capabilityKind, capabilityNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v Capability) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *Capability) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, capabilityKind, capabilityNames())
}

// CapabilityValues lists every Capability in order: the canonical iteration list. Pure.
func CapabilityValues() []Capability { return enumValues[Capability](capabilityNames()) }

// ParseCapability parses a wire spelling. Pure.
func ParseCapability(s string) (Capability, bool) { return enumParse[Capability](s, capabilityNames()) }

// Liveness says whether a recorded process still runs (Host.ProcessAlive), for cleaning up per-run files only once their agent has ended.
type Liveness uint8

// Liveness values. Order is the wire order of names; append only.
const (
	// LivenessUnknown: could not be verified (no process table, or neither the start time nor the start env could be read); keep the run's files.
	LivenessUnknown Liveness = iota
	// LivenessAlive: the pid runs and is the recorded process (start time or start env matches).
	LivenessAlive
	// LivenessEnded: the pid is gone, or it was reused by another process (start time or start env differs).
	LivenessEnded
)

// livenessKind labels Liveness in errors.
const livenessKind = "liveness"

// livenessNames is the wire spelling of each Liveness, indexed by value. Pure.
func livenessNames() []string {
	return []string{"unknown", "alive", "ended"}
}

// String returns the wire spelling. Pure.
func (v Liveness) String() string { return enumString(v, livenessKind, livenessNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v Liveness) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *Liveness) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, livenessKind, livenessNames())
}

// LivenessValues lists every Liveness in order: the canonical iteration list. Pure.
func LivenessValues() []Liveness { return enumValues[Liveness](livenessNames()) }

// ParseLiveness parses a wire spelling. Pure.
func ParseLiveness(s string) (Liveness, bool) { return enumParse[Liveness](s, livenessNames()) }

// SettingsDelivery says how per-launch settings reach an agent that reads a settings object at launch (Claude's --settings): a file or the JSON on argv.
type SettingsDelivery uint8

// SettingsDelivery values. Order is the wire order of names; append only.
const (
	// SettingsFile: a 0600 file in the caller's run dir; its path goes on argv, so nothing in it shows in the process list (the default).
	SettingsFile SettingsDelivery = iota
	// SettingsInline: the JSON itself on argv; no file to clean up, but every key (a helper command, env values) shows in ps.
	SettingsInline
)

// settingsDeliveryKind labels SettingsDelivery in errors.
const settingsDeliveryKind = "settings delivery"

// settingsDeliveryNames is the wire spelling of each SettingsDelivery, indexed by value. Pure.
func settingsDeliveryNames() []string { return []string{"file", "inline"} }

// String returns the wire spelling. Pure.
func (v SettingsDelivery) String() string {
	return enumString(v, settingsDeliveryKind, settingsDeliveryNames())
}

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v SettingsDelivery) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *SettingsDelivery) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, settingsDeliveryKind, settingsDeliveryNames())
}

// SettingsDeliveryValues lists every SettingsDelivery in order: the canonical iteration list. Pure.
func SettingsDeliveryValues() []SettingsDelivery {
	return enumValues[SettingsDelivery](settingsDeliveryNames())
}

// ParseSettingsDelivery parses a wire spelling. Pure.
func ParseSettingsDelivery(s string) (SettingsDelivery, bool) {
	return enumParse[SettingsDelivery](s, settingsDeliveryNames())
}

// Concern is the neutral launch concern a vendor option key or settings key overlaps, so a launcher can apply "one winner per concern" without knowing vendor spellings.
type Concern uint8

// Concern values. Order is the wire order of names; append only.
const (
	// ConcernNone: the key overlaps no neutral request field.
	ConcernNone Concern = iota
	// ConcernModel: LaunchRequest.Model.
	ConcernModel
	// ConcernPrompt: LaunchRequest.Prompt.
	ConcernPrompt
	// ConcernAddDir: LaunchRequest.AddDirs.
	ConcernAddDir
	// ConcernAutonomy: LaunchRequest.Autonomy (the permission level).
	ConcernAutonomy
	// ConcernHeadless: LaunchRequest.Headless.
	ConcernHeadless
	// ConcernResume: LaunchRequest.ResumeID.
	ConcernResume
	// ConcernCredential: LaunchRequest.Credential (the login the agent uses).
	ConcernCredential
	// ConcernSettings: LaunchConfig.Settings (a raw settings value of its own).
	ConcernSettings
)

// concernKind labels Concern in errors.
const concernKind = "concern"

// concernNames is the wire spelling of each Concern, indexed by value. Pure.
func concernNames() []string {
	return []string{"none", "model", "prompt", "add-dir", "autonomy", "headless", "resume", "credential", "settings"}
}

// String returns the wire spelling. Pure.
func (v Concern) String() string { return enumString(v, concernKind, concernNames()) }

// MarshalText encodes the wire spelling, so JSON and YAML carry names, not numbers. Pure.
func (v Concern) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses the wire spelling; an unknown name is an error (closed set). Pure.
func (v *Concern) UnmarshalText(b []byte) error {
	return enumUnmarshal(v, b, concernKind, concernNames())
}

// ConcernValues lists every Concern in order: the canonical iteration list. Pure.
func ConcernValues() []Concern { return enumValues[Concern](concernNames()) }

// ParseConcern parses a wire spelling. Pure.
func ParseConcern(s string) (Concern, bool) { return enumParse[Concern](s, concernNames()) }
