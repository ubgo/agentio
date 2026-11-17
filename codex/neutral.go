package codex

import (
	"context"
	"io"

	"github.com/ubgo/agentio"
)

// neutral is Codex's L1/L2 view (SPEC §5.3): it embeds the root Core for
// L1 and projects every rich Codex result onto the neutral types. The
// assertions below are its declared capabilities (SPEC §6.2 Codex column).
type neutral struct {
	*agentio.Core
	a *Agent
}

var (
	_ agentio.Agent               = neutral{}
	_ agentio.Installer           = neutral{}
	_ agentio.BinaryVersion       = neutral{}
	_ agentio.IdentityReader      = neutral{}
	_ agentio.UsageReader         = neutral{}
	_ agentio.CachedUsageReader   = neutral{}
	_ agentio.SubscriptionReader  = neutral{}
	_ agentio.LoginCommander      = neutral{}
	_ agentio.ConversationLister  = neutral{}
	_ agentio.HistoryChecker      = neutral{}
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
	_ agentio.ModelLister         = neutral{}
	_ agentio.RunEvents           = neutral{}
	_ agentio.Doctor              = neutral{}
	_ agentio.LaunchConfigurer    = neutral{}
	_ agentio.UnsupportedReasoner = neutral{}
)

// Neutral returns the agentio.Agent view of Codex. Pure.
func (a *Agent) Neutral() agentio.Agent { return neutral{Core: a.core, a: a} }

// home converts a neutral home, refusing another agent's (ErrForeignHome).
func (n neutral) home(h agentio.Home) (Home, error) { return n.a.HomeOf(h) }

// Launch maps the common launch subset onto the Codex builders. Pure.
func (n neutral) Launch(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	if err := n.Owns(t.Home); err != nil {
		return agentio.Command{}, err
	}
	return n.a.launchNeutral(t, r)
}

// Install is Disk · Read.
func (n neutral) Install() (agentio.Install, error) {
	in, err := n.a.Install()
	return in.Install, err
}

// BinaryVersion runs `codex --version` (Process · Read; reached only
// through BinaryVersionOf, the Process door).
func (n neutral) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	return n.a.Runner().BinaryVersion(ctx)
}

// Identity is Disk · Read (keyring homes: Process · Read via Host.Secrets).
func (n neutral) Identity(h agentio.Home) (agentio.Identity, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Identity{}, err
	}
	id, err := n.a.Identity(ch)
	if err != nil {
		return agentio.Identity{}, err
	}
	return id.Neutral(), nil
}

// Usage is Network · Read.
func (n neutral) Usage(ctx context.Context, h agentio.Home) (agentio.Usage, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Usage{}, err
	}
	u, err := n.a.Usage(ctx, ch)
	if err != nil {
		return agentio.Usage{}, err
	}
	return u.Neutral(), nil
}

// CachedUsage is Disk · Read.
func (n neutral) CachedUsage(h agentio.Home) (agentio.Usage, bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Usage{}, false, err
	}
	u, ok, err := n.a.CachedUsage(ch)
	if err != nil || !ok {
		return agentio.Usage{}, ok, err
	}
	return u.Neutral(), true, nil
}

// Subscription is Disk · Read first, Network · Read when the claim is absent.
func (n neutral) Subscription(ctx context.Context, h agentio.Home) (agentio.Subscription, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Subscription{}, err
	}
	s, err := n.a.Subscription(ctx, ch)
	if err != nil {
		return agentio.Subscription{}, err
	}
	return s.Neutral(), nil
}

// LoginCommand is Pure.
func (n neutral) LoginCommand(h agentio.Home, f agentio.LoginFlow) (agentio.Command, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Command{}, err
	}
	return n.a.LoginCommand(ch, f)
}

// LogoutCommand is Pure; a zero Command for a foreign home.
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
	cs, err := n.a.Conversations(ctx, ch, ConversationQuery{Dir: q.Dir, Since: q.Since, Limit: q.Limit})
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Conversation, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Neutral())
	}
	return out, nil
}

// Conversation is Disk · Read.
func (n neutral) Conversation(h agentio.Home, id string) (agentio.Conversation, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Conversation{}, err
	}
	c, err := n.a.Conversation(ch, id)
	if err != nil {
		return agentio.Conversation{}, err
	}
	return c.Neutral(), nil
}

// HasHistory is Disk · Read.
func (n neutral) HasHistory(h agentio.Home, dir string) (bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.HasHistory(ch, dir)
}

// PromptHistory is Disk · Read.
func (n neutral) PromptHistory(h agentio.Home, q agentio.PromptQuery) ([]agentio.PromptEntry, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	es, err := n.a.PromptHistory(ch, q)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.PromptEntry, 0, len(es))
	for _, e := range es {
		out = append(out, e.Neutral())
	}
	return out, nil
}

// LiveSessions is Disk · Read + one process-table read.
func (n neutral) LiveSessions(h agentio.Home) ([]agentio.LiveSession, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	return n.a.LiveSessions(ch)
}

