package claude

import (
	"context"
	"io"
	"path/filepath"
	"sort"

	"github.com/khanakia/agentcli"
)

// neutral is the L1 + L2 view of the Claude agent: it embeds the shared
// Core (L1) and projects every rich Claude result onto the neutral types.
// The compile-time assertions below declare its capabilities (SPEC §6.2).
type neutral struct {
	*agentcli.Core
	a *Agent
}

// Capabilities Claude has (SPEC §6.2 support table).
var (
	_ agentcli.Agent               = neutral{}
	_ agentcli.Installer           = neutral{}
	_ agentcli.BinaryVersion       = neutral{}
	_ agentcli.IdentityReader      = neutral{}
	_ agentcli.UsageReader         = neutral{}
	_ agentcli.CachedUsageReader   = neutral{}
	_ agentcli.SubscriptionReader  = neutral{}
	_ agentcli.TokenLogin          = neutral{}
	_ agentcli.TokenProber         = neutral{}
	_ agentcli.LoginCommander      = neutral{}
	_ agentcli.ConversationLister  = neutral{}
	_ agentcli.HistoryChecker      = neutral{}
	_ agentcli.HistoryMover        = neutral{}
	_ agentcli.ConversationMover   = neutral{}
	_ agentcli.PromptHistoryReader = neutral{}
	_ agentcli.LiveSessionLister   = neutral{}
	_ agentcli.ProcessClassifier   = neutral{}
	_ agentcli.TurnReader          = neutral{}
	_ agentcli.HomeLinker          = neutral{}
	_ agentcli.InstructionFiles    = neutral{}
	_ agentcli.MCPLister           = neutral{}
	_ agentcli.SkillLister         = neutral{}
	_ agentcli.PluginLister        = neutral{}
	_ agentcli.ConfigReader        = neutral{}
	_ agentcli.RunEvents           = neutral{}
	_ agentcli.Doctor              = neutral{}
	_ agentcli.UnsupportedReasoner = neutral{}
)

// modelListerReason is why Claude has no ModelLister (OD-15).
const modelListerReason = "Claude Code keeps no model catalog on disk that has a recorded shape; a static list would rot (OD-15)"

// Neutral returns the L1/L2 view. cost: Pure.
func (a *Agent) Neutral() agentcli.Agent { return neutral{Core: a.core, a: a} }

// home converts a neutral home, refusing another agent's.
func (n neutral) home(h agentcli.Home) (Home, error) { return n.a.HomeOf(h) }

// project builds a Project for dir; "" means no project.
func (n neutral) project(dir string) (*Project, error) {
	if dir == "" {
		return nil, nil
	}
	p, err := n.a.Project(dir)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UnsupportedReason declares the capabilities Claude lacks. cost: Pure.
func (n neutral) UnsupportedReason(c agentcli.Capability) (string, bool) {
	if c == agentcli.CapModels {
		return modelListerReason, true
	}
	return "", false
}

// Launch maps the neutral request onto LaunchOptions. cost: Pure.
func (n neutral) Launch(t agentcli.Target, r agentcli.LaunchRequest) (agentcli.Command, error) {
	return n.a.launchRequest(t, r)
}

// Install is Disk · Read.
func (n neutral) Install() (agentcli.Install, error) {
	i, err := n.a.Install()
	return i.Install, err
}

// BinaryVersion runs `claude --version` (Process · Read; reached only
// through BinaryVersionOf, the door accessor).
func (n neutral) BinaryVersion(ctx context.Context) (agentcli.Version, error) {
	return n.a.Runner().BinaryVersion(ctx)
}

// Identity is Disk · Read + keychain.
func (n neutral) Identity(h agentcli.Home) (agentcli.Identity, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Identity{}, err
	}
	id, err := n.a.Identity(ch)
	return id.Identity, err
}

// Usage is Network · Read.
func (n neutral) Usage(ctx context.Context, h agentcli.Home) (agentcli.Usage, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Usage{}, err
	}
	u, err := n.a.Usage(ctx, ch)
	return u.Usage, err
}

// CachedUsage is Disk · Read.
func (n neutral) CachedUsage(h agentcli.Home) (agentcli.Usage, bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Usage{}, false, err
	}
	u, ok, err := n.a.CachedUsage(ch)
	return u.Usage, ok, err
}

