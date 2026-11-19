package gemini

import (
	"context"
	"slices"

	"github.com/ubgo/agentio"
)

// neutral is Gemini's L1/L2 view: it embeds the root Core for L1 and
// projects every rich Gemini result onto the neutral types. The
// assertions below are its declared capabilities; HasCapability gates the
// Core methods it promotes but Gemini lacks (LoginCommand).
type neutral struct {
	*agentio.Core
	a *Agent
}

var (
	_ agentio.Agent               = neutral{}
	_ agentio.Installer           = neutral{}
	_ agentio.BinaryVersion       = neutral{}
	_ agentio.IdentityReader      = neutral{}
	_ agentio.TokenLogin          = neutral{}
	_ agentio.ConversationLister  = neutral{}
	_ agentio.HistoryChecker      = neutral{}
	_ agentio.ProcessClassifier   = neutral{}
	_ agentio.HomeLinker          = neutral{}
	_ agentio.InstructionFiles    = neutral{}
	_ agentio.MCPLister           = neutral{}
	_ agentio.SkillLister         = neutral{}
	_ agentio.PluginLister        = neutral{}
	_ agentio.ConfigReader        = neutral{}
	_ agentio.Doctor              = neutral{}
	_ agentio.UnsupportedReasoner = neutral{}
	_ agentio.CapabilityGate      = neutral{}
)

// supported lists the capabilities Gemini has. cost: Pure.
func supported() []agentio.Capability {
	return []agentio.Capability{
		agentio.CapInstaller, agentio.CapBinaryVersion, agentio.CapIdentity, agentio.CapTokenLogin,
		agentio.CapConversations, agentio.CapHistoryChecker, agentio.CapProcessClassifier,
		agentio.CapHomeLinker, agentio.CapInstructionFiles, agentio.CapMCPServers, agentio.CapSkills,
		agentio.CapPlugins, agentio.CapConfig, agentio.CapDoctor,
		agentio.CapProcessSessions, agentio.CapStopPlanner, agentio.CapTurnFiles,
	}
}

// unsupportedReason says why Gemini lacks c; "" when it has c. cost: Pure.
func unsupportedReason(c agentio.Capability) string {
	switch c {
	case agentio.CapUsage, agentio.CapSubscription:
		return "Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet"
	case agentio.CapCachedUsage:
		return "Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage"
	case agentio.CapTokenProber:
		return "probing a Gemini key spends quota through a model call that is not modelled"
	case agentio.CapLoginCommander:
		return "Gemini CLI has no login subcommand; it asks for the auth method in its first interactive run"
	case agentio.CapHistoryMover, agentio.CapConversationMover:
		return "Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled"
	case agentio.CapPromptHistory:
		return "the per-project prompt log (tmp/<slug>/logs.json) is recorded only from source, never from a real run"
	case agentio.CapLiveSessions:
		return "Gemini CLI writes no session lock or pid file to join a process to a chat"
	case agentio.CapTurns:
		return "Gemini chats rewrite earlier records ($rewindTo, $patch), so an offset-based turn reader would report dropped turns"
	case agentio.CapModels:
		return "Gemini CLI keeps its model catalog in its own defaults, not in a file on disk"
	case agentio.CapRunEvents:
		return "the --output-format stream-json event shape is not recorded from a real run yet"
	case agentio.CapLaunchConfig:
		return "Gemini CLI has no settings flag and no typed launch options in agentio; pass flags as raw args"
	case agentio.CapLiveCredential:
		return "Gemini CLI reads its login (oauth_creds.json, the keychain or GEMINI_API_KEY) at start and refreshes OAuth itself; it has no setting that re-reads a key from a caller's command"
	}
	return "not modelled for Gemini CLI"
}

// Neutral returns the agentio.Agent view of Gemini. cost: Pure.
func (a *Agent) Neutral() agentio.Agent { return neutral{Core: a.core, a: a} }

// home converts a neutral home, refusing another agent's.
func (n neutral) home(h agentio.Home) (Home, error) { return n.a.HomeOf(h) }

// HasCapability is true for the supported list only. cost: Pure.
func (n neutral) HasCapability(c agentio.Capability) bool {
	return slices.Contains(supported(), c)
}

// UnsupportedReason declares why a capability is missing. cost: Pure.
func (n neutral) UnsupportedReason(c agentio.Capability) (string, bool) {
	if n.HasCapability(c) {
		return "", false
	}
	return unsupportedReason(c), true
}

// Launch is Pure.
func (n neutral) Launch(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	return n.a.Launch(t, r)
}

