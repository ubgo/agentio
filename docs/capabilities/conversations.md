# Conversations, turns and prompt history

Every coding agent keeps resumable conversations (transcripts) and a history of the prompts the user typed. agentio exposes them through five capabilities: `ConversationLister` lists and looks up conversations, `TurnReader` reads a conversation's turns incrementally from a byte offset, `PromptHistoryReader` reads typed prompts, `HistoryChecker` answers "was this agent ever used in this folder?", and two write doors move history: `HistoryMover` (after a folder rename) and `ConversationMover` (between two homes of one agent). Use them for a resume picker, a transcript viewer that follows a live session, or a tool that moves work from a personal home to a work home.

## Before you start

- Modules: `github.com/ubgo/agentio` plus the agents.
- The homes must exist on disk. Reads never change anything; the two movers write, and each returns the `agentio.Backup` it made first.
- `ConversationMover.PlanMove` checks for a running session in the folder, so give the Host a process table (`Host.Processes`); an empty one means nothing is running.

[`examples/conversations`](../../examples/conversations/) copies the Claude and Codex fixture homes into a temp dir, so the move steps change only the copy.

## Step 1 — List conversations

`Conversations(ctx, home, query)` returns conversations newest first. `ConversationQuery` narrows the list: `Dir` (only conversations started in that exact folder), `Since` (updated at or after) and `Limit` (newest N; 0 = all). `Conversation(home, id)` looks one up and returns `ErrNotFound` when it is absent.

<!-- snippet: examples/conversations/main.go#list -->
```go
// listConversations lists the newest conversations of every home, newest
// first, then looks one up by ID. Disk · Read; transcripts are never read
// whole (head for the folder, tail for the title).
func listConversations(ctx context.Context, w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		cl, ok := agentio.ConversationsOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			convs, err := cl.Conversations(ctx, home, agentio.ConversationQuery{Limit: listLimit})
			if err != nil {
				return err
			}
			for _, c := range convs {
				fmt.Fprintf(w, "%s %s %s %s %q\n", a.ID(), home.Name(), c.ID, c.Dir, c.Title)
			}
			if len(convs) == 0 {
				continue
			}
			one, err := cl.Conversation(home, convs[0].ID)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "  by id: %s\n", tidy(h, one.Path))
		}
	}
	return nil
}
```

An `agentio.Conversation` has `Agent`, `Home`, `ID` (what you pass to `LaunchRequest.ResumeID`), `Title`, `Dir`, `Path` (the transcript file), `Created` and `Updated`.

## Step 2 — Read turns from an offset

`Turns(home, id, from, fn)` reads the transcript from byte offset `from` to the last complete line, calls `fn` once per normalized turn and returns the new `Offset`. Keep the offset and pass it next time: a viewer that follows a running session re-reads nothing, and a half-written last line is left for the next call.

<!-- snippet: examples/conversations/main.go#turns -->
```go
// readTurns reads a conversation's turns from offset 0, keeps the returned
// offset, and reads again from it: the second read sees only lines written
// since, so a watcher re-reads nothing. The offset never moves past an
// incomplete trailing line.
func readTurns(ctx context.Context, w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		tr, ok := agentio.TurnsOf(a)
		if !ok {
			continue
		}
		conv, found, err := newestConversation(ctx, a)
		if err != nil || !found {
			return err
		}
		var tokens agentio.TokenCounts
		count := 0
		off, err := tr.Turns(a.DefaultHome(), conv.ID, 0, func(t agentio.Turn) error {
			count++
			if t.Tokens != nil {
				tokens = tokens.Add(*t.Tokens)
			}
			if t.Text != "" {
				fmt.Fprintf(w, "%s %-9s model=%q text=%q\n", a.ID(), t.Role, t.Model, t.Text)
			}
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: %d turns, offset %d, tokens in=%d cached=%d out=%d\n", a.ID(), count, off, tokens.Input, tokens.CachedInput, tokens.Output)
		again := 0
		if _, err := tr.Turns(a.DefaultHome(), conv.ID, off, func(agentio.Turn) error { again++; return nil }); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: %d new turns after offset %d\n", a.ID(), again, off)
	}
	return nil
}
```

An `agentio.Turn` has `Role` (`user` or `assistant`), `Time`, `ConversationID`, `Dir`, `Model`, `StopReason`, `AgentVersion`, `Text`, `Tokens` (a `*TokenCounts`, nil when the turn carries none) and `Raw` (the vendor line). `TokenCounts` has `Input`, `CachedInput`, `CacheWrite`, `Output` and `Reasoning`; `Add` sums two tallies. Returning an error from `fn` stops the read and returns that error.