// Subscription is Network · Read.
func (n neutral) Subscription(ctx context.Context, h agentcli.Home) (agentcli.Subscription, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Subscription{}, err
	}
	return n.a.Subscription(ctx, ch)
}

// TokenKinds is Pure.
func (n neutral) TokenKinds() []agentcli.TokenKind { return n.a.TokenKinds() }

// DetectTokenKind is Pure; unusable tokens (refresh, admin) report false.
func (n neutral) DetectTokenKind(t agentcli.Token) (agentcli.TokenKind, bool) {
	k, ok, err := n.a.DetectTokenKind(t)
	return k, ok && err == nil
}

// TokenEnv is Pure.
func (n neutral) TokenEnv(c agentcli.TokenCredential) ([]agentcli.EnvChange, error) {
	return n.a.TokenEnv(c)
}

// TokenSetupCommand is Pure.
func (n neutral) TokenSetupCommand(h agentcli.Home) (agentcli.Command, bool) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Command{}, false
	}
	return n.a.SetupTokenCommand(ch), true
}

// TokenUsage is Quota · Read (the TokenProberOf door).
func (n neutral) TokenUsage(ctx context.Context, c agentcli.TokenCredential) (agentcli.Usage, error) {
	u, err := n.a.Prober().TokenUsage(ctx, c)
	return u.Usage, err
}

// LoginCommand maps the neutral flows: browser → claude.ai login; Claude
// has no device or stdin login flow. cost: Pure.
func (n neutral) LoginCommand(h agentcli.Home, f agentcli.LoginFlow) (agentcli.Command, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Command{}, err
	}
	if f != agentcli.LoginBrowser {
		return agentcli.Command{}, agentcli.UnsupportedError(ID, "login flow "+f.String(), "Claude Code logs in through the browser; tokens go through TokenEnv")
	}
	return n.a.LoginCommand(ch, LoginClaudeAI, ""), nil
}

// LogoutCommand is Pure.
func (n neutral) LogoutCommand(h agentcli.Home) agentcli.Command {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Command{}
	}
	return n.a.LogoutCommand(ch)
}

// Conversations is Disk · Read.
func (n neutral) Conversations(ctx context.Context, h agentcli.Home, q agentcli.ConversationQuery) ([]agentcli.Conversation, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	cs, err := n.a.Conversations(ctx, ch, q)
	out := make([]agentcli.Conversation, len(cs))
	for i, c := range cs {
		out[i] = c.Conversation
	}
	return out, err
}

// Conversation is Disk · Read.
func (n neutral) Conversation(h agentcli.Home, id string) (agentcli.Conversation, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Conversation{}, err
	}
	c, err := n.a.Conversation(ch, id)
	return c.Conversation, err
}

// HasHistory is Disk · Read.
func (n neutral) HasHistory(h agentcli.Home, dir string) (bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.HasHistory(ch, dir)
}

// MoveHistory is Disk · Write (the HistoryMoverOf door).
func (n neutral) MoveHistory(h agentcli.Home, from, to string) (agentcli.Backup, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Backup{}, err
	}
	_, b, err := n.a.Writer().MoveHistory(ch, from, to)
	return b, err
}

// PlanMove is Disk · Read.
func (n neutral) PlanMove(r agentcli.MoveRequest) (agentcli.MovePlan, error) { return n.a.PlanMove(r) }

// ApplyMove is Disk · Write (the ConversationMoverOf door).
func (n neutral) ApplyMove(p agentcli.MovePlan) (agentcli.Backup, error) {
	return n.a.Writer().ApplyMove(p)
}

// PromptHistory is Disk · Read.
func (n neutral) PromptHistory(h agentcli.Home, q agentcli.PromptQuery) ([]agentcli.PromptEntry, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	es, err := n.a.PromptHistory(ch, HistoryQuery{Project: q.Dir, SessionID: q.ConversationID, Since: q.Since, Limit: q.Limit})
	out := make([]agentcli.PromptEntry, len(es))
	for i, e := range es {
		out[i] = e.neutral()
	}
	return out, err
}

// LiveSessions is Disk · Read + process table.
func (n neutral) LiveSessions(h agentcli.Home) ([]agentcli.LiveSession, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ls, err := n.a.LiveSessions(ch)
	out := make([]agentcli.LiveSession, len(ls))
	for i, l := range ls {
		out[i] = l.Neutral(ch)
	}
	return out, err
}

// IsLive is Disk · Read + process table.
func (n neutral) IsLive(h agentcli.Home, id string) (bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.IsLive(ch, id)
}

