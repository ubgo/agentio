package claude

import (
	"context"

	"github.com/ubgo/agentio"
)

// Process-session capabilities Claude has.
var (
	_ agentio.ProcessSessionReader      = neutral{}
	_ agentio.ProcessSessionBatchReader = neutral{}
	_ agentio.StopPlanner               = neutral{}
)

// ProcessSession is Agent.ProcessSession. cost: Disk · Read + Process · Read.
func (n neutral) ProcessSession(ctx context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	return n.a.ProcessSession(ctx, p)
}

// ProcessSessions is Agent.ProcessSessions. cost: Disk · Read + Process ·
// Read (one environment read for every row).
func (n neutral) ProcessSessions(ctx context.Context, ps []agentio.Process) (map[int]agentio.ProcessSession, error) {
	return n.a.ProcessSessions(ctx, ps)
}

// StopPlan is Agent.StopPlan. cost: Pure.
func (n neutral) StopPlan(p agentio.Process, table []agentio.Process, s agentio.ProcessSession) (agentio.StopPlan, error) {
	return n.a.StopPlan(p, table, s)
}
