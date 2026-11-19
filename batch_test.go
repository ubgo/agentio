package agentio_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// batchAgent is a config agent with a canned ProcessSessionBatchReader
// beside its row-by-row reader, recording which form was asked.
type batchAgent struct {
	readerAgent
	batches *[][]int
	err     error
}

// ProcessSessions records the pids and returns the canned session for
// each. Pure.
func (b batchAgent) ProcessSessions(_ context.Context, ps []agentio.Process) (map[int]agentio.ProcessSession, error) {
	*b.batches = append(*b.batches, pids(ps))
	out := map[int]agentio.ProcessSession{}
	for _, p := range ps {
		s := b.session
		s.PID = p.PID
		out[p.PID] = s
	}
	return out, b.err
}

// TestFindProcessSessions groups session rows per agent, asks a batch
// reader once per group and a row reader per row, skips non-session and
// unclaimed rows, and keeps the found rows when a batch fails.
func TestFindProcessSessions(t *testing.T) {
	t.Parallel()
	host := agenttest.NewHost(t)
	toySpec := sampleSpec()
	toySpec.Process = agentio.ProcessMatch{Basenames: []string{"toy"}}
	toyCore, err := agentio.NewCore(toySpec, host)
	if err != nil {
		t.Fatal(err)
	}
	other := sampleSpec()
	other.ID, other.Binary, other.Process = "other", "other", agentio.ProcessMatch{Basenames: []string{"other"}}
	otherCore, err := agentio.NewCore(other, host)
	if err != nil {
		t.Fatal(err)
	}
	var rowAsked, batchRowAsked []int
	var batches [][]int
	batchFail := errors.New("batch read failed")
	toy := batchAgent{
		readerAgent: readerAgent{Core: toyCore, got: &batchRowAsked, session: agentio.ProcessSession{Agent: "toy", ConversationID: "t1", Confidence: agentio.JoinExact}},
		batches:     &batches, err: batchFail,
	}
	row := readerAgent{Core: otherCore, got: &rowAsked, session: agentio.ProcessSession{Agent: "other", ConversationID: "o1", Confidence: agentio.JoinArgv}}
	table := []agentio.Process{
		{PID: 1, Argv: []string{"toy"}}, {PID: 2, Argv: []string{"other"}}, {PID: 3, Argv: []string{"toy", "--x"}},
		{PID: 4, Argv: []string{"vim"}}, {PID: 5, Argv: []string{"other"}},
	}
	got, err := agentio.FindProcessSessions(context.Background(), []agentio.Agent{toy, row}, table)
	if !errors.Is(err, batchFail) {
		t.Errorf("batch error not returned: %v", err)
	}
	if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, []int{1, 2, 3, 5}) {
		t.Fatalf("found pids %v", keys)
	}
	if got[1].ConversationID != "t1" || got[3].PID != 3 || got[2].ConversationID != "o1" || got[5].PID != 5 {
		t.Errorf("sessions = %+v", got)
	}
	if len(batches) != 1 || !slices.Equal(batches[0], []int{1, 3}) || len(batchRowAsked) != 0 {
		t.Errorf("toy asked as batches %v and rows %v, want one batch [1 3]", batches, batchRowAsked)
	}
	if !slices.Equal(rowAsked, []int{2, 5}) {
		t.Errorf("row reader asked %v", rowAsked)
	}
	// The single-row form agrees with the batch for a claimed row.
	if s, ok, err := agentio.FindProcessSession(context.Background(), []agentio.Agent{toy, row}, table[0]); err != nil || !ok || s.ConversationID != "t1" {
		t.Errorf("single row = %+v %v %v", s, ok, err)
	}
	if _, ok := agentio.ProcessSessionBatchOf(row); ok {
		t.Error("a row-only reader has no batch form")
	}
}

