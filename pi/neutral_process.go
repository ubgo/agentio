package pi

import (
	"context"

	"github.com/ubgo/agentio"
)

// Process capabilities pi has.
var (
	_ agentio.ProcessClassifier    = neutral{}
	_ agentio.ProcessSessionReader = neutral{}
	_ agentio.StopPlanner          = neutral{}
)

// ClassifyProcess is Pure; pi narrows its short basename (Agent.ClassifyProcess).
func (n neutral) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	return n.a.ClassifyProcess(p)
}

// ProcessSession is Agent.ProcessSession. Process · Read + Disk · Read.
func (n neutral) ProcessSession(ctx context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	return n.a.ProcessSession(ctx, p)
}

// StopPlan is Agent.StopPlan. Pure.
func (n neutral) StopPlan(p agentio.Process, table []agentio.Process, s agentio.ProcessSession) (agentio.StopPlan, error) {
	return n.a.StopPlan(p, table, s)
}