// ClassifyProcess is Pure; Claude refines Kind on top of Spec.Process.
func (n neutral) ClassifyProcess(p agentcli.Process) (agentcli.ProcessInfo, bool) {
	return n.a.ClassifyProcess(p)
}

// Turns is Disk · Read.
func (n neutral) Turns(h agentcli.Home, id string, from agentcli.Offset, fn func(agentcli.Turn) error) (agentcli.Offset, error) {
	ch, err := n.home(h)
	if err != nil {
		return from, err
	}
	return n.a.Turns(ch, id, from, fn)
}

// memoryScope maps a memory kind onto the neutral instruction scope.
func memoryScope(k MemoryKind) agentcli.InstructionScope {
	switch k {
	case MemoryManaged:
		return agentcli.InstructionManaged
	case MemoryUser, MemoryUserRule, MemoryAgentMemory:
		return agentcli.InstructionUser
	case MemoryLocal:
		return agentcli.InstructionLocal
	case MemoryAuto:
		return agentcli.InstructionAuto
	case MemoryImport:
		return agentcli.InstructionImported
	}
	return agentcli.InstructionProject
}

// InstructionFiles lists memory files in load order plus auto memory.
// cost: Disk · Read.
func (n neutral) InstructionFiles(h agentcli.Home, dir string) ([]agentcli.InstructionFile, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	p, err := n.a.Project(dir)
	if err != nil {
		return nil, err
	}
	files, err := n.a.MemoryFiles(ch, p, true)
	if err != nil {
		return nil, err
	}
	auto, err := n.a.AutoMemory(ch, p)
	if err != nil {
		return nil, err
	}
	files = append(files, auto...)
	out := make([]agentcli.InstructionFile, len(files))
	for i, f := range files {
		out[i] = agentcli.InstructionFile{Path: f.Path, Scope: memoryScope(f.Kind), Kind: string(f.Kind), Bytes: f.Size}
	}
	return out, nil
}

// mcpScope maps an MCP scope onto the neutral config scope.
func mcpScope(s MCPScope) agentcli.ConfigScope {
	switch s {
	case MCPScopeLocal:
		return agentcli.ConfigLocal
	case MCPScopeUser, MCPScopeLegacy:
		return agentcli.ConfigUser
	case MCPScopeProject:
		return agentcli.ConfigProject
	case MCPScopeManaged:
		return agentcli.ConfigManaged
	case MCPScopePlugin:
		return agentcli.ConfigPlugin
	case MCPScopeSession:
		return agentcli.ConfigCLI
	}
	return agentcli.ConfigScopeUnknown
}

// MCPServers lists servers with env names only. cost: Disk · Read.
func (n neutral) MCPServers(h agentcli.Home, dir string) ([]agentcli.MCPServer, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	p, err := n.project(dir)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.MCPServers(ch, p)
	out := make([]agentcli.MCPServer, len(ss))
	for i, s := range ss {
		names := make([]string, 0, len(s.Env))
		for k := range s.Env {
			names = append(names, k)
		}
		sort.Strings(names)
		ns := agentcli.MCPServer{Name: s.Name, Transport: agentcli.MCPTransport(s.Transport), Scope: mcpScope(s.Scope),
			Command: s.Command, Args: s.Args, URL: s.URL, EnvNames: names, Source: s.Source.Path}
		switch s.Approval {
		case ApprovalApproved:
			ns.Enabled = boolPtr(true)
		case ApprovalRejected:
			ns.Enabled = boolPtr(false)
		}
		out[i] = ns
	}
	return out, err
}

// skillScope maps a Claude skill scope onto the neutral one.
func skillScope(s SkillScope) agentcli.SkillScope {
	switch s {
	case SkillScopeUser:
		return agentcli.SkillUser
	case SkillScopeProject, SkillScopeAddDir:
		return agentcli.SkillProject
	case SkillScopePlugin:
		return agentcli.SkillPlugin
	case SkillScopeManaged:
		return agentcli.SkillManaged
	case SkillScopeBundled, SkillScopeSynced:
		return agentcli.SkillSystem
	}
	return agentcli.SkillScope(s)
}

// Skills is Disk · Read.
func (n neutral) Skills(h agentcli.Home, dir string) ([]agentcli.Skill, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	p, err := n.project(dir)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.Skills(ch, p)
	out := make([]agentcli.Skill, len(ss))
	for i, s := range ss {
		out[i] = agentcli.Skill{Name: s.Name, Description: s.Description, Dir: s.Dir, Plugin: string(s.Plugin), Scope: skillScope(s.Scope)}
	}
	return out, err
}

