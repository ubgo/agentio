package agentio

import (
	"context"
	"io"
	"time"
)

// This file holds every shared L2 capability interface and its neutral
// types (SPEC §6.3). Signatures take agentio.Home; a home of another agent
// returns ErrForeignHome. Each method's cost is in its doc comment and the
// worst case per capability is in Capability.Cost.

// InstallMethod is how the binary was installed. Open set: unknown values
// are kept (IsKnown false).
type InstallMethod string

// InstallMethod values seen in vendor metadata.
const (
	InstallNPM        InstallMethod = "npm"
	InstallBun        InstallMethod = "bun"
	InstallPNPM       InstallMethod = "pnpm"
	InstallBrew       InstallMethod = "brew"
	InstallNative     InstallMethod = "native"
	InstallStandalone InstallMethod = "standalone"
	// InstallVolta is a global install through Volta (a ~/.volta/bin or
	// %LOCALAPPDATA%\Volta\bin shim; the package lives in Volta's image dir).
	InstallVolta   InstallMethod = "volta"
	InstallUnknown InstallMethod = "unknown"
)

// InstallMethodValues is the canonical list of known methods. Pure.
func InstallMethodValues() []InstallMethod {
	return []InstallMethod{InstallNPM, InstallBun, InstallPNPM, InstallBrew, InstallNative, InstallStandalone, InstallVolta, InstallUnknown}
}

// IsKnown reports a named method. Pure.
func (m InstallMethod) IsKnown() bool { return isKnown(m, InstallMethodValues()) }

// isKnown is the shared IsKnown body for open string sets.
func isKnown[S ~string](v S, known []S) bool {
	for _, k := range known {
		if k == v {
			return true
		}
	}
	return false
}

// Install is what is on disk for the binary.
type Install struct {
	// Path is the PATH hit; RealPath has symlinks resolved.
	Path, RealPath string
	Method         InstallMethod
	// Version comes from package metadata on disk; zero when not on disk.
	Version Version
	// Shadowed lists other PATH entries with the same name.
	Shadowed []string
}

// Installer reads the install from disk.
type Installer interface {
	// Install is Disk · Read; never runs the binary.
	Install() (Install, error)
}

// BinaryVersion runs `<binary> --version` (Process · Read; may touch the
// login for vendors whose subcommands refresh tokens, so it has its own
// accessor door).
type BinaryVersion interface {
	BinaryVersion(ctx context.Context) (Version, error)
}

// Identity is what is known about a home's login without the network.
type Identity struct {
	LoggedIn bool
	// Email and Org are personal: redacted in every output so a log or
	// JSON dump never leaks who is logged in.
	Email, Org Personal
	// PlanLabel is a display label ("Max 20x").
	PlanLabel  string
	Billing    Billing
	Credential CredentialSource
	// ExpiresAt is the access credential's expiry; nil when unknown or none.
	ExpiresAt *time.Time
}

// Expired reports a known expiry at or before now. Pure.
func (i Identity) Expired(now time.Time) bool {
	return i.ExpiresAt != nil && !i.ExpiresAt.After(now)
}

// IdentityReader reads login state locally.
type IdentityReader interface {
	// Identity is Disk · Read (+ keychain Process · Read for Claude); never
	// a network call (conformance-tested).
	Identity(h Home) (Identity, error)
}

// Window is one usage limit.
type Window struct {
	// Length is the window length; 0 when the vendor does not say.
	Length time.Duration
	// Scope is the model or surface it is limited to; "" = all usage.
	Scope   string
	Label   string
	Percent float64
	// ResetsAt is nil when the window has no scheduled reset.
	ResetsAt *time.Time
	// VendorKind is the vendor's raw kind ("five_hour", "primary_window").
	VendorKind string
}

// Window class boundaries.
const (
	day   = 24 * time.Hour
	week  = 7 * day
	month = 28 * day
)

// Class buckets the window by Length; derived, never stored. Pure.
func (w Window) Class() WindowClass {
	switch {
	case w.Length <= 0:
		return WindowUnknown
	case w.Length < day:
		return WindowShort
	case w.Length < week:
		return WindowDay
	case w.Length < month:
		return WindowWeek
	}
	return WindowMonth
}

