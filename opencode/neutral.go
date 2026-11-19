package opencode

import (
	"context"
	"slices"

	"github.com/ubgo/agentio"
)

// neutral is opencode's L1/L2 view: it embeds the root Core for L1 and
// replaces the home methods (the data dir follows XDG_DATA_HOME, which a
// Spec cannot describe), Launch, ClassifyProcess and StopPlan with the
// vendor ones. HasCapability gates the Core methods it promotes but
// opencode lacks (LoginCommand, home links).
type neutral struct {
	*agentio.Core
	a *Agent
}

var (
	_ agentio.Agent                = neutral{}
	_ agentio.Installer            = neutral{}
	_ agentio.BinaryVersion        = neutral{}
	_ agentio.ConversationLister   = neutral{}
	_ agentio.HistoryChecker       = neutral{}
	_ agentio.ProcessClassifier    = neutral{}
	_ agentio.ProcessSessionReader = neutral{}
	_ agentio.StopPlanner          = neutral{}
	_ agentio.TurnReader           = neutral{}
	_ agentio.TurnFileReader       = neutral{}
	_ agentio.Doctor               = neutral{}
	_ agentio.UnsupportedReasoner  = neutral{}
	_ agentio.CapabilityGate       = neutral{}
)

// supported lists the capabilities opencode has. cost: Pure.
func supported() []agentio.Capability {
	return []agentio.Capability{
		agentio.CapInstaller, agentio.CapBinaryVersion, agentio.CapConversations, agentio.CapHistoryChecker,
		agentio.CapProcessClassifier, agentio.CapProcessSessions, agentio.CapStopPlanner,
		agentio.CapTurns, agentio.CapTurnFiles, agentio.CapDoctor,
	}
}

// unsupportedReason says why opencode lacks c; "" when it has c. cost: Pure.
func unsupportedReason(c agentio.Capability) string {
	switch c {
	case agentio.CapIdentity:
		return "opencode keeps one credential per model provider in auth.json; there is no single account identity to report, and this package never reads that file"
	case agentio.CapUsage, agentio.CapCachedUsage, agentio.CapSubscription:
		return "opencode has no plan or quota of its own; it bills through each provider's key, and per-message tokens and cost are not plan usage"
	case agentio.CapTokenLogin, agentio.CapTokenProber:
		return "opencode logins are per provider in auth.json (opencode providers login); a per-provider token variable is not modelled"
	case agentio.CapLoginCommander:
		return "opencode's login (opencode providers login) asks for a provider interactively; its flow is not modelled"
	case agentio.CapHistoryMover, agentio.CapConversationMover:
		return "opencode stores the session directory inside opencode.db rows (and legacy session files); rewriting them is not modelled"
	case agentio.CapPromptHistory:
		return "opencode keeps no prompt log apart from the sessions themselves"
	case agentio.CapLiveSessions:
		return "opencode writes no lock or pid file per session; ProcessSessions joins a process by its command line instead"
	case agentio.CapHomeLinker:
		return "opencode's shareable files (opencode.json, agents, commands, plugins) live in the XDG config dir, apart from the data dir, so there is nothing to link between data dirs"
	case agentio.CapInstructionFiles, agentio.CapMCPServers, agentio.CapSkills, agentio.CapPlugins, agentio.CapConfig:
		return "opencode's config (opencode.json with instructions, mcp, agents and plugins in the XDG config dir and the project) is not modelled yet"
	case agentio.CapModels:
		return "opencode fetches its model catalog from models.dev at run time; no catalog file is modelled"
	case agentio.CapRunEvents:
		return "the opencode run --format json event shape is not recorded from a real run yet"
	case agentio.CapLiveCredential:
		return "opencode reads provider keys from auth.json and the environment at start; it has no setting that re-reads a key from a caller's command"
	case agentio.CapLaunchConfig:
		return "opencode's launch options beyond the neutral request (agent, variant, port, attach) are not typed in agentio; pass them as raw args"
	case agentio.CapHooks:
		return "opencode has no shell-command hooks; its plugins are JavaScript or TypeScript modules"
	}
	return "not modelled for opencode"
}

// Neutral returns the agentio.Agent view of opencode. cost: Pure.
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

