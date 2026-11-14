package agentio

import (
	"errors"
	"fmt"
)

// capabilityOf is the one place a capability type assertion happens: the
// agent must implement I and, when it gates capabilities on data
// (CapabilityGate), report c as present. Why `comparable` as the
// constraint: every interface type satisfies it and the no-`any` rule
// bans the unconstrained form.
func capabilityOf[I comparable](a Agent, c Capability) (I, bool) {
	var zero I
	if a == nil {
		return zero, false
	}
	v, ok := a.(I)
	if !ok {
		return zero, false
	}
	if g, gated := a.(CapabilityGate); gated && !g.HasCapability(c) {
		return zero, false
	}
	return v, true
}

// InstallerOf returns the agent's Installer (Disk · Read). Pure.
func InstallerOf(a Agent) (Installer, bool) { return capabilityOf[Installer](a, CapInstaller) }

// BinaryVersionOf returns the BinaryVersion door (Process · Read, may touch login). Pure.
func BinaryVersionOf(a Agent) (BinaryVersion, bool) {
	return capabilityOf[BinaryVersion](a, CapBinaryVersion)
}

// IdentityOf returns the IdentityReader (Disk · Read). Pure.
func IdentityOf(a Agent) (IdentityReader, bool) { return capabilityOf[IdentityReader](a, CapIdentity) }

// UsageOf returns the UsageReader (Network · Read). Pure.
func UsageOf(a Agent) (UsageReader, bool) { return capabilityOf[UsageReader](a, CapUsage) }

// CachedUsageOf returns the CachedUsageReader (Disk · Read). Pure.
func CachedUsageOf(a Agent) (CachedUsageReader, bool) {
	return capabilityOf[CachedUsageReader](a, CapCachedUsage)
}

// SubscriptionOf returns the SubscriptionReader (Network · Read). Pure.
func SubscriptionOf(a Agent) (SubscriptionReader, bool) {
	return capabilityOf[SubscriptionReader](a, CapSubscription)
}

// TokenLoginOf returns the TokenLogin (Pure). Pure.
func TokenLoginOf(a Agent) (TokenLogin, bool) { return capabilityOf[TokenLogin](a, CapTokenLogin) }

// TokenProberOf returns the TokenProber door (Quota). Pure.
func TokenProberOf(a Agent) (TokenProber, bool) { return capabilityOf[TokenProber](a, CapTokenProber) }

// LoginCommanderOf returns the LoginCommander (Pure). Pure.
func LoginCommanderOf(a Agent) (LoginCommander, bool) {
	return capabilityOf[LoginCommander](a, CapLoginCommander)
}

// ConversationsOf returns the ConversationLister (Disk · Read). Pure.
func ConversationsOf(a Agent) (ConversationLister, bool) {
	return capabilityOf[ConversationLister](a, CapConversations)
}

// HistoryCheckerOf returns the HistoryChecker (Disk · Read). Pure.
func HistoryCheckerOf(a Agent) (HistoryChecker, bool) {
	return capabilityOf[HistoryChecker](a, CapHistoryChecker)
}

// HistoryMoverOf returns the HistoryMover door (Disk · Write). Pure.
func HistoryMoverOf(a Agent) (HistoryMover, bool) {
	return capabilityOf[HistoryMover](a, CapHistoryMover)
}

// ConversationMoverOf returns the ConversationMover door (Disk · Write). Pure.
func ConversationMoverOf(a Agent) (ConversationMover, bool) {
	return capabilityOf[ConversationMover](a, CapConversationMover)
}

// PromptHistoryOf returns the PromptHistoryReader (Disk · Read). Pure.
func PromptHistoryOf(a Agent) (PromptHistoryReader, bool) {
	return capabilityOf[PromptHistoryReader](a, CapPromptHistory)
}

// LiveSessionsOf returns the LiveSessionLister (Disk · Read + process table). Pure.
func LiveSessionsOf(a Agent) (LiveSessionLister, bool) {
	return capabilityOf[LiveSessionLister](a, CapLiveSessions)
}