// Usage is one account's usage snapshot.
type Usage struct {
	Agent ID
	// Billing PerToken means no windows exist; an answer, not a failure.
	Billing Billing
	Windows []Window
	// ObservedAt is server time, or record/fetch time for cached readings.
	ObservedAt time.Time
	Source     UsageSource
	// Raw is the vendor body, for L3 decoders.
	Raw Raw
}

// MaxPercent is the highest percent across windows (any window at 100%
// blocks the account); 0 for no windows. Pure.
func (u Usage) MaxPercent() float64 {
	highest := 0.0
	for _, w := range u.Windows {
		highest = max(highest, w.Percent)
	}
	return highest
}

// UsageReader fetches live usage: Network · Read, may 429; once, never polled.
type UsageReader interface {
	Usage(ctx context.Context, h Home) (Usage, error)
}

// CachedUsageReader reads usage the vendor left on disk: Disk · Read, safe in a loop.
type CachedUsageReader interface {
	CachedUsage(h Home) (Usage, bool, error)
}

// CachedUsageNoter is an optional extension of CachedUsageReader: why a
// home may have no cached reading, or only an old one, for agents whose
// recent versions stopped writing it (Claude Code no longer refreshes
// cachedUsageUtilization). A caller shows the note with a missing or old
// reading instead of a generic "run the agent once". Pure.
type CachedUsageNoter interface {
	CachedUsageNote() string
}

// Subscription is what a vendor reports about an account's plan.
type Subscription struct {
	// Plan is a display label ("Max (20x)").
	Plan string
	// Status is the vendor's status ("active", "past_due").
	Status string
	// BillingType is the vendor's billing type ("stripe_subscription").
	BillingType string
	// StartedAt is nil when unknown.
	StartedAt *time.Time
	Raw       Raw
}

// SubscriptionReader reads the plan: Network · Read (Codex: Disk first).
type SubscriptionReader interface {
	Subscription(ctx context.Context, h Home) (Subscription, error)
}

// TokenLogin maps a caller-supplied token onto the vendor's variables.
type TokenLogin interface {
	// TokenKinds lists supported kinds; the first is the default. Pure.
	TokenKinds() []TokenKind
	// DetectTokenKind recognises a kind from the token's shape. Pure.
	DetectTokenKind(t Token) (TokenKind, bool)
	// TokenEnv returns the env changes that make the vendor CLI use c
	// instead of the home login; scrubbing Spec.CredentialEnv is done by
	// the launch builder. Pure.
	TokenEnv(c TokenCredential) ([]EnvChange, error)
	// TokenSetupCommand builds the vendor's token setup command. Pure.
	TokenSetupCommand(h Home) (Command, bool)
}

// TokenProber probes a token's usage: Quota · Read; never loop.
type TokenProber interface {
	TokenUsage(ctx context.Context, c TokenCredential) (Usage, error)
}

// LoginCommander builds interactive login/logout commands (data; the caller runs them).
type LoginCommander interface {
	// LoginCommand is Pure; an unsupported flow returns ErrUnsupported.
	LoginCommand(h Home, f LoginFlow) (Command, error)
	// LogoutCommand is Pure.
	LogoutCommand(h Home) Command
}

// ConversationQuery narrows a listing.
type ConversationQuery struct {
	// Dir limits to conversations started in this exact directory.
	Dir string
	// Since limits to conversations updated at or after it; zero = no limit.
	Since time.Time
	// Limit caps results (newest first); <= 0 = no cap.
	Limit int
}

// Conversation is one resumable transcript.
type Conversation struct {
	Agent            ID
	Home             Home
	ID, Title, Dir   string
	Path             string
	Created, Updated time.Time
}

// ConversationLister lists conversations.
type ConversationLister interface {
	// Conversations is Disk · Read; newest first.
	Conversations(ctx context.Context, h Home, q ConversationQuery) ([]Conversation, error)
	// Conversation is Disk · Read; ErrNotFound when absent.
	Conversation(h Home, id string) (Conversation, error)
}

