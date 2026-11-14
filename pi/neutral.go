package pi

import (
	"context"

	"github.com/ubgo/agentio"
)

// neutral is the L1 + L2 view of pi: it embeds the shared Core (L1) and
// projects pi's results onto the neutral types. The compile-time
// assertions below declare its capabilities (SPEC §6.2).
type neutral struct {
	*agentio.Core
	a *Agent
}

// Capabilities pi has.
var (
	_ agentio.Agent               = neutral{}
	_ agentio.Installer           = neutral{}
	_ agentio.BinaryVersion       = neutral{}
	_ agentio.IdentityReader      = neutral{}
	_ agentio.TokenLogin          = neutral{}
	_ agentio.ConversationLister  = neutral{}
	_ agentio.HistoryChecker      = neutral{}
	_ agentio.ProcessClassifier   = neutral{}
	_ agentio.TurnReader          = neutral{}
	_ agentio.InstructionFiles    = neutral{}
	_ agentio.MCPLister           = neutral{}
	_ agentio.SkillLister         = neutral{}
	_ agentio.ConfigReader        = neutral{}
	_ agentio.Doctor              = neutral{}
	_ agentio.UnsupportedReasoner = neutral{}
	_ agentio.CapabilityGate      = neutral{}
)

// HasCapability turns off the two capabilities the embedded Core would
// otherwise expose with nothing behind them: LoginCommander (the Spec has
// no login args) and HomeLinker (no shared home files). Pure.
func (n neutral) HasCapability(c agentio.Capability) bool {
	return c != agentio.CapLoginCommander && c != agentio.CapHomeLinker
}

// Why pi lacks the other capabilities.
const (
	reasonUsage       = "pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)"
	reasonTokenProber = "pi has no plan or quota to probe"
	reasonLogin       = "pi logs in with /login inside its interactive UI; there is no login subcommand to build"
	reasonMove        = "pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled"
	reasonPromptHist  = "pi keeps no prompt history file; prompts live only in session files"
	reasonLive        = "pi writes no live-session registry; only the process table shows a running pi"
	reasonHomeLinker  = "pi has no documented set of files to share between agent dirs"
	reasonPlugins     = "pi packages and extensions are code loaded into the process, not plugins with a manifest this library models"
	reasonModels      = "pi's model catalog (models-store.json) has no recorded shape yet"
	reasonRunEvents   = "pi's --mode json event stream is recorded once but not yet modelled as a decoder"
)

// Neutral returns the L1/L2 view. Pure.
func (a *Agent) Neutral() agentio.Agent { return neutral{Core: a.core, a: a} }

// home converts a neutral home, refusing another agent's.
func (n neutral) home(h agentio.Home) (Home, error) { return n.a.HomeOf(h) }

// UnsupportedReason declares why each missing capability is missing. Pure.
func (n neutral) UnsupportedReason(c agentio.Capability) (string, bool) {
	switch c {
	case agentio.CapUsage, agentio.CapCachedUsage, agentio.CapSubscription:
		return reasonUsage, true
	case agentio.CapTokenProber:
		return reasonTokenProber, true
	case agentio.CapLoginCommander:
		return reasonLogin, true
	case agentio.CapHistoryMover, agentio.CapConversationMover:
		return reasonMove, true
	case agentio.CapPromptHistory:
		return reasonPromptHist, true
	case agentio.CapLiveSessions:
		return reasonLive, true
	case agentio.CapHomeLinker:
		return reasonHomeLinker, true
	case agentio.CapPlugins:
		return reasonPlugins, true
	case agentio.CapModels:
		return reasonModels, true
	case agentio.CapRunEvents:
		return reasonRunEvents, true
	}
	return "", false
}

// Launch maps the neutral request onto LaunchOptions. Pure.
func (n neutral) Launch(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	return n.a.launchRequest(t, r)
}

// Install is Disk · Read.
func (n neutral) Install() (agentio.Install, error) { return n.a.Install() }

