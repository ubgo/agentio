package agentio

import "time"

// StopPlan says how to end one agent process without losing its
// conversation. It is data: agentio never sends a signal. The caller
// re-checks each target's start time right before signalling
// (Host.ProcessAlive), sends Graceful to every target in order, waits up
// to Grace, and forces only the targets still running.
type StopPlan struct {
	Agent ID
	// Targets are the pids to signal, in order: the process itself, or
	// for an agent that relaunches itself (Gemini CLI) the child that
	// holds the conversation and then its relaunch parent. Empty when
	// Refuse is set.
	Targets []int
	// Graceful is the first request to send; StopSignalNone when the OS
	// or agent has none (then the only way is to force).
	Graceful StopSignal
	// Grace is how long to wait after Graceful before forcing, long
	// enough for the agent to flush its transcript.
	Grace time.Duration
	// Force says whether forcing after Grace is safe for the
	// conversation (the transcript is written turn by turn, so a forced
	// stop loses at most the turn in flight).
	Force bool
	// Refuse, when not empty, says why this process must not be stopped
	// as a session: it is not one (an app-server daemon serving many
	// conversations, an MCP server, an npm launcher whose child is the
	// real session). Targets is empty then.
	Refuse string
	// KeepsConversation is true when the agent writes its transcript as
	// it goes, so the conversation can be resumed after the stop.
	KeepsConversation bool
	// Command, when set, is the agent's own stop command to run instead
	// of a signal (Claude background sessions: a supervisor respawns a
	// killed worker, `claude stop <id>` ends it).
	Command *Command
	// Resume brings the conversation back after the stop; nil when the
	// conversation id is unknown.
	Resume *Command
}

// ResumeLine is the line a person types to bring the conversation back
// after the stop (Resume.ShellLine): the home variable for a non-default
// home, never a token or any other env value; "" when Resume is nil.
// Pure.
func (p StopPlan) ResumeLine() string {
	if p.Resume == nil {
		return ""
	}
	return p.Resume.ShellLine()
}

// StopPlanner plans how to stop one of the agent's processes (an
// optional capability; StopPlannerOf).
type StopPlanner interface {
	// StopPlan is Pure: p is the process to stop, table the process-table
	// read it came from (for parent and child links), s what
	// ProcessSessionsOf found for p (zero when nothing; then Resume and
	// Command are nil). An error is a plan that cannot be built (p is not
	// this agent's process).
	StopPlan(p Process, table []Process, s ProcessSession) (StopPlan, error)
}

// DefaultStopGrace is the grace a plan uses when the agent documents no
// flush time of its own: generous enough for a transcript write.
const DefaultStopGrace = 4 * time.Second

// stopRefuseNotSession is the refusal for a process the classifier calls
// a non-session kind.
const stopRefuseNotSession = "not a conversation process: "

// StopPlan is the plan for a config-defined agent (and the base vendors
// refine): refuse a non-session process kind, else terminate p alone with
// DefaultStopGrace; Resume from Spec.ResumeArgs when s names a
// conversation. Pure.
func (c *Core) StopPlan(p Process, _ []Process, s ProcessSession) (StopPlan, error) {
	info, ok := c.ClassifyProcess(p)
	if !ok {
		return StopPlan{}, NotThisAgentProcess(c.spec.ID, p)
	}
	return c.BaseStopPlan(info, p, s), nil
}

// BaseStopPlan is the shared part of every agent's plan: a non-session
// kind is refused with its kind as the reason; a session is one target,
// StopSignalTerminate, DefaultStopGrace, Force, and a Resume built with
// Launch when s names a conversation and a home this agent owns. On a
// windows host (Host.GOOS) Graceful is StopSignalNone: Windows has no
// SIGTERM, and a console control event reaches only processes attached
// to the sender's own console, so another console's agent can only be
// forced (the turn in flight is lost; the transcript up to it is kept).
// Pure.
func (c *Core) BaseStopPlan(info ProcessInfo, p Process, s ProcessSession) StopPlan {
	plan := StopPlan{Agent: c.spec.ID}
	if !info.Session {
		plan.Refuse = stopRefuseNotSession + string(info.Kind)
		return plan
	}
	plan.Targets = []int{p.PID}
	plan.Graceful = StopSignalTerminate
	if c.host.GOOS == OSWindows {
		plan.Graceful = StopSignalNone
	}
	plan.Grace = DefaultStopGrace
	plan.Force = true
	plan.KeepsConversation = true
	if s.ConversationID != "" && !s.Home.IsZero() && c.Owns(s.Home) == nil {
		if cmd, err := c.Launch(Target{Home: s.Home}, LaunchRequest{Dir: s.Dir, ResumeID: s.ConversationID}); err == nil {
			plan.Resume = &cmd
		}
	}
	return plan
}
