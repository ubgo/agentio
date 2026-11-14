package claude

import (
	"context"
	"io"
	"path/filepath"
	"sort"

	"github.com/ubgo/agentio"
)

// neutral is the L1 + L2 view of the Claude agent: it embeds the shared
// Core (L1) and projects every rich Claude result onto the neutral types.
// The compile-time assertions below declare its capabilities (SPEC §6.2).
type neutral struct {
	*agentio.Core
	a *Agent
}

// Capabilities Claude has (SPEC §6.2 support table).
var (
	_ agentio.Agent               = neutral{}
	_ agentio.Installer           = neutral{}
	_ agentio.BinaryVersion       = neutral{}
	_ agentio.IdentityReader      = neutral{}
	_ agentio.UsageReader         = neutral{}
	_ agentio.CachedUsageReader   = neutral{}
	_ agentio.SubscriptionReader  = neutral{}
	_ agentio.TokenLogin          = neutral{}
	_ agentio.TokenProber         = neutral{}
	_ agentio.LoginCommander      = neutral{}
	_ agentio.ConversationLister  = neutral{}
	_ agentio.HistoryChecker      = neutral{}
	_ agentio.HistoryMover        = neutral{}
	_ agentio.ConversationMover   = neutral{}
	_ agentio.PromptHistoryReader = neutral{}
	_ agentio.LiveSessionLister   = neutral{}
	_ agentio.ProcessClassifier   = neutral{}
	_ agentio.TurnReader          = neutral{}
	_ agentio.HomeLinker          = neutral{}
	_ agentio.InstructionFiles    = neutral{}
	_ agentio.MCPLister           = neutral{}
	_ agentio.SkillLister         = neutral{}
	_ agentio.PluginLister        = neutral{}
	_ agentio.ConfigReader        = neutral{}
	_ agentio.RunEvents           = neutral{}
	_ agentio.Doctor              = neutral{}
	_ agentio.UnsupportedReasoner = neutral{}
)

// modelListerReason is why Claude has no ModelLister (OD-15).
const modelListerReason = "Claude Code keeps no model catalog on disk that has a recorded shape; a static list would rot"

// Neutral returns the L1/L2 view. cost: Pure.
func (a *Agent) Neutral() agentio.Agent { return neutral{Core: a.core, a: a} }

// home converts a neutral home, refusing another agent's.
func (n neutral) home(h agentio.Home) (Home, error) { return n.a.HomeOf(h) }

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
func (n neutral) UnsupportedReason(c agentio.Capability) (string, bool) {
	if c == agentio.CapModels {
		return modelListerReason, true
	}
	return "", false
}

// Launch maps the neutral request onto LaunchOptions. cost: Pure.
func (n neutral) Launch(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	return n.a.launchRequest(t, r)
}

// Install is Disk · Read.
func (n neutral) Install() (agentio.Install, error) {
	i, err := n.a.Install()
	return i.Install, err
}

// BinaryVersion runs `claude --version` (Process · Read; reached only
// through BinaryVersionOf, the door accessor).
func (n neutral) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	return n.a.Runner().BinaryVersion(ctx)
}

// Identity is Disk · Read + keychain.
func (n neutral) Identity(h agentio.Home) (agentio.Identity, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Identity{}, err
	}
	id, err := n.a.Identity(ch)
	return id.Identity, err
}

// Usage is Network · Read.
func (n neutral) Usage(ctx context.Context, h agentio.Home) (agentio.Usage, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Usage{}, err
	}
	u, err := n.a.Usage(ctx, ch)
	return u.Usage, err
}

// CachedUsage is Disk · Read.
func (n neutral) CachedUsage(h agentio.Home) (agentio.Usage, bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Usage{}, false, err
	}
	u, ok, err := n.a.CachedUsage(ch)
	return u.Usage, ok, err
}

// Subscription is Network · Read.
func (n neutral) Subscription(ctx context.Context, h agentio.Home) (agentio.Subscription, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Subscription{}, err
	}
	return n.a.Subscription(ctx, ch)
}

