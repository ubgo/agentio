// Command pick chooses the agent home with the most headroom with the
// agentio/pick policy: it reads each home's usage (live from a fake vendor
// API, then cached from disk), builds pick.Candidates, picks the best with
// pick.Best, explains the choice and builds the launch command for the
// chosen home. A custom policy (a reserve threshold) shows how to plug in
// your own rule. Everything runs on copies of the fixture homes and an
// in-process usage server: no real network, account or quota.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/pick"
)

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pick:", err)
		os.Exit(1)
	}
}

// runMain starts the fake API, copies the fixtures and runs every step.
// Disk · Write (temp dir).
func runMain(w io.Writer) error {
	srv := fakeAPI()
	defer srv.Close()
	tmp, err := os.MkdirTemp("", "agentio-pick-")
	if err != nil {
		return err
	}
	// The temp dir holds only copies; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	h, err := setup(tmp, srv.Client())
	if err != nil {
		return err
	}
	return run(context.Background(), w, h, srv.URL)
}

// run prints each step. baseURL is the fake API origin both agents are
// pointed at, so no request leaves the process.
func run(ctx context.Context, w io.Writer, h agentio.Host, baseURL string) error {
	agents := []agentio.Agent{claude.New(h).WithBaseURL(baseURL).Neutral(), codex.New(h).WithBaseURL(baseURL).Neutral()}
	tidy := strings.NewReplacer(h.UserHome, "~")
	fmt.Fprintln(w, "== live usage ==")
	live, owners, err := liveCandidates(ctx, agents)
	if err != nil {
		return err
	}
	res, err := pick.Best(live)
	if err != nil {
		return err
	}
	printResult(w, res)
	if err := launchChosen(w, res, owners, tidy); err != nil {
		return err
	}
	fmt.Fprintln(w, "== cached usage ==")
	cached, err := cachedCandidates(agents)
	if err != nil {
		return err
	}
	res, err = pick.Best(cached, pick.WithNow(h.Now()))
	if err != nil {
		return err
	}
	printResult(w, res)
	fmt.Fprintln(w, "== custom policy ==")
	res, err = pick.Best(live, pick.WithPolicy(reserve{limit: reserveLimit}))
	if err != nil {
		return err
	}
	printResult(w, res)
	fmt.Fprintln(w, "== nothing eligible ==")
	_, err = pick.Best(live, pick.WithPolicy(reserve{limit: 1}))
	fmt.Fprintf(w, "no candidate: %t\n%v\n", errors.Is(err, pick.ErrNoCandidate), err)
	return nil
}

// snippet:start live
// liveCandidates reads live usage for every home of every agent that has
// a UsageReader: one network request per home, which may 429, so call it
// once per user action and never in a loop. A read error is kept on the
// candidate (pick skips it with the reason). owners maps a home back to
// its agent, to launch it once chosen.
func liveCandidates(ctx context.Context, agents []agentio.Agent) ([]pick.Candidate, map[agentio.Home]agentio.Agent, error) {
	var cands []pick.Candidate
	owners := map[agentio.Home]agentio.Agent{}
	for _, a := range agents {
		ur, ok := agentio.UsageOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return nil, nil, err
		}
		for _, home := range homes {
			u, err := ur.Usage(ctx, home)
			cands = append(cands, pick.Candidate{Home: home, Usage: u, Err: err})
			owners[home] = a
		}
	}
	return cands, owners, nil
}

// snippet:end live

// snippet:start launch
// launchChosen builds the launch command on the chosen home: its Target
// carries the home, so the child gets the right CLAUDE_CONFIG_DIR (or none
// for the default home). The command is printed, never run.
func launchChosen(w io.Writer, res pick.Result, owners map[agentio.Home]agentio.Agent, tidy *strings.Replacer) error {
	a := owners[res.Chosen.Home]
	cmd, err := a.Launch(res.Chosen.Home.Target(), agentio.LaunchRequest{Prompt: "fix the failing tests"})
	if err != nil {
		return err
	}
	home, _ := agentio.EnvLookup(cmd.Env, claude.ConfigDirEnv)
	fmt.Fprintf(w, "launch: %s (%s=%s)\n", cmd, claude.ConfigDirEnv, tidy.Replace(home))
	return nil
}

// snippet:end launch

// snippet:start cached
// errNoCachedUsage marks a home whose vendor CLI left no usage on disk.
var errNoCachedUsage = errors.New("no cached usage on disk")

// cachedCandidates reads the usage each vendor CLI left on disk: no
// network and no quota, safe in a loop or a status bar. Each reading keeps
// its Source and ObservedAt, so Explain can say how old it is.
func cachedCandidates(agents []agentio.Agent) ([]pick.Candidate, error) {
	var cands []pick.Candidate
	for _, a := range agents {
		cu, ok := agentio.CachedUsageOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return nil, err
		}
		for _, home := range homes {
			u, found, err := cu.CachedUsage(home)
			if err == nil && !found {
				err = errNoCachedUsage
			}
			cands = append(cands, pick.Candidate{Home: home, Usage: u, Err: err})
		}
	}
	return cands, nil
}

// snippet:end cached

// snippet:start policy
// reserveLimit keeps a home out of the running once any window reaches it.
const reserveLimit = 50

// reserve is a custom pick.Policy: a home whose busiest window is at or
// over limit percent is refused (kept in reserve), the rest score like the
// default MaxPercent. Lower is better; ok=false skips with the reason.
type reserve struct{ limit float64 }

// Name labels the policy in Explain. Pure.
func (reserve) Name() string { return "reserve" }

// Score refuses a home at or over the limit. Pure.
func (r reserve) Score(c pick.Candidate) (float64, bool, string) {
	if m := c.Usage.MaxPercent(); m >= r.limit {
		return 0, false, fmt.Sprintf("a window is at %.0f%%, the reserve starts at %.0f%%", m, r.limit)
	}
	return c.Usage.MaxPercent(), true, ""
}

// snippet:end policy

// snippet:start explain
// printResult prints the one-line explanation, then every skipped home
// with its typed reason and cause.
func printResult(w io.Writer, res pick.Result) {
	fmt.Fprintln(w, res.Explain())
	for _, s := range res.Skipped {
		fmt.Fprintf(w, "  skipped %s:%s (%s): %v\n", s.Home.Agent(), s.Home.Name(), s.Reason, s.Err)
	}
}

// snippet:end explain
