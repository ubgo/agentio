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
	if h.Processes == nil {
		return LivenessUnknown, errors.New("agentio: no process lister in host")
	}
	procs, err := h.Processes.Processes(ctx)
	if err != nil {
		return LivenessUnknown, fmt.Errorf("agentio: process table: %w", err)
	}
	i := slices.IndexFunc(procs, func(p Process) bool { return p.PID == pid })
	if i < 0 {
		return LivenessEnded, nil
	}
	p := procs[i]
	startKnown := !startedAt.IsZero() && !p.Start.IsZero()
	if startKnown && p.Start.Sub(startedAt).Abs() > startTolerance {
		return LivenessEnded, nil
	}
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

// processEnv reads pid's start env through ProcessEnvReader. Process · Read.
func (h Host) processEnv(ctx context.Context, pid int) ([]string, error) {
	r, ok := h.Processes.(ProcessEnvReader)
	if !ok {
		return nil, UnsupportedError("", "process env", "the host's process lister cannot read a process environment")
	}
	return r.Env(ctx, pid)
}