// HistoryChecker answers "any conversation started in dir?" cheaply.
type HistoryChecker interface {
	// HasHistory is Disk · Read; never reads transcripts.
	HasHistory(h Home, dir string) (bool, error)
}

// HistoryMover re-keys a folder's history after the folder moved.
type HistoryMover interface {
	// MoveHistory is Disk · Write; backs up first. A folder with no history
	// is HistoryMove{Moved: false} with a nil error, never an empty Backup
	// that looks like a move.
	MoveHistory(h Home, from, to string) (HistoryMove, error)
}

// HistoryMove is the result of HistoryMover.MoveHistory.
type HistoryMove struct {
	// Moved is false when the folder had no history (nothing to re-key).
	Moved bool
	// Backup is the copy taken before the move; empty when Moved is false.
	Backup Backup
}

// ConversationMover moves conversations between homes of one agent.
type ConversationMover interface {
	// PlanMove is Disk · Read: pure inspection.
	PlanMove(r MoveRequest) (MovePlan, error)
	// ApplyMove is Disk · Write; every write returns its backup (invariant 11).
	ApplyMove(p MovePlan) (Backup, error)
}

// HomeLink is one planned symlink To -> From.
type HomeLink struct {
	// Agent owns both homes; ApplyHomeLinks refuses a plan of another
	// agent with ErrForeignHome, as ApplyMove does for a MovePlan.
	Agent ID
	Name  string
	// From is the entry in the default home; To is the path in the new home.
	From, To string
	// Exists means To is already present; it is never overwritten.
	Exists bool
}

// HomeLinker links shared files of the default home into a new home.
type HomeLinker interface {
	// PlanHomeLinks is Disk · Read, over Spec.SharedHomeFiles.
	PlanHomeLinks(def, h Home) ([]HomeLink, error)
	// ApplyHomeLinks is Disk · Write: symlinks only, skips Exists, nothing to back up.
	ApplyHomeLinks(links []HomeLink) error
}

// PromptQuery narrows prompt history.
type PromptQuery struct {
	// Dir limits to prompts typed in this folder (agents that record it).
	Dir string
	// ConversationID limits to one conversation.
	ConversationID string
	// Since limits to prompts at or after it.
	Since time.Time
	// Limit caps results (newest first); <= 0 = no cap.
	Limit int
}

// PromptEntry is one typed prompt.
type PromptEntry struct {
	Time                time.Time
	Text                string
	Dir, ConversationID string
}

// PromptHistoryReader reads typed prompt history.
type PromptHistoryReader interface {
	// PromptHistory is Disk · Read; reverse scan from the tail.
	PromptHistory(h Home, q PromptQuery) ([]PromptEntry, error)
}

// SessionKind is the vendor's kind of live session. Open set.
type SessionKind string

// SessionKind values seen.
const (
	SessionInteractive SessionKind = "interactive"
	SessionBackground  SessionKind = "bg"
)

// SessionKindValues lists known kinds. Pure.
func SessionKindValues() []SessionKind { return []SessionKind{SessionInteractive, SessionBackground} }

// IsKnown reports a named kind. Pure.
func (k SessionKind) IsKnown() bool { return isKnown(k, SessionKindValues()) }

// LiveSession is one running conversation process.
type LiveSession struct {
	PID            int
	ConversationID string
	Dir            string
	Home           Home
	Kind           SessionKind
	Status         SessionStatus
	// StatusSince is when Status last changed; zero when the vendor has none.
	StatusSince time.Time
	StartedAt   time.Time
	// Alive means the pid is present with the same start time (pid-reuse guard).
	Alive bool
}

// LiveSessionLister lists live sessions of a home.
type LiveSessionLister interface {
	// LiveSessions is Disk · Read + process table.
	LiveSessions(h Home) ([]LiveSession, error)
	// IsLive is Disk · Read + process table.
	IsLive(h Home, conversationID string) (bool, error)
}