// DefaultHome is the data dir the Host env selects. Pure.
func (n neutral) DefaultHome() agentio.Home { return n.a.DefaultHome().Neutral() }

// HomeFromEnv is the default home when XDG_DATA_HOME selects it. Pure.
func (n neutral) HomeFromEnv() (agentio.Home, bool) {
	h, ok := n.a.HomeFromEnv()
	return h.Neutral(), ok
}

// Homes is Disk · Read.
func (n neutral) Homes() ([]agentio.Home, error) {
	hs, err := n.a.Homes()
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Home, 0, len(hs))
	for _, h := range hs {
		out = append(out, h.Neutral())
	}
	return out, nil
}

// IsHome is Disk · Read.
func (n neutral) IsHome(dir string) (bool, error) { return n.a.IsHome(dir) }

// Launch is Pure.
func (n neutral) Launch(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	return n.a.Launch(t, r)
}

// BinaryVersion runs `opencode --version` (Process · Read; reached only
// through BinaryVersionOf, the Process door).
func (n neutral) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	return n.a.Runner().BinaryVersion(ctx)
}

// Conversations lists the legacy sessions. Disk · Read; newest first.
func (n neutral) Conversations(ctx context.Context, h agentio.Home, q agentio.ConversationQuery) ([]agentio.Conversation, error) {
	oh, err := n.home(h)
	if err != nil {
		return nil, err
	}
	ss, err := n.a.Sessions(ctx, oh, q)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Conversation, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Neutral(oh))
	}
	return out, nil
}

// Conversation is Disk · Read; ErrNotFound when absent.
func (n neutral) Conversation(h agentio.Home, id string) (agentio.Conversation, error) {
	oh, err := n.home(h)
	if err != nil {
		return agentio.Conversation{}, err
	}
	s, err := n.a.Session(oh, id)
	if err != nil {
		return agentio.Conversation{}, err
	}
	return s.Neutral(oh), nil
}

// HasHistory is Disk · Read.
func (n neutral) HasHistory(h agentio.Home, dir string) (bool, error) {
	oh, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.HasHistory(context.Background(), oh, dir)
}

// ClassifyProcess is Pure.
func (n neutral) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	return n.a.ClassifyProcess(p)
}

// ProcessSession is Disk · Read + Process · Read.
func (n neutral) ProcessSession(ctx context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	return n.a.ProcessSession(ctx, p)
}

// StopPlan is the shared base plan over opencode's own classification: a
// session process (the TUI, run) is terminated alone and resumed with
// --session; servers, attach clients, management commands and the npm
// launcher are refused. Pure.
func (n neutral) StopPlan(p agentio.Process, _ []agentio.Process, s agentio.ProcessSession) (agentio.StopPlan, error) {
	info, ok := n.a.ClassifyProcess(p)
	if !ok {
		return agentio.StopPlan{}, agentio.NotThisAgentProcess(ID, p)
	}
	plan := n.BaseStopPlan(info, p, s)
	if plan.Resume != nil {
		// Rebuild with the vendor launch, which sets XDG_DATA_HOME for a
		// non-default data dir.
		if cmd, err := n.a.Launch(agentio.Target{Home: s.Home}, agentio.LaunchRequest{Dir: s.Dir, ResumeID: s.ConversationID}); err == nil {
			plan.Resume = &cmd
		} else {
			plan.Resume = nil
		}
	}
	return plan, nil
}

// Turns is Disk · Read.
func (n neutral) Turns(h agentio.Home, id string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	oh, err := n.home(h)
	if err != nil {
		return from, err
	}
	return n.a.Turns(oh, id, from, fn)
}

// TurnsAt is Disk · Read.
func (n neutral) TurnsAt(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	return n.a.TurnsAt(path, from, fn)
}

// TranscriptOwner is Pure (the path alone).
func (n neutral) TranscriptOwner(path string) (agentio.TranscriptRef, error) {
	return n.a.TranscriptOwner(path)
}

// Checks is Disk · Read.
func (n neutral) Checks(ctx context.Context, h agentio.Home) []agentio.Check {
	oh, err := n.home(h)
	if err != nil {
		return []agentio.Check{{ID: agentio.CheckHome, Status: agentio.CheckFail, Message: err.Error()}}
	}
	return n.a.Checks(ctx, oh)
}
