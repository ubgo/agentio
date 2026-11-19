package agentio_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestProcessesAliveExactStart: a start one second off is the same
// process by default (whole-second clocks) and another process with
// ExactStart; an equal start is alive either way.
func TestProcessesAliveExactStart(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	procs := &agenttest.FakeProcesses{List: []agentio.Process{{PID: 10, Start: start}, {PID: 12}}}
	h := agentio.Host{Processes: procs}
	refs := []agentio.ProcessRef{
		{PID: 10, Start: start},
		{PID: 10, Start: start.Add(time.Second)},
		{PID: 10, Start: start.In(time.FixedZone("alice", 3600))}, // same instant, another zone
		{PID: 12, Start: start},
		{PID: 99, Start: start},
	}
	tolerant, err := h.ProcessesAlive(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	exact, err := h.ProcessesAlive(context.Background(), refs, agentio.ExactStart())
	if err != nil {
		t.Fatal(err)
	}
	alive, ended, unknown := agentio.LivenessAlive, agentio.LivenessEnded, agentio.LivenessUnknown
	if want := []agentio.Liveness{alive, alive, alive, unknown, ended}; !slices.Equal(tolerant, want) {
		t.Errorf("tolerant = %v, want %v", tolerant, want)
	}
	if want := []agentio.Liveness{alive, ended, alive, unknown, ended}; !slices.Equal(exact, want) {
		t.Errorf("exact = %v, want %v", exact, want)
	}
}

// TestFakeProcessesConcurrent calls every FakeProcesses method from many
// goroutines; run with -race (task check does) to prove the counters and
// reads are locked.
func TestFakeProcessesConcurrent(t *testing.T) {
	t.Parallel()
	procs := &agenttest.FakeProcesses{
		List: []agentio.Process{{PID: 1}},
		Cwds: map[int]string{1: "/work/alice"},
		Envs: map[int][]string{1: {"PATH=/bin"}},
	}
	h := agentio.Host{Processes: procs}
	ctx := context.Background()
	const workers = 16
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = procs.Cwd(ctx, 1)
			_, _ = h.ProcessCwd(ctx, agentio.Process{PID: 1})
			_, _ = procs.Env(ctx, 1)
			_, _ = procs.EnvBatch(ctx, []int{1})
			_, _ = procs.Processes(ctx)
			_ = procs.Counts()
		}()
	}
	wg.Wait()
	if c := procs.Counts(); c.CwdCalls != 2*workers || c.EnvCalls != workers || c.EnvBatchCalls != workers || c.TableReads != workers {
		t.Errorf("counts = %+v", c)
	}
}