// TestProcessHomes reads every environment with one batch call and
// leaves a pid with an unreadable environment out (unknown, never the
// default).
func TestProcessHomes(t *testing.T) {
	t.Parallel()
	procs := &agenttest.FakeProcesses{Envs: map[int][]string{
		1: {"PATH=/bin", "TOY_HOME=/work/toy-home"},
		2: {"PATH=/bin"},
	}}
	host := agenttest.NewHost(t, agenttest.WithProcesses(procs))
	c, err := agentio.NewCore(sampleSpec(), host)
	if err != nil {
		t.Fatal(err)
	}
	homes, err := c.ProcessHomes(context.Background(), []int{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if h, ok := homes[1]; !ok || h.Dir() != c.HomeAt("/work/toy-home").Dir() {
		t.Errorf("env home = %v %v", h, ok)
	}
	if h, ok := homes[2]; !ok || !h.IsDefault() {
		t.Errorf("default home = %v %v", h, ok)
	}
	if _, ok := homes[3]; ok {
		t.Error("an unreadable env must leave the pid out")
	}
	if procs.EnvBatchCalls != 1 || procs.EnvCalls != 0 {
		t.Errorf("env reads: %d batch, %d single; want 1 batch", procs.EnvBatchCalls, procs.EnvCalls)
	}
	// A lister with only the single-pid reader is asked once per pid.
	single := &envOnly{f: &agenttest.FakeProcesses{Envs: procs.Envs}}
	c2, _ := agentio.NewCore(sampleSpec(), agenttest.NewHost(t, agenttest.WithProcesses(single)))
	homes, err = c2.ProcessHomes(context.Background(), []int{1, 2, 3})
	if err != nil || len(homes) != 2 || single.f.EnvCalls != 3 {
		t.Errorf("single-pid fallback: %v %v, %d env calls", homes, err, single.f.EnvCalls)
	}
	// No env reader at all: an error, no homes.
	if _, err := c2.ProcessHomes(context.Background(), nil); err != nil {
		t.Errorf("empty pid list: %v", err)
	}
	none, _ := agentio.NewCore(sampleSpec(), agenttest.NewHost(t, agenttest.WithProcesses(tableOnly{})))
	if h, err := none.ProcessHomes(context.Background(), []int{1}); !errors.Is(err, agentio.ErrUnsupported) || h != nil {
		t.Errorf("no env reader: %v %v", h, err)
	}
}

// envOnly exposes a fake's single-pid Env but not EnvBatch.
type envOnly struct{ f *agenttest.FakeProcesses }

func (e *envOnly) Processes(ctx context.Context) ([]agentio.Process, error) {
	return e.f.Processes(ctx)
}

func (e *envOnly) Cwd(ctx context.Context, pid int) (string, error) { return e.f.Cwd(ctx, pid) }

func (e *envOnly) Env(ctx context.Context, pid int) ([]string, error) { return e.f.Env(ctx, pid) }

// tableOnly is a lister with no env reader and an empty table.
type tableOnly struct{}

func (tableOnly) Processes(context.Context) ([]agentio.Process, error) { return nil, nil }

func (tableOnly) Cwd(context.Context, int) (string, error) { return "", agentio.ErrNotFound }

// TestProcessCwd uses a filled Process.Cwd without asking the lister.
func TestProcessCwd(t *testing.T) {
	t.Parallel()
	procs := &agenttest.FakeProcesses{Cwds: map[int]string{7: "/work/looked-up"}}
	h := agentio.Host{Processes: procs}
	ctx := context.Background()
	if d, err := h.ProcessCwd(ctx, agentio.Process{PID: 7, Cwd: "/work/known"}); err != nil || d != "/work/known" || procs.CwdCalls != 0 {
		t.Errorf("filled cwd = %q %v, %d lookups", d, err, procs.CwdCalls)
	}
	if d, err := h.ProcessCwd(ctx, agentio.Process{PID: 7}); err != nil || d != "/work/looked-up" || procs.CwdCalls != 1 {
		t.Errorf("looked-up cwd = %q %v, %d lookups", d, err, procs.CwdCalls)
	}
	if _, err := (agentio.Host{}).ProcessCwd(ctx, agentio.Process{PID: 7}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("no lister: %v", err)
	}
}

// TestProcessesAlive re-checks many refs with one table read: gone,
// reused, confirmed within the tolerance, and unconfirmable rows.
func TestProcessesAlive(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	procs := &agenttest.FakeProcesses{List: []agentio.Process{{PID: 10, Start: start}, {PID: 11, Start: start}, {PID: 12}}}
	h := agentio.Host{Processes: procs}
	refs := []agentio.ProcessRef{
		{PID: 10, Start: start.Add(time.Second)}, // same process, darwin whole-second clock
		{PID: 11, Start: start.Add(time.Hour)},   // pid reused
		{PID: 12, Start: start},                  // table has no start time
		{PID: 13, Start: start},                  // gone
		{PID: 10, Start: start},                  // asked twice: one answer per ref
		{PID: 11},                                // no recorded start time
	}
	got, err := h.ProcessesAlive(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	want := []agentio.Liveness{agentio.LivenessAlive, agentio.LivenessEnded, agentio.LivenessUnknown, agentio.LivenessEnded, agentio.LivenessAlive, agentio.LivenessUnknown}
	if !slices.Equal(got, want) {
		t.Errorf("ProcessesAlive = %v, want %v", got, want)
	}
	if procs.TableReads != 1 {
		t.Errorf("table reads = %d, want 1", procs.TableReads)
	}
	procs.Err = errors.New("ps failed")
	if got, err := h.ProcessesAlive(context.Background(), refs); err == nil || got != nil {
		t.Errorf("table error = %v %v", got, err)
	}
	if _, err := (agentio.Host{}).ProcessesAlive(context.Background(), refs); err == nil {
		t.Error("no lister must fail")
	}
}

// TestProcessAliveUsesBatch: the single-pid check reads the table once,
// through the batch check.
func TestProcessAliveUsesBatch(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	procs := &agenttest.FakeProcesses{List: []agentio.Process{{PID: 10, Start: start}}}
	h := agentio.Host{Processes: procs}
	if got, err := h.ProcessAlive(context.Background(), 10, start, "", ""); err != nil || got != agentio.LivenessAlive || procs.TableReads != 1 {
		t.Errorf("ProcessAlive = %v %v, %d table reads", got, err, procs.TableReads)
	}
}

// TestShellLine prints the home variable for a non-default home, leaves
// it out for the default home, and never prints a token or any other env
// value, whatever its name.
func TestShellLine(t *testing.T) {
	t.Parallel()
	_, host := newToy(t)
	secret := "alice-dummy-secret-value"
	host.Env = append(host.Env, "TOY_TOKEN_EXTRA="+secret, "PLAIN=visible-only-in-env")
	toy, err := agentio.FromSpec(sampleSpec(), host)
	if err != nil {
		t.Fatal(err)
	}
	homes, err := toy.Homes()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(homes, func(h agentio.Home) bool { return !h.IsDefault() && strings.HasSuffix(h.Dir(), ".toy-work") })
	if i < 0 {
		t.Fatalf("no work home in %v", homes)
	}
	work, l := homes[i], toy
	cmd, err := l.Launch(agentio.Target{Home: work}, agentio.LaunchRequest{ResumeID: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cmd.ShellLine(), "TOY_HOME="+posixQuote(work.Dir())+" toy --resume abc"; got != want {
		t.Errorf("non-default home: %q, want %q", got, want)
	}
	cmd, err = l.Launch(agentio.Target{Home: toy.DefaultHome()}, agentio.LaunchRequest{ResumeID: "abc", Dir: "/work/alice proj"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cmd.ShellLine(), `cd "/work/alice proj" && toy --resume abc`; got != want {
		t.Errorf("default home: %q, want %q", got, want)
	}
	// A token set in the env (a secret source, and a credential name set
	// again with a plain value) never reaches the line.
	tok := agentio.NewToken(secret)
	withSecrets := agentio.NewCommand("toy", []string{"--resume", "abc"}, host.Env, "",
		agentio.EnvSetSecret("TOY_HOME", tok), agentio.EnvSetSecret("TOY_API_KEY", tok),
		agentio.EnvUnset("TOY_TOKEN"), agentio.EnvSet("TOY_TOKEN", secret))
	if line := withSecrets.ShellLine(); strings.Contains(line, secret) || strings.Contains(line, "visible-only") || line != "toy --resume abc" {
		t.Errorf("secret env leaked or line wrong: %q", line)
	}
	// A home change overridden later by a secret is not printed either.
	home, _ := agentio.HomeEnvChange("TOY_HOME", work)
	overridden := agentio.NewCommand("toy", nil, nil, "", home, agentio.EnvSetSecret("TOY_HOME", tok))
	if line := overridden.ShellLine(); strings.Contains(line, secret) || line != "toy" {
		t.Errorf("overridden home: %q", line)
	}
	if line := overridden.Redacted().ShellLine(); line != "toy" {
		t.Errorf("redacted copy: %q", line)
	}
	// StopPlan.ResumeLine is the resume's line, "" without a resume.
	if (agentio.StopPlan{}).ResumeLine() != "" {
		t.Error("no resume, no line")
	}
	resume, _ := l.Launch(agentio.Target{Home: work}, agentio.LaunchRequest{ResumeID: "abc"})
	if got := (agentio.StopPlan{Resume: &resume}).ResumeLine(); got != resume.ShellLine() || !strings.HasPrefix(got, "TOY_HOME=") {
		t.Errorf("ResumeLine = %q", got)
	}
}

// posixQuote quotes s the way Command.String does (a Windows temp path
// with backslashes is quoted, a unix one is not). Pure.
func posixQuote(s string) string { return agentio.Command{Path: s}.String() }