// BinaryVersion runs `pi --version` (Process · Read; reached only through
// BinaryVersionOf, the door accessor).
func (n neutral) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	return n.a.Runner().BinaryVersion(ctx)
}

// Identity is Disk · Read.
func (n neutral) Identity(h agentio.Home) (agentio.Identity, error) {
	ph, err := n.home(h)
	if err != nil {
		return agentio.Identity{}, err
	}
	return n.a.Identity(ph)
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

// TokenSetupCommand is Pure.
func (n neutral) TokenSetupCommand(h agentio.Home) (agentio.Command, bool) {
	ph, err := n.home(h)
	if err != nil {
		return agentio.Command{}, false
	}
	return n.a.TokenSetupCommand(ph)
}

// Conversations is Disk · Read.
func (n neutral) Conversations(ctx context.Context, h agentio.Home, q agentio.ConversationQuery) ([]agentio.Conversation, error) {
	ph, err := n.home(h)
	if err != nil {
		return nil, err
	}
	return n.a.Conversations(ctx, ph, q)
}

// Conversation is Disk · Read; ErrNotFound when absent.
func (n neutral) Conversation(h agentio.Home, id string) (agentio.Conversation, error) {
	ph, err := n.home(h)
	if err != nil {
		return agentio.Conversation{}, err
	}
	s, err := n.a.FindSession(ph, id)
	if err != nil {
		return agentio.Conversation{}, err
	}
	return conversation(s), nil
}

// HasHistory is Disk · Read.
func (n neutral) HasHistory(h agentio.Home, dir string) (bool, error) {
	ph, err := n.home(h)
	if err != nil {
		return false, err
	}
	return n.a.HasHistory(ph, dir)
}

// Turns is Disk · Read.
func (n neutral) Turns(h agentio.Home, id string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	ph, err := n.home(h)
	if err != nil {
		return from, err
	}
	return n.a.Turns(ph, id, from, fn)
}

// InstructionFiles is Disk · Read.
func (n neutral) InstructionFiles(h agentio.Home, dir string) ([]agentio.InstructionFile, error) {
	ph, err := n.home(h)
	if err != nil {
		return nil, err
	}
	return n.a.InstructionFiles(ph, dir)
}

// MCPServers is Disk · Read.
func (n neutral) MCPServers(h agentio.Home, dir string) ([]agentio.MCPServer, error) {
	ph, err := n.home(h)
	if err != nil {
		return nil, err
	}
	return n.a.MCPServers(ph, dir)
}

// Skills is Disk · Read.
func (n neutral) Skills(h agentio.Home, dir string) ([]agentio.Skill, error) {
	ph, err := n.home(h)
	if err != nil {
		return nil, err
	}
	return n.a.Skills(ph, dir)
}

// ConfigLayers is Disk · Read.
func (n neutral) ConfigLayers(h agentio.Home, dir string) ([]agentio.ConfigLayer, error) {
	ph, err := n.home(h)
	if err != nil {
		return nil, err
	}
	return n.a.ConfigLayers(ph, dir)
}

// ProjectTrust is Disk · Read.
func (n neutral) ProjectTrust(h agentio.Home, dir string) (agentio.Trust, error) {
	ph, err := n.home(h)
	if err != nil {
		return agentio.TrustUnknown, err
	}
	return n.a.ProjectTrust(ph, dir)
}

// Defaults is Disk · Read.
func (n neutral) Defaults(h agentio.Home, dir string) (agentio.Defaults, error) {
	ph, err := n.home(h)
	if err != nil {
		return agentio.Defaults{}, err
	}
	return n.a.Defaults(ph, dir)
}

// Checks is Disk · Read.
func (n neutral) Checks(ctx context.Context, h agentio.Home) []agentio.Check {
	ph, err := n.home(h)
	if err != nil {
		return []agentio.Check{{ID: agentio.CheckHome, Status: agentio.CheckFail, Message: err.Error()}}
	}
	return n.a.Checks(ctx, ph)
}
