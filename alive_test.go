package agentio_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestProcessAlive covers every branch agx clean relies on: gone, reused
// (start time or env), confirmed, and unverifiable.
func TestProcessAlive(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	procs := &agenttest.FakeProcesses{
		List: []agentio.Process{{PID: 10, Start: start}, {PID: 11, Start: start}, {PID: 12}},
		Envs: map[int][]string{10: {"PATH=/bin", "AGX_RUN_ID=r1"}, 11: {"PATH=/bin"}},
	}
	h := agentio.Host{Processes: procs}
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		pid     int
		started time.Time
		key     string
		want    agentio.Liveness
	}{
		{"gone", 99, start, "AGX_RUN_ID", agentio.LivenessEnded},
		{"start differs", 10, start.Add(time.Hour), "AGX_RUN_ID", agentio.LivenessEnded},
		{"start within tolerance and env matches", 10, start.Add(time.Second), "AGX_RUN_ID", agentio.LivenessAlive},
		{"env lacks the run id", 11, start, "AGX_RUN_ID", agentio.LivenessEnded},
		{"start only", 11, start, "", agentio.LivenessAlive},
		{"no start, no env", 12, time.Time{}, "AGX_RUN_ID", agentio.LivenessUnknown},
	} {
		got, err := h.ProcessAlive(ctx, tc.pid, tc.started, tc.key, "r1")
		if got != tc.want {
			t.Errorf("%s: %v (%v), want %v", tc.name, got, err, tc.want)
		}
		if (got == agentio.LivenessUnknown) != (err != nil) {
			t.Errorf("%s: unknown must carry its reason: %v %v", tc.name, got, err)
		}
	}
	// An env that cannot be read (windows) still confirms by start time.
	procs.Envs = nil
	if got, err := h.ProcessAlive(ctx, 11, start, "AGX_RUN_ID", "r1"); got != agentio.LivenessAlive {
		t.Errorf("start match without env = %v %v", got, err)
	}
	if got, err := h.ProcessAlive(ctx, 12, time.Time{}, "AGX_RUN_ID", "r1"); got != agentio.LivenessUnknown || !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("nothing readable = %v %v", got, err)
	}
	if got, _ := (agentio.Host{}).ProcessAlive(ctx, 1, start, "", ""); got != agentio.LivenessUnknown {
		t.Errorf("no lister = %v", got)
	}
}
