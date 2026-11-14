// Command runevents decodes the event stream of a headless run with the
// neutral RunEvents decoder: a recorded `claude -p --output-format
// stream-json` run and a `codex exec --json` sample. It also prints the
// headless launch command whose stdout such a decoder reads. Nothing is
// run: the streams come from files and constants.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

// Recorded stream of a real Claude Code 2.1.325 headless run, in the repo.
const claudeStream = "claude/testdata/stream/2.1.325/stream-partial.jsonl"

// codexStream is a `codex exec --json` run in the shape Codex 0.162.1
// prints, including a non-JSON warning line and an event type this
// library does not know yet.
const codexStream = `{"type":"thread.started","thread_id":"0199e1a0-0000-7000-8000-000000000001"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"I will list the files."}}
{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"ls","aggregated_output":"a.txt","exit_code":0,"status":"completed"}}
warning: not json
{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":1000,"output_tokens":80,"reasoning_output_tokens":16}}
{"type":"future.event","x":1}
`

// Fixed values for the printed launch.
const (
	fixtureUserHome = "/home/user"
	examplePrompt   = "list the files"
	maxText         = 40
)

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "runevents:", err)
		os.Exit(1)
	}
}

// runMain opens the recorded Claude stream and runs. Disk · Read.
func runMain(w io.Writer) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(root, claudeStream))
	if err != nil {
		return err
	}
	// Read-only file; a close error changes nothing.
	defer func() { _ = f.Close() }()
	return run(w, f, strings.NewReader(codexStream))
}

// run decodes both streams and prints the headless launch of each agent.
func run(w io.Writer, claudeOut, codexOut io.Reader) error {
	h := agentio.Host{UserHome: fixtureUserHome, Env: []string{"HOME=" + fixtureUserHome}}
	streams := []struct {
		agent agentio.Agent
		out   io.Reader
	}{
		{claude.New(h).Neutral(), claudeOut},
		{codex.New(h).Neutral(), codexOut},
	}
	for _, s := range streams {
		fmt.Fprintf(w, "== %s ==\n", s.agent.ID())
		cmd, err := headlessCommand(s.agent)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "launch: %s\n", cmd)
		if err := decode(w, s.agent, s.out); err != nil {
			return err
		}
	}
	return nil
}

// snippet:start launch
// headlessCommand builds the non-interactive launch whose stdout is the
// event stream: Headless picks the vendor's machine-readable output.
func headlessCommand(a agentio.Agent) (agentio.Command, error) {
	return a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{Prompt: examplePrompt, Headless: true})
}

// snippet:end launch

// snippet:start decode
// decode reads every event until io.EOF. Unknown vendor events arrive as
// RunEventOther with the full line in Raw, so a new vendor event never
// stops the loop; Done carries the run's token counts. Partial-message
// events carry no text, so empty messages are counted but not printed.
func decode(w io.Writer, a agentio.Agent, out io.Reader) error {
	re, ok := agentio.RunEventsOf(a)
	if !ok {
		return fmt.Errorf("%s has no run events decoder", a.ID())
	}
	d := re.NewRunDecoder(out)
	counts := map[agentio.RunEventType]int{}
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		counts[ev.Type]++
		switch ev.Type {
		case agentio.RunEventStarted:
			fmt.Fprintf(w, "started conversation %s\n", ev.ConversationID)
		case agentio.RunEventMessage:
			if ev.Text != "" {
				fmt.Fprintf(w, "message %q\n", truncate(ev.Text))
			}
		case agentio.RunEventDone:
			if ev.Tokens != nil {
				fmt.Fprintf(w, "done tokens in=%d cached=%d out=%d reasoning=%d\n", ev.Tokens.Input, ev.Tokens.CachedInput, ev.Tokens.Output, ev.Tokens.Reasoning)
			}
		case agentio.RunEventError:
			fmt.Fprintf(w, "error %q\n", ev.Text)
		}
	}
	for _, t := range agentio.RunEventTypeValues() {
		if counts[t] > 0 {
			fmt.Fprintf(w, "  %-7s x%d\n", t, counts[t])
		}
	}
	return nil
}

// snippet:end decode

// truncate shortens s to maxText runes for display. Pure.
func truncate(s string) string {
	r := []rune(s)
	if len(r) <= maxText {
		return s
	}
	return string(r[:maxText]) + "..."
}

// repoRoot walks up from the working directory to the folder holding the
// recorded stream. Disk · Read.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, claudeStream)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("example: %s not found above the working directory", claudeStream)
		}
		dir = parent
	}
}