// TokenKinds is Pure.
func (n neutral) TokenKinds() []agentio.TokenKind { return n.a.TokenKinds() }

// DetectTokenKind is Pure; unusable tokens (refresh, admin) report false.
func (n neutral) DetectTokenKind(t agentio.Token) (agentio.TokenKind, bool) {
	k, ok, err := n.a.DetectTokenKind(t)
	return k, ok && err == nil
}

// TokenEnv is Pure.
func (n neutral) TokenEnv(c agentio.TokenCredential) ([]agentio.EnvChange, error) {
	return n.a.TokenEnv(c)
}

// TokenSetupCommand is Pure.
func (n neutral) TokenSetupCommand(h agentio.Home) (agentio.Command, bool) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Command{}, false
	}
	return n.a.SetupTokenCommand(ch), true
}

// TokenUsage is Quota · Read (the TokenProberOf door).
func (n neutral) TokenUsage(ctx context.Context, c agentio.TokenCredential) (agentio.Usage, error) {
	u, err := n.a.Prober().TokenUsage(ctx, c)
	return u.Usage, err
}

// LoginCommand maps the neutral flows: browser → claude.ai login; Claude
// has no device or stdin login flow. cost: Pure.
func (n neutral) LoginCommand(h agentio.Home, f agentio.LoginFlow) (agentio.Command, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Command{}, err
	}
	if f != agentio.LoginBrowser {
		return agentio.Command{}, agentio.UnsupportedError(ID, "login flow "+f.String(), "Claude Code logs in through the browser; tokens go through TokenEnv")
	}
	return n.a.LoginCommand(ch, LoginClaudeAI, ""), nil
}

// LogoutCommand is Pure.
func (n neutral) LogoutCommand(h agentio.Home) agentio.Command {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Command{}
	}
	return n.a.LogoutCommand(ch)
}

// Conversations is Disk · Read.
func (n neutral) Conversations(ctx context.Context, h agentio.Home, q agentio.ConversationQuery) ([]agentio.Conversation, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	cs, err := n.a.Conversations(ctx, ch, q)
	out := make([]agentio.Conversation, len(cs))
	for i, c := range cs {
		out[i] = c.Conversation
	}
	return out, err
}

// Conversation is Disk · Read.
func (n neutral) Conversation(h agentio.Home, id string) (agentio.Conversation, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Conversation{}, err
	}
	c, err := n.a.Conversation(ch, id)
	return c.Conversation, err
}

// HasHistory is Disk · Read.
func (n neutral) HasHistory(h agentio.Home, dir string) (bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.HasHistory(ch, dir)
}

// MoveHistory is Disk · Write (the HistoryMoverOf door).
func (n neutral) MoveHistory(h agentio.Home, from, to string) (agentio.HistoryMove, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.HistoryMove{}, err
	}
	return n.a.Writer().MoveHistory(ch, from, to)
}

// PlanMove is Disk · Read.
func (n neutral) PlanMove(r agentio.MoveRequest) (agentio.MovePlan, error) { return n.a.PlanMove(r) }

// ApplyMove is Disk · Write (the ConversationMoverOf door).
func (n neutral) ApplyMove(p agentio.MovePlan) (agentio.Backup, error) {
	return n.a.Writer().ApplyMove(p)
}

// PromptHistory is Disk · Read.
func (n neutral) PromptHistory(h agentio.Home, q agentio.PromptQuery) ([]agentio.PromptEntry, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	es, err := n.a.PromptHistory(ch, HistoryQuery{Project: q.Dir, SessionID: q.ConversationID, Since: q.Since, Limit: q.Limit})
	out := make([]agentio.PromptEntry, len(es))
	for i, e := range es {
		out[i] = e.neutral()
	}
	return out, err
}

// LiveSessions is Disk · Read + process table.
func (n neutral) LiveSessions(h agentio.Home) ([]agentio.LiveSession, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ls, err := n.a.LiveSessions(ch)
	out := make([]agentio.LiveSession, len(ls))
	for i, l := range ls {
		out[i] = l.Neutral(ch)
	}
	return out, err
}

