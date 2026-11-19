package pi

import (
	"slices"

	"github.com/ubgo/agentio"
)

// ClassifyProcess says whether p is pi. The basename "pi" is short and
// other programs carry it (CLN's `pi 1000` prints digits of pi), so a
// basename match counts only for a row pi's own title wrote: process.title
// = "pi" overwrites the whole argv area, leaving argv exactly ["pi"], or
// the exe is an interpreter (linux /proc/<pid>/exe is node). A row that
// runs the package's cli.js through node (windows, where the title is the
// console's) matches by its script path. Every match is an interactive or
// headless pi session: pi has no daemon or server mode. Pure.
func (a *Agent) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	m := Spec().Process
	if !m.Match(p) {
		return agentio.ProcessInfo{}, false
	}
	if _, script := m.ScriptIndex(p); !script && !titleRow(p) {
		return agentio.ProcessInfo{}, false
	}
	return agentio.ProcessInfo{Agent: ID, Kind: agentio.ProcessCLI, Session: true}, true
}

// titleRow reports a row whose argv is pi's process title: exactly one
// argument, or an exe that is an interpreter (the title replaced argv but
// /proc/<pid>/exe still names node). Pure.
func titleRow(p agentio.Process) bool {
	if len(p.Argv) == 1 {
		return true
	}
	return p.Exe != "" && slices.ContainsFunc(agentio.DefaultInterpreters(), func(n string) bool { return agentio.ProcessBase(p.Exe) == n })
}