// IsLive is Disk · Read + one process-table read.
func (n neutral) IsLive(h agentio.Home, id string) (bool, error) {
	ch, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.IsLive(ch, id)
}

// ClassifyProcess is Pure.
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

// InstructionFiles is Disk · Read.
func (n neutral) InstructionFiles(h agentio.Home, dir string) ([]agentio.InstructionFile, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	fs, err := n.a.InstructionFiles(ch, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.InstructionFile, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Neutral())
	}
	return out, nil
}

// MCPServers is Disk · Read.
func (n neutral) MCPServers(h agentio.Home, dir string) ([]agentio.MCPServer, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.MCPServers(ch, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.MCPServer, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Neutral())
	}
	return out, nil
}

// Skills is Disk · Read.
func (n neutral) Skills(h agentio.Home, dir string) ([]agentio.Skill, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.Skills(ch, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Skill, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Neutral())
	}
	return out, nil
}

// Plugins is Disk · Read (Codex plugins are per home; dir is unused).
func (n neutral) Plugins(h agentio.Home, _ string) ([]agentio.Plugin, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ps, err := n.a.Plugins(ch)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Plugin, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Neutral())
	}
	return out, nil
}

// ConfigLayers lists the applied layers highest precedence first (the
// agentio.ConfigReader order); disabled (untrusted) layers are left out.
// Disk · Read.
func (n neutral) ConfigLayers(h agentio.Home, dir string) ([]agentio.ConfigLayer, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ls, err := n.a.ConfigLayers(ch, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.ConfigLayer, 0, len(ls))
	for _, l := range ls {
		if l.Disabled != "" {
			continue
		}
		out = append(out, agentio.ConfigLayer{Scope: layerScope(l.Kind), Path: l.File, Exists: l.Exists})
	}
	return out, nil
}

// ProjectTrust is Disk · Read.
func (n neutral) ProjectTrust(h agentio.Home, dir string) (agentio.Trust, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.TrustUnknown, err
	}
	c, err := n.a.ConfigFile(ch)
	if err != nil {
		return agentio.TrustUnknown, err
	}
	t, ok := c.TrustOf(dir)
	switch {
	case !ok:
		return agentio.TrustUnknown, nil
	case t == TrustTrusted:
		return agentio.TrustTrusted, nil
	case t == TrustUntrusted:
		return agentio.TrustUntrusted, nil
	}
	return agentio.TrustUnknown, nil
}

// Defaults is Disk · Read: the effective model and the autonomy the
// effective sandbox/approval pair maps to exactly (else Default).
func (n neutral) Defaults(h agentio.Home, dir string) (agentio.Defaults, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.Defaults{}, err
	}
	eff, err := n.a.Effective(ch, EffectiveQuery{Dir: dir})
	if err != nil {
		return agentio.Defaults{}, err
	}
	c := eff.Config
	d := agentio.Defaults{Model: c.Model.V, Autonomy: agentio.AutonomyDefault}
	switch {
	case c.SandboxMode.V == ModeReadOnly && c.ApprovalPolicy.V == ApprovalOnRequest:
		d.Autonomy = agentio.AutonomyReadOnly
	case c.SandboxMode.V == ModeWorkspaceWrite && c.ApprovalPolicy.V == ApprovalOnRequest:
		d.Autonomy = agentio.AutonomyAsk
	case c.SandboxMode.V == ModeDangerFullAccess && c.ApprovalPolicy.V == ApprovalNever:
		d.Autonomy = agentio.AutonomyFull
	}
	return d, nil
}

// Models is Disk · Read.
func (n neutral) Models(h agentio.Home) ([]agentio.Model, error) {
	ch, err := n.home(h)
	if err != nil {
		return nil, err
	}
	cat, err := n.a.Models(ch)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Model, 0, len(cat.Models))
	for _, m := range cat.Models {
		out = append(out, m.Neutral())
	}
	return out, nil
}

// NewRunDecoder is Pure.
func (n neutral) NewRunDecoder(r io.Reader) agentio.RunDecoder { return n.a.NewRunDecoder(r) }

// Checks is Disk · Read; a foreign home yields the core's failing check.
func (n neutral) Checks(ctx context.Context, h agentio.Home) []agentio.Check {
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

// reasonNoLiveCredential is why Codex has no live credential: it reads
// auth.json or its keyring entry and refreshes the token itself; there is
// no setting that makes it ask a caller's command for a key.
const reasonNoLiveCredential = "Codex reads its login from auth.json or the keyring and refreshes it itself; it has no credential-helper setting"

// UnsupportedReason declares why Codex lacks a capability. Pure.
func (n neutral) UnsupportedReason(c agentio.Capability) (string, bool) {
	switch c {
	case agentio.CapHistoryMover, agentio.CapConversationMover:
		return n.a.MoveUnsupportedReason(), true
	case agentio.CapTokenLogin:
		return reasonNoTokenLogin, true
	case agentio.CapTokenProber:
		return reasonNoProbe, true
	case agentio.CapLiveCredential:
		return reasonNoLiveCredential, true
	}
	return "", false
}
