package codex

import (
	"context"

	"github.com/ubgo/agentio"
)

// Process-session capabilities Codex has.
var (
	_ agentio.ProcessSessionReader = neutral{}
	_ agentio.StopPlanner          = neutral{}
)

// ProcessSession is Agent.ProcessSession. Disk · Read + Process · Read.
func (n neutral) ProcessSession(ctx context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	return n.a.ProcessSession(ctx, p)
}

// StopPlan is Agent.StopPlan. Pure.
func (n neutral) StopPlan(p agentio.Process, table []agentio.Process, s agentio.ProcessSession) (agentio.StopPlan, error) {
	return n.a.StopPlan(p, table, s)
}