// Plugins is Disk · Read.
func (n neutral) Plugins(h agentcli.Home, dir string) ([]agentcli.Plugin, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	p, err := n.project(dir)
	if err != nil {
		return nil, err
	}
	ps, err := n.a.Plugins(ch, p)
	out := make([]agentcli.Plugin, len(ps))
	for i, pl := range ps {
		name, market := pl.ID.Split()
		out[i] = agentcli.Plugin{Name: name, Marketplace: market, Version: pl.Version, Dir: pl.InstallPath, Enabled: pl.Enabled || pl.ProjectEnabled}
	}
	return out, err
}

// settingsConfigScope maps a settings layer onto the neutral scope.
func settingsConfigScope(s SettingsScope) agentcli.ConfigScope {
	switch s {
	case ScopeManaged:
		return agentcli.ConfigManaged
	case ScopeCLI:
		return agentcli.ConfigCLI
	case ScopeLocal:
		return agentcli.ConfigLocal
	case ScopeProject:
		return agentcli.ConfigProject
	case ScopeUser:
		return agentcli.ConfigUser
	}
	return agentcli.ConfigScopeUnknown
}

// ConfigLayers lists the settings files that apply, highest first, with
// presence. cost: Disk · Read.
func (n neutral) ConfigLayers(h agentcli.Home, dir string) ([]agentcli.ConfigLayer, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	p, err := n.project(dir)
	if err != nil {
		return nil, err
	}
	eff, err := n.a.Settings(ch, p, nil)
	out := make([]agentcli.ConfigLayer, 0, len(eff.Layers))
	for _, l := range eff.Layers {
		out = append(out, agentcli.ConfigLayer{Scope: settingsConfigScope(l.Scope), Path: l.Source.Path, Exists: l.Present})
	}
	return out, err
}

// ProjectTrust reads hasTrustDialogAccepted. cost: Disk · Read.
func (n neutral) ProjectTrust(h agentcli.Home, dir string) (agentcli.Trust, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.TrustUnknown, err
	}
	ps, err := n.a.ProjectState(ch, filepath.Clean(dir))
	if err != nil || ps == nil {
		return agentcli.TrustUnknown, err
	}
	if ps.TrustAccepted {
		return agentcli.TrustTrusted, nil
	}
	return agentcli.TrustUntrusted, nil
}

// autonomyOf maps a permission mode onto the neutral level; modes with no
// exact level (auto, dontAsk) stay AutonomyDefault.
func autonomyOf(m PermissionMode) agentcli.Autonomy {
	switch m {
	case ModePlan:
		return agentcli.AutonomyReadOnly
	case ModeManual:
		return agentcli.AutonomyAsk
	case ModeAcceptEdits:
		return agentcli.AutonomyAcceptEdits
	case ModeBypassPermissions:
		return agentcli.AutonomyFull
	}
	return agentcli.AutonomyDefault
}

// Defaults reads the merged model and permission mode. cost: Disk · Read.
func (n neutral) Defaults(h agentcli.Home, dir string) (agentcli.Defaults, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Defaults{}, err
	}
	p, err := n.project(dir)
	if err != nil {
		return agentcli.Defaults{}, err
	}
	eff, err := n.a.Settings(ch, p, nil)
	d := agentcli.Defaults{Model: eff.Merged.Model}
	if eff.Merged.Permissions != nil {
		d.Autonomy = autonomyOf(eff.Merged.Permissions.DefaultMode)
	}
	return d, err
}

// NewRunDecoder decodes a stream-json run (Type + docs-shaped Text/Tokens,
// Raw always; OD-11). cost: Pure.
func (n neutral) NewRunDecoder(r io.Reader) agentcli.RunDecoder { return newRunDecoder(r) }

// Checks runs the L1 checks plus Claude's disk checks. cost: Disk · Read.
func (n neutral) Checks(ctx context.Context, h agentcli.Home) []agentcli.Check {
	ch, err := n.home(h)
	if err != nil {
		return []agentcli.Check{{ID: agentcli.CheckHome, Status: agentcli.CheckFail, Message: err.Error()}}
	}
	return n.a.Doctor(ctx, ch, nil, DoctorOptions{})
}