// ProcessKind is the kind of agent process. Open set.
type ProcessKind string

// ProcessKind values.
const (
	ProcessCLI       ProcessKind = "cli"
	ProcessExec      ProcessKind = "exec"
	ProcessAppServer ProcessKind = "app-server"
	ProcessMCPServer ProcessKind = "mcp-server"
	ProcessDesktop   ProcessKind = "desktop"
	ProcessBridge    ProcessKind = "bridge"
	ProcessLauncher  ProcessKind = "launcher"
	// ProcessCommand is a short-lived subcommand that is not a
	// conversation (`claude mcp list`, `codex mcp add`): never a session.
	ProcessCommand ProcessKind = "command"
)

// ProcessKindValues lists known kinds. Pure.
func ProcessKindValues() []ProcessKind {
	return []ProcessKind{ProcessCLI, ProcessExec, ProcessAppServer, ProcessMCPServer, ProcessDesktop, ProcessBridge, ProcessLauncher, ProcessCommand}
}

// IsKnown reports a named kind. Pure.
func (k ProcessKind) IsKnown() bool { return isKnown(k, ProcessKindValues()) }

// ProcessInfo is what a classifier says about a process row.
type ProcessInfo struct {
	Agent ID
	Kind  ProcessKind
	// Session means a conversation process (reapable).
	Session bool
}

// ProcessClassifier recognises a process row as this agent: Pure, on rows
// the caller already has.
type ProcessClassifier interface {
	ClassifyProcess(p Process) (ProcessInfo, bool)
}

// Offset is a resume cursor for an incremental read (TurnReader,
// TurnFileReader): store the value a read returns and pass it back
// unchanged. It is opaque and agent-defined: for a JSONL transcript it is
// the byte offset just after the last record that was fully emitted (which
// can be before a complete line, when a turn's usage arrives on a later
// line), for a database-backed store a row cursor. Never compute one, and
// never compare cursors of different files. 0 starts at the beginning.
type Offset int64

// TokenCounts is a neutral token tally (also RunEvent.Tokens).
type TokenCounts struct {
	Input, CachedInput, CacheWrite, Output, Reasoning int64
}

// Add returns the field-wise sum. Pure.
func (t TokenCounts) Add(o TokenCounts) TokenCounts {
	return TokenCounts{
		Input: t.Input + o.Input, CachedInput: t.CachedInput + o.CachedInput,
		CacheWrite: t.CacheWrite + o.CacheWrite, Output: t.Output + o.Output, Reasoning: t.Reasoning + o.Reasoning,
	}
}

// Turn is one normalized turn per side.
type Turn struct {
	// ID is the vendor's stable id for this message (Claude record uuid,
	// Codex item id, Gemini message id, pi entry id, opencode message id),
	// so a caller can dedup a message an agent rewrites or re-appends;
	// "" when the format has none.
	ID string
	// Interim marks an assistant message the agent showed while still
	// working (Codex phase "commentary"), not its answer to the prompt. A
	// caller that stores answers skips Interim turns; a viewer shows them.
	Interim                                bool
	Role                                   Role
	Time                                   time.Time
	ConversationID, Dir, Model, StopReason string
	AgentVersion, Text                     string
	Tokens                                 *TokenCounts
	Raw                                    Raw
}

// TurnReader reads turns incrementally.
type TurnReader interface {
	// Turns is Disk · Read: from `from` to the last complete line, calls fn
	// per turn, returns the new offset; never advances past an incomplete
	// trailing line.
	Turns(h Home, conversationID string, from Offset, fn func(Turn) error) (Offset, error)
}

// InstructionFile is one instruction file in load order.
type InstructionFile struct {
	Path  string
	Scope InstructionScope
	// Kind is the vendor's file kind ("CLAUDE.md", "rule", "AGENTS.override.md").
	Kind  string
	Bytes int64
}

// InstructionFiles lists instruction files that apply in dir.
type InstructionFiles interface {
	// InstructionFiles is Disk · Read.
	InstructionFiles(h Home, dir string) ([]InstructionFile, error)
}

