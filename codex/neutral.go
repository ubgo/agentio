package codex

import (
	"context"
	"io"

	"github.com/khanakia/agentcli"
)

// neutral is Codex's L1/L2 view (SPEC §5.3): it embeds the root Core for
// L1 and projects every rich Codex result onto the neutral types. The
// assertions below are its declared capabilities (SPEC §6.2 Codex column).
type neutral struct {
	*agentcli.Core
	a *Agent
}

var (
	_ agentcli.Agent               = neutral{}
	_ agentcli.Installer           = neutral{}
	_ agentcli.BinaryVersion       = neutral{}
	_ agentcli.IdentityReader      = neutral{}
	_ agentcli.UsageReader         = neutral{}
	_ agentcli.CachedUsageReader   = neutral{}
	_ agentcli.SubscriptionReader  = neutral{}
	_ agentcli.LoginCommander      = neutral{}
	_ agentcli.ConversationLister  = neutral{}
	_ agentcli.HistoryChecker      = neutral{}
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
	_ agentcli.ModelLister         = neutral{}
	_ agentcli.RunEvents           = neutral{}
	_ agentcli.Doctor              = neutral{}
	_ agentcli.UnsupportedReasoner = neutral{}
)

// Neutral returns the agentcli.Agent view of Codex. Pure.
func (a *Agent) Neutral() agentcli.Agent { return neutral{Core: a.core, a: a} }

// home converts a neutral home, refusing another agent's (ErrForeignHome).
func (n neutral) home(h agentcli.Home) (Home, error) { return n.a.HomeOf(h) }

// Launch maps the common launch subset onto the Codex builders. Pure.
func (n neutral) Launch(t agentcli.Target, r agentcli.LaunchRequest) (agentcli.Command, error) {
	if err := n.Owns(t.Home); err != nil {
		return agentcli.Command{}, err
	}
	return n.a.launchNeutral(t, r)
}

// Install is Disk · Read.
func (n neutral) Install() (agentcli.Install, error) {
	in, err := n.a.Install()
	return in.Install, err
}

// BinaryVersion runs `codex --version` (Process · Read; reached only
// through BinaryVersionOf, the Process door).
func (n neutral) BinaryVersion(ctx context.Context) (agentcli.Version, error) {
	return n.a.Runner().BinaryVersion(ctx)
}

// Identity is Disk · Read (keyring homes: Process · Read via Host.Secrets).
func (n neutral) Identity(h agentcli.Home) (agentcli.Identity, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Identity{}, err
	}
	id, err := n.a.Identity(ch)
	if err != nil {
		return agentcli.Identity{}, err
	}
	return id.Neutral(), nil
}

// Usage is Network · Read.
func (n neutral) Usage(ctx context.Context, h agentcli.Home) (agentcli.Usage, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Usage{}, err
	}
	u, err := n.a.Usage(ctx, ch)
	if err != nil {
		return agentcli.Usage{}, err
	}
	return u.Neutral(), nil
}

// CachedUsage is Disk · Read.
func (n neutral) CachedUsage(h agentcli.Home) (agentcli.Usage, bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Usage{}, false, err
	}
	u, ok, err := n.a.CachedUsage(ch)
	if err != nil || !ok {
		return agentcli.Usage{}, ok, err
	}
	return u.Neutral(), true, nil
}

// Subscription is Disk · Read first, Network · Read when the claim is absent.
func (n neutral) Subscription(ctx context.Context, h agentcli.Home) (agentcli.Subscription, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Subscription{}, err
	}
	s, err := n.a.Subscription(ctx, ch)
	if err != nil {
		return agentcli.Subscription{}, err
	}
	return s.Neutral(), nil
}

// LoginCommand is Pure.
func (n neutral) LoginCommand(h agentcli.Home, f agentcli.LoginFlow) (agentcli.Command, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Command{}, err
	}
	return n.a.LoginCommand(ch, f)
}

// LogoutCommand is Pure; a zero Command for a foreign home.
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
	cs, err := n.a.Conversations(ctx, ch, ConversationQuery{Dir: q.Dir, Since: q.Since, Limit: q.Limit})
	if err != nil {
		return nil, err
	}
	out := make([]agentcli.Conversation, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Neutral())
	}
	return out, nil
}

// Conversation is Disk · Read.
func (n neutral) Conversation(h agentcli.Home, id string) (agentcli.Conversation, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Conversation{}, err
	}
	c, err := n.a.Conversation(ch, id)
	if err != nil {
		return agentcli.Conversation{}, err
	}
	return c.Neutral(), nil
}

// HasHistory is Disk · Read.
func (n neutral) HasHistory(h agentcli.Home, dir string) (bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.HasHistory(ch, dir)
}

// PromptHistory is Disk · Read.
func (n neutral) PromptHistory(h agentcli.Home, q agentcli.PromptQuery) ([]agentcli.PromptEntry, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	es, err := n.a.PromptHistory(ch, q)
	if err != nil {
		return nil, err
	}
	out := make([]agentcli.PromptEntry, 0, len(es))
	for _, e := range es {
		out = append(out, e.Neutral())
	}
	return out, nil
}

