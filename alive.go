package agentio

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// startTolerance is how far a process's start time may be from the
// recorded one and still be the same process: darwin `ps` reports start
// times to the second and linux to the clock tick, and a caller may record
// its own clock a moment before or after the exec.
const startTolerance = 2 * time.Second

// ProcessAlive reports whether the process a caller recorded (pid, its
// start time, and a variable it set in the process's env, such as
// AGX_RUN_ID=<run id>) still runs. It exists for cleaning up per-run files
// (agx clean) without deleting the files of a live agent:
//   - the pid is gone: ended;
//   - the pid runs but started at another time (beyond startTolerance), or
//     its start env is readable and lacks envKey=envValue: ended (the pid
//     was reused);
//   - the start time matches, or the env holds envKey=envValue: alive;
//   - otherwise unknown, with the reason as the error (no process table,
//     no start time, an env that cannot be read on this OS).
//
// A zero startedAt or an empty envKey skips that check. Record startedAt
// from Host.Processes (Process.Start) when possible: it is the clock the
// comparison uses. Process · Read (one process-table read, plus one env
// read when the start time does not decide).
func (h Host) ProcessAlive(ctx context.Context, pid int, startedAt time.Time, envKey, envValue string) (Liveness, error) {
	lives, err := h.ProcessesAlive(ctx, []ProcessRef{{PID: pid, Start: startedAt}})
	if err != nil {
		return LivenessUnknown, err
	}
	live := lives[0]
	if live == LivenessEnded {
		return LivenessEnded, nil
	}
	startKnown := live == LivenessAlive
	if envKey == "" {
		if startKnown {
			return LivenessAlive, nil
		}
		return LivenessUnknown, errors.New("agentio: pid runs but neither a start time nor an env key can confirm it")
	}
	env, envErr := h.processEnv(ctx, pid)
	switch {
	case envErr == nil && slices.Contains(env, envKey+"="+envValue):
		return LivenessAlive, nil
	case envErr == nil:
		return LivenessEnded, nil
	case startKnown:
		return LivenessAlive, nil
	}
	return LivenessUnknown, fmt.Errorf("agentio: pid %d runs, start time unknown, env unreadable: %w", pid, envErr)
}

// ProcessRef names one process a caller recorded: its pid and the start
// time read with it (Process.Start), which tells it from a later process
// that reused the pid.
type ProcessRef struct {
	PID   int
	Start time.Time
}

// ProcessesAlive re-checks many recorded processes with one
// process-table read (a reaper re-checking every row right before it
// signals). The result has one answer per ref, in the order of refs (a
// pid asked twice with two start times gets two answers):
//   - LivenessEnded: the pid is gone, or runs with a start time more than
//     startTolerance from the recorded one (the pid was reused);
//   - LivenessAlive: the pid runs with the recorded start time;
//   - LivenessUnknown: the pid runs but the ref or the table has no start
//     time, so nothing confirms it is the same process.
//
// Record Start from Host.Processes: it is the clock the comparison uses.
// The error is for a table that cannot be read (then the result is nil).
// Process · Read (one process-table read).
func (h Host) ProcessesAlive(ctx context.Context, refs []ProcessRef) ([]Liveness, error) {
	if h.Processes == nil {
		return nil, errors.New("agentio: no process lister in host")
	}
	procs, err := h.Processes.Processes(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentio: process table: %w", err)
	}
	starts := make(map[int]time.Time, len(procs))
	for _, p := range procs {
		starts[p.PID] = p.Start
	}
	out := make([]Liveness, len(refs))
	for i, r := range refs {
		start, runs := starts[r.PID]
		switch {
		case !runs:
			out[i] = LivenessEnded
		case r.Start.IsZero() || start.IsZero():
			out[i] = LivenessUnknown
		case start.Sub(r.Start).Abs() > startTolerance:
			out[i] = LivenessEnded
		default:
			out[i] = LivenessAlive
		}
	}
	return out, nil
}

// processEnv reads pid's start env through ProcessEnvReader. Process · Read.
func (h Host) processEnv(ctx context.Context, pid int) ([]string, error) {
	r, ok := h.Processes.(ProcessEnvReader)
	if !ok {
		return nil, UnsupportedError("", "process env", "the host's process lister cannot read a process environment")
	}
	return r.Env(ctx, pid)
}

// processEnvs reads the start env of every pid: one ProcessEnvBatchReader
// read when the lister has it, else Env per pid. A pid whose env cannot
// be read is missing from the map; the error is set only when nothing
// could be asked (no env reader, or the batch read failed as a whole).
// Process · Read.
func (h Host) processEnvs(ctx context.Context, pids []int) (map[int][]string, error) {
	if r, ok := h.Processes.(ProcessEnvBatchReader); ok {
		return r.EnvBatch(ctx, pids)
	}
	r, ok := h.Processes.(ProcessEnvReader)
	if !ok {
		return nil, UnsupportedError("", "process env", "the host's process lister cannot read a process environment")
	}
	out := make(map[int][]string, len(pids))
	for _, pid := range pids {
		if env, err := r.Env(ctx, pid); err == nil {
			out[pid] = env
		}
	}
	return out, nil
}