// BinaryVersion runs `gemini --version` (Process · Read; reached only
// through BinaryVersionOf, the Process door).
func (n neutral) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	return n.a.Runner().BinaryVersion(ctx)
}

// Identity is Disk · Read.
func (n neutral) Identity(h agentio.Home) (agentio.Identity, error) {
	gh, err := n.home(h)
	if err != nil {
		return agentio.Identity{}, err
	}
	id, err := n.a.Identity(gh)
	return id.Identity, err
}

// TokenKinds is Pure.
func (n neutral) TokenKinds() []agentio.TokenKind { return n.a.TokenKinds() }

// DetectTokenKind is Pure.
func (n neutral) DetectTokenKind(t agentio.Token) (agentio.TokenKind, bool) {
	return n.a.DetectTokenKind(t)
}

// TokenEnv is Pure.
func (n neutral) TokenEnv(c agentio.TokenCredential) ([]agentio.EnvChange, error) {
	return n.a.TokenEnv(c)
}

// TokenSetupCommand: Gemini has no token setup command. Pure.
func (n neutral) TokenSetupCommand(agentio.Home) (agentio.Command, bool) {
	return agentio.Command{}, false
}

// Conversations is Disk · Read; newest first.
func (n neutral) Conversations(ctx context.Context, h agentio.Home, q agentio.ConversationQuery) ([]agentio.Conversation, error) {
	gh, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.Sessions(ctx, gh, q.Dir)
	if err != nil {
		return nil, err
	}
	var out []agentio.Conversation
	for _, s := range ss {
		if !q.Since.IsZero() && s.Updated.Before(q.Since) {
			continue
		}
		out = append(out, s.Neutral(gh))
		if q.Limit > 0 && len(out) == q.Limit {
			break
		}
	}
	return out, nil
}

// Conversation is Disk · Read; ErrNotFound when absent.
func (n neutral) Conversation(h agentio.Home, id string) (agentio.Conversation, error) {
	gh, err := n.home(h)
	if err != nil {
		return agentio.Conversation{}, err
	}
	s, err := n.a.Session(context.Background(), gh, id)
	if err != nil {
		return agentio.Conversation{}, err
	}
	return s.Neutral(gh), nil
}

// HasHistory is Disk · Read.
func (n neutral) HasHistory(h agentio.Home, dir string) (bool, error) {
	gh, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.HasHistory(gh, dir)
}

// ClassifyProcess is Pure.
func (n neutral) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	return n.a.ClassifyProcess(p)
}

// InstructionFiles is Disk · Read.
func (n neutral) InstructionFiles(h agentio.Home, dir string) ([]agentio.InstructionFile, error) {
	gh, err := n.home(h)
	if err != nil {
		return nil, err
	}
	return n.a.InstructionFiles(gh, dir)
}

// MCPServers is Disk · Read.
func (n neutral) MCPServers(h agentio.Home, dir string) ([]agentio.MCPServer, error) {
	gh, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.MCPServers(gh, dir)
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
	gh, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.Skills(gh, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Skill, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Neutral())
	}
	return out, nil
}

// Plugins lists extensions. Disk · Read.
func (n neutral) Plugins(h agentio.Home, dir string) ([]agentio.Plugin, error) {
	gh, err := n.home(h)
	if err != nil {
		return nil, err
	}
	xs, err := n.a.Extensions(gh, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Plugin, 0, len(xs))
	for _, x := range xs {
		out = append(out, x.Neutral())
	}
	return out, nil
}

// ConfigLayers is Disk · Read.
func (n neutral) ConfigLayers(h agentio.Home, dir string) ([]agentio.ConfigLayer, error) {
	gh, err := n.home(h)
	if err != nil {
		return nil, err
	}
	return n.a.ConfigLayers(gh, dir)
}

// ProjectTrust is Disk · Read.
func (n neutral) ProjectTrust(h agentio.Home, dir string) (agentio.Trust, error) {
	gh, err := n.home(h)
	if err != nil {
		return agentio.TrustUnknown, err
	}
	return n.a.ProjectTrust(gh, dir)
}

// Defaults is Disk · Read.
func (n neutral) Defaults(h agentio.Home, dir string) (agentio.Defaults, error) {
	gh, err := n.home(h)
	if err != nil {
		return agentio.Defaults{}, err
	}
	return n.a.Defaults(gh, dir)
}

// Checks is Disk · Read.
func (n neutral) Checks(ctx context.Context, h agentio.Home) []agentio.Check {
	gh, err := n.home(h)
	if err != nil {
		return []agentio.Check{{ID: agentio.CheckHome, Status: agentio.CheckFail, Message: err.Error()}}
	}
	return n.a.Checks(ctx, gh)
}