// ProcessClassifierOf returns the ProcessClassifier (Pure). Pure.
func ProcessClassifierOf(a Agent) (ProcessClassifier, bool) {
	return capabilityOf[ProcessClassifier](a, CapProcessClassifier)
}

// TurnsOf returns the TurnReader (Disk · Read). Pure.
func TurnsOf(a Agent) (TurnReader, bool) { return capabilityOf[TurnReader](a, CapTurns) }

// HomeLinkerOf returns the HomeLinker door (Disk · Write). Pure.
func HomeLinkerOf(a Agent) (HomeLinker, bool) { return capabilityOf[HomeLinker](a, CapHomeLinker) }

// InstructionFilesOf returns the InstructionFiles reader (Disk · Read). Pure.
func InstructionFilesOf(a Agent) (InstructionFiles, bool) {
	return capabilityOf[InstructionFiles](a, CapInstructionFiles)
}

// MCPServersOf returns the MCPLister (Disk · Read). Pure.
func MCPServersOf(a Agent) (MCPLister, bool) { return capabilityOf[MCPLister](a, CapMCPServers) }

// SkillsOf returns the SkillLister (Disk · Read). Pure.
func SkillsOf(a Agent) (SkillLister, bool) { return capabilityOf[SkillLister](a, CapSkills) }

// PluginsOf returns the PluginLister (Disk · Read). Pure.
func PluginsOf(a Agent) (PluginLister, bool) { return capabilityOf[PluginLister](a, CapPlugins) }

// ConfigOf returns the ConfigReader (Disk · Read). Pure.
func ConfigOf(a Agent) (ConfigReader, bool) { return capabilityOf[ConfigReader](a, CapConfig) }

// ModelsOf returns the ModelLister (Disk · Read). Pure.
func ModelsOf(a Agent) (ModelLister, bool) { return capabilityOf[ModelLister](a, CapModels) }

// RunEventsOf returns the RunEvents decoder factory (Pure). Pure.
func RunEventsOf(a Agent) (RunEvents, bool) { return capabilityOf[RunEvents](a, CapRunEvents) }

// DoctorOf returns the Doctor (Disk · Read). Pure.
func DoctorOf(a Agent) (Doctor, bool) { return capabilityOf[Doctor](a, CapDoctor) }

// LiveCredentialOf returns the live-credential planner (Pure): an agent
// whose running session re-reads its credential from a caller's command.
// Pure.
func LiveCredentialOf(a Agent) (LiveCredentialer, bool) {
	return capabilityOf[LiveCredentialer](a, CapLiveCredential)
}

// capabilityRow binds a Capability to its accessor and cost.
type capabilityRow struct {
	cap  Capability
	has  func(Agent) bool
	cost Cost
}

// present adapts a typed accessor to a presence check.
func present[I comparable](of func(Agent) (I, bool)) func(Agent) bool {
	return func(a Agent) bool { _, ok := of(a); return ok }
}