// IsLive is Disk · Read + process table.
func (n neutral) IsLive(h agentio.Home, id string) (bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.IsLive(ch, id)
}

// ClassifyProcess is Pure; Claude refines Kind on top of Spec.Process.
func (n neutral) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	return n.a.ClassifyProcess(p)
}

// Turns is Disk · Read.
func (n neutral) Turns(h agentio.Home, id string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	ch, err := n.home(h)
	if err != nil {
		return from, err
	}
	return n.a.Turns(ch, id, from, fn)
}

// memoryScope maps a memory kind onto the neutral instruction scope.
func memoryScope(k MemoryKind) agentio.InstructionScope {
	switch k {
	case MemoryManaged:
		return agentio.InstructionManaged
	case MemoryUser, MemoryUserRule, MemoryAgentMemory:
		return agentio.InstructionUser
	case MemoryLocal:
		return agentio.InstructionLocal
	case MemoryAuto:
		return agentio.InstructionAuto
	case MemoryImport:
		return agentio.InstructionImported
	}
	return agentio.InstructionProject
}

// InstructionFiles lists memory files in load order plus auto memory.
// cost: Disk · Read.
func (n neutral) InstructionFiles(h agentio.Home, dir string) ([]agentio.InstructionFile, error) {
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
	out := make([]agentio.InstructionFile, len(files))
	for i, f := range files {
		out[i] = agentio.InstructionFile{Path: f.Path, Scope: memoryScope(f.Kind), Kind: string(f.Kind), Bytes: f.Size}
	}
	return out, nil
}

// mcpScope maps an MCP scope onto the neutral config scope.
func mcpScope(s MCPScope) agentio.ConfigScope {
	switch s {
	case MCPScopeLocal:
		return agentio.ConfigLocal
	case MCPScopeUser, MCPScopeLegacy:
		return agentio.ConfigUser
	case MCPScopeProject:
		return agentio.ConfigProject
	case MCPScopeManaged:
		return agentio.ConfigManaged
	case MCPScopePlugin:
		return agentio.ConfigPlugin
	case MCPScopeSession:
		return agentio.ConfigCLI
	}
	return agentio.ConfigScopeUnknown
}