// MCPTransport is an MCP server transport. Open set.
type MCPTransport string

// MCPTransport values.
const (
	MCPStdio MCPTransport = "stdio"
	MCPHTTP  MCPTransport = "http"
	MCPSSE   MCPTransport = "sse"
	MCPWS    MCPTransport = "ws"
)

// MCPTransportValues lists known transports. Pure.
func MCPTransportValues() []MCPTransport { return []MCPTransport{MCPStdio, MCPHTTP, MCPSSE, MCPWS} }

// IsKnown reports a named transport. Pure.
func (t MCPTransport) IsKnown() bool { return isKnown(t, MCPTransportValues()) }

// MCPServer is one configured MCP server; env values never appear here.
type MCPServer struct {
	Name      string
	Transport MCPTransport
	Scope     ConfigScope
	Command   string
	Args      []string
	URL       string
	// EnvNames are names only.
	EnvNames []string
	// Enabled is nil when the vendor does not say.
	Enabled *bool
	// Source is the file it came from.
	Source string
}

// MCPLister lists MCP servers; never spawns one.
type MCPLister interface {
	// MCPServers is Disk · Read.
	MCPServers(h Home, dir string) ([]MCPServer, error)
}

// SkillScope is where a skill comes from. Open set.
type SkillScope string

// SkillScope values.
const (
	SkillUser    SkillScope = "user"
	SkillProject SkillScope = "project"
	SkillPlugin  SkillScope = "plugin"
	SkillManaged SkillScope = "managed"
	SkillSystem  SkillScope = "system"
)

// SkillScopeValues lists known scopes. Pure.
func SkillScopeValues() []SkillScope {
	return []SkillScope{SkillUser, SkillProject, SkillPlugin, SkillManaged, SkillSystem}
}

// IsKnown reports a named scope. Pure.
func (s SkillScope) IsKnown() bool { return isKnown(s, SkillScopeValues()) }

// Skill is one SKILL.md (frontmatter only).
type Skill struct {
	Name, Description, Dir string
	// Plugin is the plugin providing it; "" for none.
	Plugin  string
	Scope   SkillScope
	Enabled *bool
}

// SkillLister lists skills.
type SkillLister interface {
	// Skills is Disk · Read.
	Skills(h Home, dir string) ([]Skill, error)
}

// Plugin is one installed plugin.
type Plugin struct {
	Name, Marketplace, Version, Dir string
	Enabled                         bool
}

// PluginLister lists plugins.
type PluginLister interface {
	// Plugins is Disk · Read.
	Plugins(h Home, dir string) ([]Plugin, error)
}

// ConfigLayer is one config file that applies.
type ConfigLayer struct {
	Scope  ConfigScope
	Path   string
	Exists bool
}

// Defaults are the effective launch defaults.
type Defaults struct {
	// Model is "" when the vendor default applies.
	Model string
	// Autonomy is AutonomyDefault when not configured or not mappable.
	Autonomy Autonomy
}

// ConfigReader reads the facts both vendors' configs share.
type ConfigReader interface {
	// ConfigLayers lists the config files that apply to h and dir, highest
	// precedence first, for every vendor: Claude managed, CLI, local,
	// project, user; Codex legacy managed, session (-c), project (deepest
	// first), profile, user, system. Why highest first: answering "which
	// file sets this value" scans from the top and stops at the first layer
	// that has it, and both vendors document their precedence in this
	// order. Merge by applying the slice last to first. Disk · Read.
	ConfigLayers(h Home, dir string) ([]ConfigLayer, error)
	// ProjectTrust is Disk · Read.
	ProjectTrust(h Home, dir string) (Trust, error)
	// Defaults is Disk · Read.
	Defaults(h Home, dir string) (Defaults, error)
}

// Model is one entry of a vendor model catalog.
type Model struct {
	Slug, DisplayName string
	Visible           bool
	ContextWindow     int64
}

// ModelLister lists models from an on-disk catalog.
type ModelLister interface {
	// Models is Disk · Read.
	Models(h Home) ([]Model, error)
}

