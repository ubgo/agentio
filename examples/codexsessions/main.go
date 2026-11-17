// Command codexsessions answers "is Codex running, and on which
// conversation?" from a copy of the checked-in fixture home plus a fake
// process table: thread-writer locks, live sessions, the process list and
// its classification, background chat processes and the daemon socket.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/codex"
)

// defaultFixture is the checked-in Codex home, relative to the examples module.
const defaultFixture = "../codex/testdata/homes/0.162.1"

// Fixture facts.
const (
	codexDir    = ".codex"
	fixtureUser = "user"
	filePerm    = os.FileMode(0o600)
	dirPerm     = os.FileMode(0o700)
	// liveThread has a rollout in the fixture; staleThread has none.
	liveThread  = "0199e1a0-0000-7000-8000-000000000001"
	staleThread = "01a1d83a-e1cb-70d1-bee7-7aebab05bc2b"
	lockSuffix  = ".lock"
)

// Process-table times: one codex started before the live lock was taken,
// the stale lock is older than every running codex. Functions, not vars:
// the repo allows no package-level vars.
func liveLockTime() time.Time  { return time.Date(2026, 11, 13, 11, 0, 0, 0, time.UTC) }
func staleLockTime() time.Time { return time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC) }
func codexStart() time.Time    { return time.Date(2026, 11, 13, 10, 59, 0, 0, time.UTC) }

// chatProcessesJSON is a process_manager/chat_processes.json entry
// (keys recorded from Codex Desktop; values replaced).
const chatProcessesJSON = `[{"id":"cp1","processId":"p-1","osPid":4242,"command":"npm run dev","cwd":"/work/proj",
"conversationId":"0199e1a0-0000-7000-8000-000000000001","chatTitle":"List the files","turnId":"t1","itemId":"i9",
"startedAtMs":1794567600000,"updatedAtMs":1794567660000}]`

func main() {
	fixture := flag.String("fixture", defaultFixture, "Codex fixture home to copy")
	flag.Parse()
	if err := run(os.Stdout, *fixture); err != nil {
		fmt.Fprintln(os.Stderr, "codexsessions:", err)
		os.Exit(1)
	}
}

// run lays out the home and the fake process table, then prints.
func run(w io.Writer, fixture string) error {
	userHome, err := os.MkdirTemp("", "codexsessions-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	home := filepath.Join(userHome, codexDir)
	if err := agentio.CopyTree(fixture, home); err != nil {
		return err
	}
	locks := filepath.Join(home, "thread-writer-locks")
	for id, ts := range map[string]time.Time{liveThread: liveLockTime(), staleThread: staleLockTime()} {
		p := filepath.Join(locks, id+lockSuffix)
		if err := os.WriteFile(p, nil, filePerm); err != nil {
			return err
		}
		if err := os.Chtimes(p, ts, ts); err != nil {
			return err
		}
	}
	chat := filepath.Join(home, "process_manager", "chat_processes.json")
	if err := os.MkdirAll(filepath.Dir(chat), dirPerm); err != nil {
		return err
	}
	if err := os.WriteFile(chat, []byte(chatProcessesJSON), filePerm); err != nil {
		return err
	}
	procs := &agenttest.FakeProcesses{
		List: []agentio.Process{
			{PID: 101, Start: codexStart(), Exe: "/opt/codex/bin/codex", Argv: []string{"codex"}},
			{PID: 102, Start: codexStart(), Exe: "/opt/codex/bin/codex", Argv: []string{"codex", "exec", "--json", "-"}},
			{PID: 103, Start: codexStart(), Exe: "/usr/bin/node", Argv: []string{"node", "/usr/lib/node_modules/@openai/codex/bin/codex.js"}},
			{PID: 104, Start: codexStart(), Exe: "/opt/codex/bin/codex", Argv: []string{"codex", "app-server", "daemon"}},
			{PID: 105, Start: codexStart(), Exe: "/usr/bin/vim", Argv: []string{"vim"}},
		},
		Cwds: map[int]string{101: "/work/proj"},
	}
	host := agentio.Host{
		UserHome:  userHome,
		User:      fixtureUser,
		Env:       []string{"HOME=" + userHome},
		Now:       agenttest.FixedNow,
		Secrets:   agenttest.NewFakeSecrets(),
		Processes: procs,
	}
	a := codex.New(host)
	h := a.DefaultHome()
	if err := listLiveSessions(w, a, h); err != nil {
		return err
	}
	if err := readThreadLocks(w, a, h); err != nil {
		return err
	}
	return listProcesses(w, a, h, procs.List)
}

// snippet:start live
// listLiveSessions turns thread-writer locks into live sessions. A lock
// counts as Alive only when a Codex process that started no later than
// the lock is running; otherwise it is a crash leftover. PID stays 0:
// the lock file does not say which process holds it.
func listLiveSessions(w io.Writer, a *codex.Agent, h codex.Home) error {
	ls, err := a.LiveSessions(h)
	if err != nil {
		return err
	}
	for _, s := range ls {
		fmt.Fprintf(w, "session %s alive=%v pid=%d dir=%q status=%s started=%s\n",
			s.ConversationID, s.Alive, s.PID, s.Dir, s.Status, s.StartedAt.UTC().Format(time.RFC3339))
	}
	for _, id := range []string{liveThread, staleThread} {
		live, err := a.IsLive(h, id)
		if err != nil {
			return err
		}
		exists, err := a.LiveConversation(h, id)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s IsLive=%v lock file exists=%v\n", id, live, exists)
	}
	return nil
}

// snippet:end live

// snippet:start locks
// readThreadLocks lists the lock files themselves; .coordination.lock and
// any name that is not <uuid>.lock are skipped.
func readThreadLocks(w io.Writer, a *codex.Agent, h codex.Home) error {
	tl, err := a.ThreadLocks(h)
	if err != nil {
		return err
	}
	for _, l := range tl {
		fmt.Fprintf(w, "lock %s taken=%s\n", l.ThreadID, l.ModTime.UTC().Format(time.RFC3339))
	}
	for _, name := range []string{staleThread + lockSuffix, ".coordination.lock", "notes.lock"} {
		id, ok := codex.ParseThreadLockName(name)
		fmt.Fprintf(w, "ParseThreadLockName(%q) = %q, %v\n", name, id, ok)
	}
	return nil
}

// snippet:end locks

// snippet:start processes
// listProcesses classifies running Codex processes (one process-table
// read), then reads background chat processes and the daemon socket.
func listProcesses(w io.Writer, a *codex.Agent, h codex.Home, table []agentio.Process) error {
	ps, err := a.Processes(context.Background())
	if err != nil {
		return err
	}
	for _, p := range ps {
		fmt.Fprintf(w, "process pid=%d kind=%s cwd=%q\n", p.PID, p.Kind, p.Cwd)
	}
	for _, p := range table {
		info, ok := a.ClassifyProcess(p)
		fmt.Fprintf(w, "classify pid=%d codex=%v kind=%s session=%v\n", p.PID, ok, info.Kind, info.Session)
	}

	chats, err := a.ChatProcesses(h)
	if err != nil {
		return err
	}
	for _, c := range chats {
		fmt.Fprintf(w, "chat process %q pid=%d conversation=%s started=%s\n",
			c.Command, c.OSPid, c.ConversationID, c.StartedAt.UTC().Format(time.RFC3339))
	}
	_, ok := a.DaemonSocket(h)
	fmt.Fprintf(w, "daemon socket present: %v\n", ok)
	return nil
}

// snippet:end processes