// MCPServers lists servers with env names only. cost: Disk · Read.
func (n neutral) MCPServers(h agentio.Home, dir string) ([]agentio.MCPServer, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	p, err := n.project(dir)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.MCPServers(ch, p)
	out := make([]agentio.MCPServer, len(ss))
	for i, s := range ss {
		names := make([]string, 0, len(s.Env))
		for k := range s.Env {
			names = append(names, k)
		}
		sort.Strings(names)
		ns := agentio.MCPServer{Name: s.Name, Transport: agentio.MCPTransport(s.Transport), Scope: mcpScope(s.Scope),
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
func skillScope(s SkillScope) agentio.SkillScope {
	switch s {
	case SkillScopeUser:
		return agentio.SkillUser
	case SkillScopeProject, SkillScopeAddDir:
		return agentio.SkillProject
	case SkillScopePlugin:
		return agentio.SkillPlugin
	case SkillScopeManaged:
		return agentio.SkillManaged
	case SkillScopeBundled, SkillScopeSynced:
		return agentio.SkillSystem
	}
	return agentio.SkillScope(s)
}

// Skills is Disk · Read.
func (n neutral) Skills(h agentio.Home, dir string) ([]agentio.Skill, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	p, err := n.project(dir)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.Skills(ch, p)
	out := make([]agentio.Skill, len(ss))
	for i, s := range ss {
		out[i] = agentio.Skill{Name: s.Name, Description: s.Description, Dir: s.Dir, Plugin: string(s.Plugin), Scope: skillScope(s.Scope)}
	}
	return out, err
}

// Plugins is Disk · Read.
func (n neutral) Plugins(h agentio.Home, dir string) ([]agentio.Plugin, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	p, err := n.project(dir)
	if err != nil {
		return nil, err
	}
	ps, err := n.a.Plugins(ch, p)
	out := make([]agentio.Plugin, len(ps))
	for i, pl := range ps {
		name, market := pl.ID.Split()
		out[i] = agentio.Plugin{Name: name, Marketplace: market, Version: pl.Version, Dir: pl.InstallPath, Enabled: pl.Enabled || pl.ProjectEnabled}
	}
	return out, err
}

// settingsConfigScope maps a settings layer onto the neutral scope.
func settingsConfigScope(s SettingsScope) agentio.ConfigScope {
	switch s {
	case ScopeManaged:
		return agentio.ConfigManaged
	case ScopeCLI:
		return agentio.ConfigCLI
	case ScopeLocal:
		return agentio.ConfigLocal
	case ScopeProject:
		return agentio.ConfigProject
	case ScopeUser:
		return agentio.ConfigUser
	}
	return agentio.ConfigScopeUnknown
}

// ConfigLayers lists the settings files that apply, highest first, with
// presence. cost: Disk · Read.
func (n neutral) ConfigLayers(h agentio.Home, dir string) ([]agentio.ConfigLayer, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	p, err := n.project(dir)
	if err != nil {
		return nil, err
	}
	eff, err := n.a.Settings(ch, p, nil)
	out := make([]agentio.ConfigLayer, 0, len(eff.Layers))
	for _, l := range eff.Layers {
		out = append(out, agentio.ConfigLayer{Scope: settingsConfigScope(l.Scope), Path: l.Source.Path, Exists: l.Present})
	}
	return out, err
}

// ProjectTrust reads hasTrustDialogAccepted. cost: Disk · Read.
func (n neutral) ProjectTrust(h agentio.Home, dir string) (agentio.Trust, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.TrustUnknown, err
	}
	ps, err := n.a.ProjectState(ch, filepath.Clean(dir))
	if err != nil || ps == nil {
		return agentio.TrustUnknown, err
	}
	if ps.TrustAccepted {
		return agentio.TrustTrusted, nil
	}
	return agentio.TrustUntrusted, nil
}

// autonomyOf maps a permission mode onto the neutral level; modes with no
// exact level (auto, dontAsk) stay AutonomyDefault.
func autonomyOf(m PermissionMode) agentio.Autonomy {
	switch m {
	case ModePlan:
		return agentio.AutonomyReadOnly
	case ModeManual:
		return agentio.AutonomyAsk
	case ModeAcceptEdits:
		return agentio.AutonomyAcceptEdits
	case ModeBypassPermissions:
		return agentio.AutonomyFull
	}
	return agentio.AutonomyDefault
}

// Defaults reads the merged model and permission mode. cost: Disk · Read.
func (n neutral) Defaults(h agentio.Home, dir string) (agentio.Defaults, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Defaults{}, err
	}
	p, err := n.project(dir)
	if err != nil {
		return agentio.Defaults{}, err
	}
	eff, err := n.a.Settings(ch, p, nil)
	d := agentio.Defaults{Model: eff.Merged.Model}
	if eff.Merged.Permissions != nil {
		d.Autonomy = autonomyOf(eff.Merged.Permissions.DefaultMode)
	}
	return d, err
}

// NewRunDecoder decodes a stream-json run (Type + docs-shaped Text/Tokens,
// Raw always; OD-11). cost: Pure.
func (n neutral) NewRunDecoder(r io.Reader) agentio.RunDecoder { return newRunDecoder(r) }

// Checks runs the L1 checks plus Claude's disk checks. cost: Disk · Read.
func (n neutral) Checks(ctx context.Context, h agentio.Home) []agentio.Check {
	ch, err := n.home(h)
	if err != nil {
		return []agentio.Check{{ID: agentio.CheckHome, Status: agentio.CheckFail, Message: err.Error()}}
	}
	return n.a.Doctor(ctx, ch, nil, DoctorOptions{})
}