// RunEvent is one event of a headless run stream.
type RunEvent struct {
	Type           RunEventType
	ConversationID string
	Text           string
	Tokens         *TokenCounts
	// Raw is always the full vendor line.
	Raw Raw
}

// RunDecoder yields events; io.EOF ends the stream.
type RunDecoder interface {
	Next() (RunEvent, error)
}

// RunEvents builds a decoder for a stream the caller started with
// LaunchRequest.Headless: Pure (reads only r).
type RunEvents interface {
	NewRunDecoder(r io.Reader) RunDecoder
}

// CheckID names a doctor check. Open set: vendors add their own.
type CheckID string

// Shared CheckIDs.
const (
	CheckBinary        CheckID = "binary-found"
	CheckHome          CheckID = "home"
	CheckHomeEnv       CheckID = "home-env"
	CheckAPIBillingEnv CheckID = "api-billing-env"
	CheckVersionTested CheckID = "version-tested"
	CheckLogin         CheckID = "login"
)

// CheckIDValues lists the shared IDs. Pure.
func CheckIDValues() []CheckID {
	return []CheckID{CheckBinary, CheckHome, CheckHomeEnv, CheckAPIBillingEnv, CheckVersionTested, CheckLogin}
}

// IsKnown reports a shared ID. Pure.
func (c CheckID) IsKnown() bool { return isKnown(c, CheckIDValues()) }

// Check is one doctor result.
type Check struct {
	ID      CheckID
	Status  CheckStatus
	Message string
	// Fix is a command that would fix it; nil when none.
	Fix *Command
}

// Doctor runs disk-only checks; checks that need the binary live on the
// vendor Runner.
type Doctor interface {
	// Checks is Disk · Read.
	Checks(ctx context.Context, h Home) []Check
}

// TokenProviderLister is an optional extension of TokenLogin for agents
// whose token variable depends on the model provider (pi: anthropic ->
// ANTHROPIC_API_KEY): the providers a login of kind k can name in
// TokenCredential.Provider. An agent without it needs no provider.
type TokenProviderLister interface {
	// TokenProviders is Pure; the first is a sensible default.
	TokenProviders(k TokenKind) []string
}

// LiveCredentialPlan is what a launch adds so the running agent fetches
// its credential again, every Refresh, by running a command the caller
// owns. The caller can then switch a running session's login by changing
// what the command prints, without restarting the agent. The plan names
// no secret: the command prints it to the agent alone.
//
// The command must always print one usable credential line and exit 0. An
// agent may treat an empty output or a non-zero exit as fatal: Claude Code
// 2.1.327 runs the helper three times, then exits 1, and never falls back
// to the home's own login. A caller that cannot resolve the credential
// must fail loudly (one stderr line, non-zero exit), never print an empty
// value.
type LiveCredentialPlan struct {
	// Settings is a JSON object to merge into the launch's settings
	// (Claude: {"apiKeyHelper": "<command>"}); empty when the agent takes
	// the command elsewhere.
	Settings Raw
	// Env are process env changes the plan needs (Claude: the helper's
	// refresh interval, CLAUDE_CODE_API_KEY_HELPER_TTL_MS).
	Env []EnvChange
	// Scrub are the variables that would outrank the command; the launch
	// must remove them, or the command is never asked (Claude:
	// ANTHROPIC_AUTH_TOKEN and ANTHROPIC_API_KEY, plus
	// CLAUDE_CODE_OAUTH_TOKEN so only one login is in play).
	Scrub []string
	// Refresh is the interval the agent re-runs the command at.
	Refresh time.Duration
}

// LiveCredentialer plans a launch whose credential comes from a command
// the agent re-runs (an agent without one has a typed unsupported reason
// instead). The agent runs command itself; nothing here runs it.
type LiveCredentialer interface {
	// LiveCredential is Pure: command is the argv-free shell string the
	// agent runs; refresh 0 = the plan's default interval.
	LiveCredential(command string, refresh time.Duration) (LiveCredentialPlan, error)
}
