// Command backupmove shows the write helpers every Writer door is built
// on: backing a file up before a change, planning a conversation move
// between two homes of one agent, applying the plan (backup first, rollback
// on failure), and linking shared files of the default home into another
// home. It copies the 2.1.325 fixture user home into a temp dir first and
// only ever writes there.
//
//	cd examples && go run ./backupmove
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultUserHome is the fixture user home, relative to the examples dir.
const defaultUserHome = "../claude/testdata/homes/2.1.325/userhome"

// Inputs taken from the fixture.
const (
	workHomeRel = ".claude-work"
	// movedDir is the working folder whose conversations move; the fixture
	// default home has one conversation recorded for it.
	movedDir = "/work/proj12"
	// blockedDir has a conversation the example also copies into the
	// target first, so its plan has a blocker.
	blockedDir      = "/work/proj13"
	blockedConvFile = "3f474440-903e-5410-8bfb-d537b89f9547.jsonl"
	settingsFile    = "settings.json"
)

func main() {
	userHome := flag.String("home", defaultUserHome, "fixture user home to copy")
	flag.Parse()
	if err := run(os.Stdout, *userHome); err != nil {
		fmt.Fprintln(os.Stderr, "backupmove:", err)
		os.Exit(1)
	}
}

// run copies the fixture into a temp dir and runs every step on the copy.
func run(w io.Writer, fixture string) error {
	tmp, err := os.MkdirTemp("", "agentio-backupmove-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return err
	}
	userHome := filepath.Join(tmp, "userhome")
	if err := copyFixture(fixture, userHome); err != nil {
		return err
	}
	p := printer{w: w, repl: strings.NewReplacer(userHome, "~")}
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	h := agentio.Host{UserHome: userHome, User: "user", Now: func() time.Time { return now }}
	a := claude.New(h)
	def, work := a.DefaultHome(), a.HomeAt(workHomeRel)
	if err := backupFile(p, def.Dir(), now); err != nil {
		return err
	}
	plan, err := planMove(p, a, def.Neutral(), work.Neutral())
	if err != nil {
		return err
	}
	if err := applyMove(p, plan, now); err != nil {
		return err
	}
	if err := blockedMove(p, a, def, work, now); err != nil {
		return err
	}
	return homeLinks(p, h, def.Neutral(), work.Neutral())
}

// snippet:start copy-fixture
// copyFixture copies a whole home tree. CopyTree keeps file modes, copies
// symlinks as links and refuses an existing destination, so it can never
// overwrite anything. Disk · Write.
func copyFixture(src, dst string) error {
	if err := agentio.CopyTree(src, dst); err != nil {
		return fmt.Errorf("copy fixture: %w", err)
	}
	return nil
}

// snippet:end copy-fixture

// snippet:start backup-file
// backupFile saves settings.json before a (pretend) edit. Each backup gets
// a fresh name: a second backup in the same second gets a "-2" suffix
// instead of overwriting the first. A missing file returns an empty Backup.
// Disk · Write.
func backupFile(p printer, home string, now time.Time) error {
	path := filepath.Join(home, settingsFile)
	next, err := agentio.BackupPath(path, now)
	if err != nil {
		return err
	}
	p.line("backup path: " + next)
	first, err := agentio.BackupFile(path, now)
	if err != nil {
		return err
	}
	second, err := agentio.BackupFile(path, now)
	if err != nil {
		return err
	}
	all := first.Merge(second)
	for _, e := range all.Entries {
		p.line("backup: " + e.Original + " -> " + e.Copy)
	}
	missing, err := agentio.BackupFile(filepath.Join(home, "no-such-file.json"), now)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("backup of a missing file: empty=%t", missing.IsEmpty()))
	return nil
}

// snippet:end backup-file