// LiveSessions is Disk · Read + one process-table read.
func (n neutral) LiveSessions(h agentcli.Home) ([]agentcli.LiveSession, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	return n.a.LiveSessions(ch)
}

// IsLive is Disk · Read + one process-table read.
func (n neutral) IsLive(h agentcli.Home, id string) (bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.IsLive(ch, id)
}

// ClassifyProcess is Pure.
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

// InstructionFiles is Disk · Read.
func (n neutral) InstructionFiles(h agentcli.Home, dir string) ([]agentcli.InstructionFile, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	fs, err := n.a.InstructionFiles(ch, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentcli.InstructionFile, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Neutral())
	}
	return out, nil
}

// MCPServers is Disk · Read.
func (n neutral) MCPServers(h agentcli.Home, dir string) ([]agentcli.MCPServer, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.MCPServers(ch, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentcli.MCPServer, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Neutral())
	}
	return out, nil
}

// Skills is Disk · Read.
func (n neutral) Skills(h agentcli.Home, dir string) ([]agentcli.Skill, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.Skills(ch, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentcli.Skill, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Neutral())
	}
	return out, nil
}

// Plugins is Disk · Read (Codex plugins are per home; dir is unused).
func (n neutral) Plugins(h agentcli.Home, _ string) ([]agentcli.Plugin, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ps, err := n.a.Plugins(ch)
	if err != nil {
		return nil, err
	}
	out := make([]agentcli.Plugin, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Neutral())
	}
	return out, nil
}

// ConfigLayers is Disk · Read.
func (n neutral) ConfigLayers(h agentcli.Home, dir string) ([]agentcli.ConfigLayer, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ls, err := n.a.ConfigLayers(ch, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentcli.ConfigLayer, 0, len(ls))
	for _, l := range ls {
		if l.Disabled != "" {
			continue
		}
		out = append(out, agentcli.ConfigLayer{Scope: layerScope(l.Kind), Path: l.File, Exists: l.Exists})
	}
	return out, nil
}

// ProjectTrust is Disk · Read.
func (n neutral) ProjectTrust(h agentcli.Home, dir string) (agentcli.Trust, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.TrustUnknown, err
	}
	c, err := n.a.ConfigFile(ch)
	if err != nil {
		return agentcli.TrustUnknown, err
	}
	t, ok := c.TrustOf(dir)
	switch {
	case !ok:
		return agentcli.TrustUnknown, nil
	case t == TrustTrusted:
		return agentcli.TrustTrusted, nil
	case t == TrustUntrusted:
		return agentcli.TrustUntrusted, nil
	}
	return agentcli.TrustUnknown, nil
}

// Defaults is Disk · Read: the effective model and the autonomy the
// effective sandbox/approval pair maps to exactly (else Default).
func (n neutral) Defaults(h agentcli.Home, dir string) (agentcli.Defaults, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentcli.Defaults{}, err
	}
	eff, err := n.a.Effective(ch, EffectiveQuery{Dir: dir})
	if err != nil {
		return agentcli.Defaults{}, err
	}
	c := eff.Config
	d := agentcli.Defaults{Model: c.Model.V, Autonomy: agentcli.AutonomyDefault}
	switch {
	case c.SandboxMode.V == ModeReadOnly && c.ApprovalPolicy.V == ApprovalOnRequest:
		d.Autonomy = agentcli.AutonomyReadOnly
	case c.SandboxMode.V == ModeWorkspaceWrite && c.ApprovalPolicy.V == ApprovalOnRequest:
		d.Autonomy = agentcli.AutonomyAsk
	case c.SandboxMode.V == ModeDangerFullAccess && c.ApprovalPolicy.V == ApprovalNever:
		d.Autonomy = agentcli.AutonomyFull
	}
	return d, nil
}

// Models is Disk · Read.
func (n neutral) Models(h agentcli.Home) ([]agentcli.Model, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	cat, err := n.a.Models(ch)
	if err != nil {
		return nil, err
	}
	out := make([]agentcli.Model, 0, len(cat.Models))
	for _, m := range cat.Models {
		out = append(out, m.Neutral())
	}
	return out, nil
}

// NewRunDecoder is Pure.
func (n neutral) NewRunDecoder(r io.Reader) agentcli.RunDecoder { return n.a.NewRunDecoder(r) }

// Checks is Disk · Read; a foreign home yields the core's failing check.
func (n neutral) Checks(ctx context.Context, h agentcli.Home) []agentcli.Check {
	ch, err := n.home(h)
	if err != nil {
		return n.Core.Checks(ctx, h)
	}
	return n.a.Checks(ctx, ch)
}

// Unsupported capability reasons.
const (
	reasonNoTokenLogin = "Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)"
	reasonNoProbe      = "Codex has no quota probe: no token login to probe"
)

// UnsupportedReason declares why Codex lacks a capability. Pure.
func (n neutral) UnsupportedReason(c agentcli.Capability) (string, bool) {
	switch c {
	case agentcli.CapHistoryMover, agentcli.CapConversationMover:
		return n.a.MoveUnsupportedReason(), true
	case agentcli.CapTokenLogin:
		return reasonNoTokenLogin, true
	case agentcli.CapTokenProber:
		return reasonNoProbe, true
	}
	return "", false
}
