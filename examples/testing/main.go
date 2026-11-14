// Command testing is a small "your own code" program written so it can be
// tested: every machine input arrives through an agentio.Host parameter,
// never through HostFromOS inside the logic. main builds a Host over the
// fixture homes checked into this repository; main_test.go builds Hosts
// with agenttest (fixture homes, fake secrets, fake processes, fake exec,
// an httptest server) and runs the conformance suite.
//
// Cost: Disk · Read of the fixture homes. No vendor binary is run, the
// network is never used and no secret store is read.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
	"github.com/ubgo/agentio/codex"
)

// Fixture locations relative to the repository root. The Claude fixture is
// a whole user home (~/.claude, ~/.claude-work, ~/.claude.json, a project);
// the Codex fixture is one Codex home, selected with CODEX_HOME.
const (
	claudeFixture = "claude/testdata/homes/2.1.325/userhome"
	codexFixture  = "codex/testdata/homes/0.162.1"
	// repoFromExamples is the repository root seen from `cd examples`.
	repoFromExamples = ".."
	// noBinaries is a PATH that holds no vendor binary, so nothing real
	// can be found or run by accident.
	noBinaries  = "PATH=/nonexistent-bin"
	fixtureUser = "user"
)

// fixtureClock is the fixed time main reads the fixtures at, so the output
// does not change from day to day.
func fixtureClock() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

func main() {
	h, err := repoFixtureHost(repoFromExamples)
	if err == nil {
		err = run(context.Background(), os.Stdout, h)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "testing:", err)
		os.Exit(1)
	}
}

// snippet:start repohost
// repoFixtureHost builds a Host over the fixture homes under repoRoot
// without the testing package: a program (not a test) can still run on
// fixtures. Secrets, Processes, HTTP and Exec stay nil; the readers this
// program calls never reach them.
func repoFixtureHost(repoRoot string) (agentio.Host, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return agentio.Host{}, err
	}
	home := filepath.Join(root, claudeFixture)
	return agentio.Host{
		UserHome: home,
		User:     fixtureUser,
		Env: []string{
			"HOME=" + home,
			noBinaries,
			codex.EnvHome + "=" + filepath.Join(root, codexFixture),
		},
		Now: fixtureClock,
	}, nil
}

// snippet:end repohost

// snippet:start yourcode
// AgentReport is one row of this program's report.
type AgentReport struct {
	ID            agentio.ID
	Homes         []string
	Conversations int
	Capabilities  int
}

// collect is the code under test: it loops over every built-in agent on h,
// plus one config-defined agent per custom Spec, and counts homes,
// conversations and supported capabilities. It takes the Host as a
// parameter, so a test hands it a fixture Host and a program hands it
// HostFromOS(). An agent without a conversation reader counts zero. Disk ·
// Read.
func collect(ctx context.Context, h agentio.Host, custom ...agentio.Spec) ([]AgentReport, error) {
	agents, err := builtin.New(h, custom...)
	if err != nil {
		return nil, err
	}
	var out []AgentReport
	for _, a := range agents.All() {
		homes, err := a.Homes()
		if err != nil {
			return nil, fmt.Errorf("%s homes: %w", a.ID(), err)
		}
		r := AgentReport{ID: a.ID(), Capabilities: len(agentio.Supported(a))}
		lister, canList := agentio.ConversationsOf(a)
		for _, home := range homes {
			r.Homes = append(r.Homes, home.Name())
			if !canList {
				continue
			}
			convs, err := lister.Conversations(ctx, home, agentio.ConversationQuery{})
			if err != nil {
				return nil, fmt.Errorf("%s conversations in %s: %w", a.ID(), home.Name(), err)
			}
			r.Conversations += len(convs)
		}
		out = append(out, r)
	}
	return out, nil
}

// run prints the report for h.
func run(ctx context.Context, w io.Writer, h agentio.Host) error {
	rows, err := collect(ctx, h)
	if err != nil {
		return err
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%s homes=%v conversations=%d capabilities=%d\n", r.ID, r.Homes, r.Conversations, r.Capabilities)
	}
	return nil
}

// snippet:end yourcode