// snippet:start plan-move
// planMove asks the agent what moving one folder's conversations from one
// home to another would do. Planning changes nothing: the plan lists every
// rename, what stays behind, and blockers that stop the move. Disk · Read.
func planMove(p printer, a *claude.Agent, from, to agentio.Home) (agentio.MovePlan, error) {
	plan, err := a.PlanMove(agentio.MoveRequest{From: from, To: to, Dir: movedDir})
	if err != nil {
		return agentio.MovePlan{}, fmt.Errorf("plan: %w", err)
	}
	p.line(fmt.Sprintf("plan: agent=%s dir=%s conversations=%v", plan.Agent, plan.Dir, plan.Conversations))
	for _, it := range plan.Items {
		p.line("plan: move " + it.From + " -> " + it.To)
	}
	p.line(fmt.Sprintf("plan: blockers=%d left=%d cleanup=%d", len(plan.Blockers), len(plan.Left), len(plan.Cleanup)))
	return plan, nil
}

// snippet:end plan-move

// snippet:start apply-move
// applyMove executes the plan: every source item is first copied into one
// backup dir beside the source home, then renamed into place; if a rename
// fails, the items already moved are renamed back. Disk · Write.
func applyMove(p printer, plan agentio.MovePlan, now time.Time) error {
	backup, err := agentio.ApplyMovePlan(plan, now)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	for _, e := range backup.Entries {
		p.line("apply: backed up " + e.Original + " -> " + e.Copy)
	}
	for _, it := range plan.Items {
		_, err := os.Stat(it.To)
		p.line(fmt.Sprintf("apply: %s present=%t", it.To, err == nil))
	}
	return nil
}

// snippet:end apply-move

// snippet:start blocked-move
// blockedMove plans a move whose target already holds the conversation.
// The plan carries a blocker, and ApplyMovePlan refuses it with
// ErrMoveBlocked before a single file is touched. Disk · Write (setup).
func blockedMove(p printer, a *claude.Agent, from, to claude.Home, now time.Time) error {
	src := filepath.Join(a.HistoryDir(from, blockedDir), blockedConvFile)
	dst := filepath.Join(a.HistoryDir(to, blockedDir), blockedConvFile)
	if err := agentio.CopyTree(src, dst); err != nil {
		return err
	}
	plan, err := a.PlanMove(agentio.MoveRequest{From: from.Neutral(), To: to.Neutral(), Dir: blockedDir})
	if err != nil {
		return err
	}
	for _, b := range plan.Blockers {
		p.line("blocked: " + b)
	}
	_, err = agentio.ApplyMovePlan(plan, now)
	p.line(fmt.Sprintf("blocked: apply refused=%t", errors.Is(err, agentio.ErrMoveBlocked)))
	return nil
}

// snippet:end blocked-move

// snippet:start home-links
// homeLinks links the shared files of the default home (CLAUDE.md,
// skills/, settings.json for Claude) into the work home. The plan marks a
// target that already exists; applying never replaces it, so there is
// nothing to back up. Plan is Disk · Read, apply is Disk · Write.
func homeLinks(p printer, h agentio.Host, def, work agentio.Home) error {
	core, err := agentio.NewCore(claude.Spec(), h)
	if err != nil {
		return err
	}
	links, err := core.PlanHomeLinks(def, work)
	if err != nil {
		return err
	}
	for _, l := range links {
		p.line(fmt.Sprintf("link plan: %s exists=%t", l.Name, l.Exists))
	}
	if err := core.ApplyHomeLinks(links); err != nil {
		return err
	}
	for _, l := range links {
		if target, err := os.Readlink(l.To); err == nil {
			p.line("linked: " + l.To + " -> " + target)
		}
	}
	// The package function works on any name list, for two homes of one agent.
	again, err := agentio.PlanHomeLinks(def, work, []string{"CLAUDE.md"})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("link plan again: %s exists=%t", again[0].Name, again[0].Exists))
	return nil
}

// snippet:end home-links

// printer writes lines with the temp user home replaced by "~", so the
// output is the same on every run.
type printer struct {
	w    io.Writer
	repl *strings.Replacer
}

// line prints one line with the home folder shown as ~.
// A write error on the output (a closed pipe) is dropped: the example has
// nothing better to do with it than stop printing.
func (p printer) line(s string) {
	_, _ = fmt.Fprintln(p.w, p.repl.Replace(s))
}
