package pi

import "github.com/ubgo/agentio"

// ClassifyProcess is Pure; pi narrows its short basename (Agent.ClassifyProcess).
func (n neutral) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	return n.a.ClassifyProcess(p)
}