// capabilityTable is the single table Supported and Capability.Cost read.
// A test fails when a Capability constant has no row, so the listing
// cannot drift from the accessors (archtest 9). Costs follow SPEC §6.2.
func capabilityTable() []capabilityRow {
	diskR := Cost{Class: CostDisk, Access: Read}
	diskW := Cost{Class: CostDisk, Access: Write}
	pure := Cost{Class: CostPure, Access: Read}
	return []capabilityRow{
		{CapInstaller, present(InstallerOf), diskR},
		{CapBinaryVersion, present(BinaryVersionOf), Cost{Class: CostProcess, Access: Read, MayTouchLogin: true}},
		// Identity is Disk for Codex, Disk + keychain Process for Claude.
		{CapIdentity, present(IdentityOf), Cost{Class: CostProcess, Access: Read}},
		{CapUsage, present(UsageOf), Cost{Class: CostNetwork, Access: Read}},
		{CapCachedUsage, present(CachedUsageOf), diskR},
		{CapSubscription, present(SubscriptionOf), Cost{Class: CostNetwork, Access: Read}},
		{CapTokenLogin, present(TokenLoginOf), pure},
		{CapTokenProber, present(TokenProberOf), Cost{Class: CostQuota, Access: Read}},
		{CapLoginCommander, present(LoginCommanderOf), pure},
		{CapConversations, present(ConversationsOf), diskR},
		{CapHistoryChecker, present(HistoryCheckerOf), diskR},
		{CapHistoryMover, present(HistoryMoverOf), diskW},
		{CapConversationMover, present(ConversationMoverOf), diskW},
		{CapPromptHistory, present(PromptHistoryOf), diskR},
		// Live sessions read the process table (Process) besides disk.
		{CapLiveSessions, present(LiveSessionsOf), Cost{Class: CostProcess, Access: Read}},
		{CapProcessClassifier, present(ProcessClassifierOf), pure},
		{CapTurns, present(TurnsOf), diskR},
		{CapHomeLinker, present(HomeLinkerOf), diskW},
		{CapInstructionFiles, present(InstructionFilesOf), diskR},
		{CapMCPServers, present(MCPServersOf), diskR},
		{CapSkills, present(SkillsOf), diskR},
		{CapPlugins, present(PluginsOf), diskR},
		{CapConfig, present(ConfigOf), diskR},
		{CapModels, present(ModelsOf), diskR},
		{CapRunEvents, present(RunEventsOf), pure},
		{CapDoctor, present(DoctorOf), diskR},
		{CapLiveCredential, present(LiveCredentialOf), pure},
	}
}

// Cost is the worst case over the capability's methods (SPEC §4). Pure.
func (c Capability) Cost() Cost {
	for _, r := range capabilityTable() {
		if r.cap == c {
			return r.cost
		}
	}
	return Cost{}
}

// Cost is a method's cost class and access kind (SPEC §4).
type Cost struct {
	Class  CostClass
	Access Access
	// MayTouchLogin: the call runs the vendor binary, which can refresh and
	// rewrite the home's login (seen: codex rewrote auth.json, 2026-11-13).
	MayTouchLogin bool
}

// Supported lists a's capabilities by calling every accessor from the one
// table. Pure.
func Supported(a Agent) []Capability {
	var out []Capability
	for _, r := range capabilityTable() {
		if r.has(a) {
			out = append(out, r.cap)
		}
	}
	return out
}

// Unsupported gives the reason a declares for a missing capability. Pure.
func Unsupported(a Agent, c Capability) (reason string, declared bool) {
	if r, ok := a.(UnsupportedReasoner); ok {
		return r.UnsupportedReason(c)
	}
	return "", false
}

// FindLiveSession joins a pid to (agent, home, live session) by looping
// Homes() × LiveSessionsOf. A process in a home Homes() does not discover
// is missed. Disk · Read + process table.
func FindLiveSession(agents []Agent, pid int) (Agent, LiveSession, bool, error) {
	var errs []error
	for _, a := range agents {
		lister, ok := LiveSessionsOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s homes: %w", a.ID(), err))
			continue
		}
		for _, h := range homes {
			sessions, err := lister.LiveSessions(h)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s live sessions: %w", h, err))
				continue
			}
			for _, s := range sessions {
				if s.PID == pid {
					return a, s, true, nil
				}
			}
		}
	}
	return nil, LiveSession{}, false, errors.Join(errs...)
}

// ClassifyProcess returns the first agent that recognises p, in the order
// given (built-ins before custom, as builtin.Agents.All orders them). Pure.
func ClassifyProcess(agents []Agent, p Process) (ProcessInfo, bool) {
	for _, a := range agents {
		c, ok := ProcessClassifierOf(a)
		if !ok {
			continue
		}
		if info, ok := c.ClassifyProcess(p); ok {
			return info, true
		}
	}
	return ProcessInfo{}, false
}