## Step 3 — Read prompt history

`PromptHistory(home, query)` reads the prompts the user typed, newest first, scanning the history file from its tail. `PromptQuery` has `Dir`, `ConversationID`, `Since` and `Limit`.

<!-- snippet: examples/conversations/main.go#prompts -->
```go
// readPrompts reads the newest typed prompts, scanning history from its
// tail; Dir narrows to one folder for agents that record it.
func readPrompts(w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		pr, ok := agentio.PromptHistoryOf(a)
		if !ok {
			continue
		}
		entries, err := pr.PromptHistory(a.DefaultHome(), agentio.PromptQuery{Limit: promptLimit})
		if err != nil {
			return err
		}
		for _, e := range entries {
			fmt.Fprintf(w, "%s %s dir=%q %q\n", a.ID(), e.Time.UTC().Format("2006-01-02 15:04"), e.Dir, e.Text)
		}
	}
	return nil
}
```

Each `PromptEntry` has `Time`, `Text`, `Dir` and `ConversationID`. Not every vendor records the folder: Codex's prompt history has none, so `Dir` is empty and a `Dir` filter matches nothing there.

## Step 4 — Check and move history

`HasHistory(home, dir)` is the cheap question "any conversation started in this folder?"; it never reads a transcript. The two movers are write doors with cost `disk write`:

- `HistoryMover.MoveHistory(home, from, to)` re-keys a folder's history after the folder itself was renamed on disk, so the agent finds the old conversations from the new path.
- `ConversationMover.PlanMove(req)` plans moving one folder's conversations from one home to another home of the same agent (a pure inspection that returns a `MovePlan` with `Conversations`, `Items`, `Blockers`, `Left` and `Cleanup`); `ApplyMove(plan)` executes it, backing every item up first and rolling back if a rename fails.

<!-- snippet: examples/conversations/main.go#move -->
```go
// checkAndMove asks whether a folder has history, re-keys Claude's history
// after the folder was renamed, and plans a move of one folder's
// conversations to another home. Moves are writes: each returns its backup.
// Codex declares why it cannot move.
func checkAndMove(w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		if hc, ok := agentio.HistoryCheckerOf(a); ok {
			for _, dir := range []string{recordedDir, codexDir, emptyDir} {
				has, err := hc.HasHistory(a.DefaultHome(), dir)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "%s has history in %s: %t\n", a.ID(), dir, has)
			}
		}
		hm, ok := agentio.HistoryMoverOf(a)
		if !ok {
			reason, _ := agentio.Unsupported(a, agentio.CapHistoryMover)
			fmt.Fprintf(w, "%s cannot move history: %.60s...\n", a.ID(), reason)
			continue
		}
		b, err := hm.MoveHistory(a.DefaultHome(), recordedDir, renamedDir)
		if err != nil {
			return err
		}
		for _, e := range b.Entries {
			fmt.Fprintf(w, "%s moved %s -> %s, backup %s\n", a.ID(), recordedDir, renamedDir, tidy(h, e.Copy))
		}
		if err := planMove(w, h, a); err != nil {
			return err
		}
	}
	return nil
}

// planMove plans moving the renamed folder's conversations from the
// default home to the agent's second home and prints the plan; applying it
// is one more call (ApplyMove) on the same door.
func planMove(w io.Writer, h agentio.Host, a agentio.Agent) error {
	cm, ok := agentio.ConversationMoverOf(a)
	if !ok {
		return nil
	}
	homes, err := a.Homes()
	if err != nil || len(homes) < 2 {
		return err
	}
	plan, err := cm.PlanMove(agentio.MoveRequest{From: homes[0], To: homes[1], Dir: renamedDir})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s plan %s -> %s: %d conversations, %d items, blockers %v\n", a.ID(), plan.From.Name(), plan.To.Name(), len(plan.Conversations), len(plan.Items), plan.Blockers)
	b, err := cm.ApplyMove(plan)
	if err != nil {
		return err
	}
	for _, e := range b.Entries {
		fmt.Fprintf(w, "%s applied, backup %s\n", a.ID(), tidy(h, e.Copy))
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./conversations
```

Output:

```text
== conversations ==
claude personal 3f474440-903e-5410-8bfb-d537b89f9547 /work/proj13 "Fixture title 166"
claude personal f85efbdb-fc2f-1e8e-7748-9f915edc8e2d /work/proj12 "Fixture title 185"
  by id: ~/.claude/projects/-work-proj13/3f474440-903e-5410-8bfb-d537b89f9547.jsonl
claude work 0b5e3a8e-0000-4000-8000-000000000099 /work/proj99 "Work home fixture"
  by id: ~/.claude-work/projects/-work-proj99/0b5e3a8e-0000-4000-8000-000000000099.jsonl
codex personal 0199e1a0-0000-7000-8000-000000000001 /work/proj "List the files"
codex personal 0199e1a0-0000-7000-8000-000000000002 /work/other "hello"
  by id: ~/.codex/sessions/2026/10/14/rollout-2026-10-14T10-00-00-0199e1a0-0000-7000-8000-000000000001.jsonl
== turns ==
claude user      model="" text="fixture text 140"
claude assistant model="claude-opus-5-5" text="fixture text 160"
claude assistant model="claude-opus-5-5" text="fixture text 162"
claude assistant model="claude-opus-5-5" text="fixture text 165"
claude: 12 turns, offset 46441, tokens in=24 cached=1186928 out=4198
claude: 0 new turns after offset 46441
codex user      model="gpt-5.5" text="list the files"
codex assistant model="gpt-5.5" text="There is one file: a.txt"
codex: 2 turns, offset 4236, tokens in=1200 cached=1000 out=80
codex: 0 new turns after offset 4236
== prompt history ==
claude 2026-11-13 19:35 dir="/work/proj13" "fixture text 139"
claude 2026-11-13 19:10 dir="/work/proj4" "fixture text 138"
claude 2026-11-13 19:08 dir="/work/proj4" "fixture text 137"
codex 2026-10-15 10:01 dir="" "and the sizes"
codex 2026-10-15 10:00 dir="" "list the files"
codex 2026-03-01 08:00 dir="" "hello"
== check and move ==
claude has history in /work/proj13: true
claude has history in /work/proj: false
claude has history in /work/never-used: false
claude moved /work/proj13 -> /work/proj13-renamed, backup ~/.claude/projects/-work-proj13.agentio-bak-20261113T120000Z
claude plan personal -> work: 1 conversations, 1 items, blockers []
claude applied, backup ~/.claude.agentio-bak-20261113T120000Z/projects/-work-proj13-renamed/3f474440-903e-5410-8bfb-d537b89f9547.jsonl
codex has history in /work/proj13: false
codex has history in /work/proj: true
codex has history in /work/never-used: false
codex cannot move history: Codex indexes conversations in its own SQLite database with ...
```

## Pitfalls

- **`Updated` is the transcript file's modification time.** Ordering (newest first) and `ConversationQuery.Since` use it too, so copying a home (backup restore, `cp -r`) resets every conversation's `Updated`. `Created` comes from the transcript itself.
- **Transcripts are never read whole.** Listing reads the head of each file (for the folder) and the last 256 KiB (for the title). A title written earlier than that window is not seen; read turns when you need the content.
- **A `Dir` query is an exact folder.** Claude looks the folder up by its encoded name (every byte that is not a letter or digit becomes `-`), the way Claude Code does, and reports `Dir` as the folder you asked for, because the cwd inside old transcripts is stale after a history move.
- **`MoveHistory` with no history is a silent no-op.** The neutral method returns an empty `Backup` and a nil error when `from` has no history; check `Backup.IsEmpty()`. An existing destination is an error (Claude: `claude.ErrHistoryExists`); two histories are never merged.
- **Codex history cannot move.** Codex indexes conversations in its SQLite database with absolute paths, so it has neither `HistoryMover` nor `ConversationMover`; `agentio.Unsupported` gives the reason. Continue Codex work in the original home.
- **`PlanMove` blockers stop `ApplyMove`.** A running session in the folder (in either home) or a conversation already present in the target makes `ApplyMove` return `ErrMoveBlocked` before touching a file. Moves rename, so both homes must be on one filesystem; a cross-device move fails and rolls back.
- **Offsets are bytes into one transcript.** Do not reuse an offset across conversations or after the vendor rewrote the file.

## Related

- [Live sessions and processes](live-sessions.md): `IsLive` before you move or resume.
- [Backup, move and link](../core/backup-move-link.md): `Backup`, `MovePlan`, `ApplyMovePlan`.
- [Doors: Runner, Prober and Writer](doors.md#step-3--write-through-a-writer): the vendor Writer that the movers use.
- Vendor pages: [Claude conversations](../claude/conversations.md), [Claude transcript](../claude/transcript.md), [Claude history](../claude/history.md), [Codex conversations](../codex/conversations.md), [Codex rollouts](../codex/rollout.md).
- [Choosing guides](../choosing/README.md). Example: [`examples/conversations`](../../examples/conversations/). Feature index: [features](../reference/features.md).
