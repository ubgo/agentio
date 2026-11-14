# Codex SQLite state (codex/state)

Codex keeps an authoritative index of its conversations and several side stores in SQLite files in the home: threads (`state_5.sqlite`), turns and items (`thread_history_1.sqlite`), goals, the message queue, memories and logs. The `codex/state` module reads them read-only and returns typed rows. Use it when rollout files are not enough: titles and previews as Codex shows them, token totals, archive and pin flags, sub-agent trees, Desktop projects, paginated turn history, goals and logs.

## Before you start

- Add the module: `go get github.com/ubgo/agentio/codex/state`. It is separate from `codex` because it needs a SQLite driver (`modernc.org/sqlite`, pure Go, so `CGO_ENABLED=0` builds work); callers of `codex` alone never download it.
- A `codex.Paths` for the home, preferably from `a.PathsWith(h, cfg)` so `sqlite_home` and `CODEX_SQLITE_HOME` are honoured ([paths](paths.md#step-1--get-home-paths)).
- The files must exist. A missing store fails its first query with an error matching `fs.ErrNotExist`.

The snippets come from [`examples/codexstate`](../../examples/codexstate/). No database is checked in: the example builds the six stores in a temp home from the upstream migrations kept in `codex/state/testdata/migrations` (the SQL Codex itself runs), inserts a few made-up rows, and reads them only through `state.DB`.

## Step 1 — Open the state database

`state.Open(paths)` prepares a `*state.DB` and reads nothing: each file is opened read-only on its first query, so listing threads never touches the logs store (tens of MB on a real machine). Only the stores you query need a path: a store whose path is empty fails with a `*state.NoPathError` (matching `state.ErrNoPath`) when you use it, never at `Open`, so `state.Open(codex.Paths{StateDB: p})` is enough to list threads. `DB.Path(f)` is the file behind a `state.File` (`FileState`, `FileThreadHistory`, `FileGoals`, `FileQueue`, `FileMemories`, `FileLogs`), `DB.SchemaVersion(ctx, f)` the newest applied migration, `f.MaxKnownMigration()` the newest one this library was written against, and `DB.Close` releases every connection. A store whose schema is newer fails with a `*state.SchemaTooNewError` (matching `state.ErrSchemaTooNew`) instead of guessing at renamed columns.

<!-- snippet: examples/codexstate/main.go#open -->
```go
// openState opens the stores of the default home. Open reads nothing:
// each file is opened read-only on its first query. PathsWith honours
// sqlite_home and CODEX_SQLITE_HOME, which move the files out of the home.
func openState(w io.Writer, a *codex.Agent) (*state.DB, error) {
	h := a.DefaultHome()
	cfg, err := a.ConfigFile(h)
	if err != nil {
		return nil, err
	}
	db := state.Open(a.PathsWith(h, cfg))
	ctx := context.Background()
	for _, f := range []state.File{state.FileState, state.FileThreadHistory, state.FileLogs} {
		v, err := db.SchemaVersion(ctx, f)
		var tooNew *state.SchemaTooNewError
		switch {
		case errors.As(err, &tooNew):
			// A newer Codex may have renamed columns: refuse, don't guess.
			fmt.Fprintf(w, "%s: schema %d is newer than %d\n", f, tooNew.Found, tooNew.Max)
		case err != nil:
			return nil, err
		default:
			fmt.Fprintf(w, "%s at %s schema=%d max-known=%d\n", f, filepath.Base(db.Path(f)), v, f.MaxKnownMigration())
		}
	}
	return db, nil
}
```

## Step 2 — List threads

`DB.Threads(ctx, ThreadStatusFilter)` lists threads, most recently updated first; the filter narrows by `Archived`, `Pinned` (both `*bool`, nil = either), exact `Dir`, `ProjectID` and `Limit` (0 = 200, capped at 10000). `DB.Thread(ctx, id)` returns one row or `state.ErrNotFound` (the root `agentio.ErrNotFound`). A `Thread` carries title, preview, first user message, created/updated/recency times, archive and pin flags, `Source`, model and reasoning effort, the decoded `Sandbox` policy and `Approval`, tokens used, CLI version, git info, sub-agent info, history mode, project id and per-column decode `Warnings`. `Thread.Conversation(h)` projects a row onto the same `codex.Conversation` the rollout lister returns. `DB.SubAgents(ctx, parent)` lists the `SpawnEdge`s a thread opened; `DB.Projects(ctx)` lists Desktop projects with their root folders.

<!-- snippet: examples/codexstate/main.go#threads -->
```go
// listThreads reads the thread index: richer than rollout files (title,
// preview, tokens, archive flag, git info). Thread.Conversation projects
// a row onto the same codex.Conversation the rollout lister returns.
func listThreads(ctx context.Context, w io.Writer, db *state.DB, h codex.Home) error {
	ts, err := db.Threads(ctx, state.ThreadStatusFilter{Limit: 50})
	if err != nil {
		return err
	}
	for _, t := range ts {
		fmt.Fprintf(w, "thread %s %q source=%s archived=%v pinned=%v tokens=%d history=%s sandbox=%s\n",
			t.ID, t.Title, t.Source, t.Archived, t.Pinned, t.TokensUsed, t.HistoryMode, t.Sandbox.Type)
	}
	notArchived := false
	active, err := db.Threads(ctx, state.ThreadStatusFilter{Archived: &notArchived, Dir: "/work/app"})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "active threads in /work/app: %d\n", len(active))

	a, err := db.Thread(ctx, threadA)
	if err != nil {
		return err
	}
	c := a.Conversation(h)
	fmt.Fprintf(w, "as conversation: %q dir=%s originator=%s branch=%s\n", c.Title, c.Dir, c.Originator, c.Git.Branch)
	if _, err := db.Thread(ctx, "019a0000-0000-7000-8000-0000000000ff"); errors.Is(err, state.ErrNotFound) {
		fmt.Fprintln(w, "unknown thread: ErrNotFound")
	}

	edges, err := db.SubAgents(ctx, threadA)
	if err != nil {
		return err
	}
	for _, e := range edges {
		child, err := db.Thread(ctx, e.ChildID)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "spawned %s status=%s nickname=%s role=%s\n", e.ChildID, e.Status, child.Agent.Nickname, child.Agent.Role)
	}

	projects, err := db.Projects(ctx)
	if err != nil {
		return err
	}
	for _, p := range projects {
		fmt.Fprintf(w, "project %s %q roots=%v\n", p.ID, p.Name, p.Roots)
	}
	return nil
}
```

## Step 3 — Read turns and items

`DB.Turns(ctx, thread)` lists a thread's turns in rollout order (`Status`, `Error` as raw JSON, start/completion times, `Duration`, the first user and final agent item ids, and the byte range the turn occupies in the rollout). `DB.Items(ctx, thread, after, limit)` pages through items with `rollout_ordinal > after` (0 = 200 per page, capped at 10000): a typed header (`Type`, ordinal, timestamps) plus the vendor `JSON`, which stays raw because its schema moves fast.

<!-- snippet: examples/codexstate/main.go#turns -->
```go
// readTurnsItems reads a paginated thread's turns and pages through its
// items by rollout ordinal. A legacy thread (HistoryMode legacy) has no
// rows here: its history lives only in the rollout file.
func readTurnsItems(ctx context.Context, w io.Writer, db *state.DB) error {
	turns, err := db.Turns(ctx, threadA)
	if err != nil {
		return err
	}
	for _, t := range turns {
		fmt.Fprintf(w, "turn %s status=%s duration=%s", t.TurnID, t.Status, t.Duration)
		if len(t.Error) > 0 { // nil when error_json is NULL
			fmt.Fprintf(w, " error=%s", t.Error)
		}
		fmt.Fprintln(w)
	}
	const pageSize = 2
	var after int64
	for page := 1; ; page++ {
		items, err := db.Items(ctx, threadA, after, pageSize)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			fmt.Fprintf(w, "page %d item %s type=%s known=%v ordinal=%d\n", page, it.ItemID, it.Type, it.Type.IsKnown(), it.RolloutOrdinal)
		}
		after = items[len(items)-1].RolloutOrdinal
	}
	legacy, err := db.Turns(ctx, threadB)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "legacy thread turns: %d\n", len(legacy))
	return nil
}
```

## Step 4 — Read goals, queue, memories and logs

`DB.Goal(ctx, thread)` returns a thread's `/goal` objective with status, token budget (nil = none), tokens and time used; `found=false` when there is none. `DB.Queue(ctx, thread)` lists messages queued while the thread was busy, in send order. `DB.Memories(ctx, limit)` lists stage-1 memories extracted from rollouts; `DB.MemoryJobs(ctx)` the background extraction and consolidation jobs (the worker ownership token is never read). `DB.Logs(ctx, LogQuery)` returns log lines newest first, filtered by `ThreadID`, exact `Level`, `Since`, and bounded by `Limit` (0 = 200).

<!-- snippet: examples/codexstate/main.go#small -->
```go
// readSmallStores reads the per-thread goal and message queue, the
// memories and memory jobs, and the newest warning log lines. Each store
// is its own file, opened only when first queried.
func readSmallStores(ctx context.Context, w io.Writer, db *state.DB) error {
	g, found, err := db.Goal(ctx, threadA)
	if err != nil {
		return err
	}
	if found {
		fmt.Fprintf(w, "goal %q status=%s used=%d budget=%d time=%s\n", g.Objective, g.Status, g.TokensUsed, *g.TokenBudget, g.TimeUsed)
	}

	queue, err := db.Queue(ctx, threadA)
	if err != nil {
		return err
	}
	for _, q := range queue {
		fmt.Fprintf(w, "queued %s order=%d payload=%s\n", q.ID, q.Order, q.Payload)
	}

	mems, err := db.Memories(ctx, 10)
	if err != nil {
		return err
	}
	for _, m := range mems {
		fmt.Fprintf(w, "memory thread=%s summary=%q phase2=%v\n", m.ThreadID, m.RolloutSummary, m.SelectedPhase2)
	}
	jobs, err := db.MemoryJobs(ctx)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		fmt.Fprintf(w, "memory job %s status=%s retries-left=%d\n", j.Kind, j.Status, j.RetryRemaining)
	}

	logs, err := db.Logs(ctx, state.LogQuery{ThreadID: threadA, Level: state.LevelWarn, Since: time.Unix(1793000000, 0), Limit: 20})
	if err != nil {
		return err
	}
	for _, l := range logs {
		fmt.Fprintf(w, "log %s %s %s %q\n", l.Time.UTC().Format(time.RFC3339), l.Level, l.Target, l.Body)
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./codexstate
```

```text
state_5.sqlite at state_5.sqlite schema=59 max-known=59
thread_history_1.sqlite at thread_history_1.sqlite schema=8 max-known=8
logs_2.sqlite at logs_2.sqlite schema=2 max-known=2
thread 019a0000-0000-7000-8000-00000000000b "Sub task" source=subagent archived=true pinned=false tokens=10 history=legacy sandbox=danger-full-access
thread 019a0000-0000-7000-8000-00000000000a "Fix the build" source=vscode archived=false pinned=true tokens=1234 history=paginated sandbox=workspace-write
active threads in /work/app: 1
as conversation: "Fix the build" dir=/work/app originator=Codex Desktop branch=main
unknown thread: ErrNotFound
spawned 019a0000-0000-7000-8000-00000000000b status=closed nickname=helper role=worker
project p1 "App" roots=[/work/app /work/app-docs]
turn t1 status=completed duration=1m0s
turn t2 status=failed duration=10s error={"message":"boom"}
page 1 item i1 type=userMessage known=true ordinal=4
page 1 item i2 type=agentMessage known=true ordinal=5
page 2 item i3 type=userMessage known=true ordinal=10
legacy thread turns: 0
goal "ship it" status=budget_limited used=1200 budget=1000 time=1m30s
queued q1 order=1 payload={"text":"first"}
queued q2 order=2 payload={"text":"second"}
memory thread=019a0000-0000-7000-8000-00000000000a summary="fixed the build" phase2=true
memory job stage1 status=done retries-left=3
log 2026-10-26T07:33:30Z WARN codex_core "slow tool call"
```

## Pitfalls

- **Read-only, always.** Every connection is opened read-only with `query_only`; nothing here writes. Archive, delete and queue changes go through Codex's own commands ([launch](launch.md#step-3--build-archive-delete-and-queue-commands)), because Codex keeps rollouts, threads and thread history consistent itself.
- **WAL is honoured, `immutable` is never used.** Codex writes these files live in WAL mode; an immutable open would skip the WAL and miss the newest rows. Expect to see rows Codex wrote a moment ago.
- **`Open` cannot fail; a missing path fails on use.** `Open` returns only a `*DB`. A store you gave no path for is `*state.NoPathError` (`errors.Is(err, state.ErrNoPath)`) on its first query; a path to a file that does not exist wraps `fs.ErrNotExist`.
- **A newer Codex can stop the reader.** When Codex upgrades a store past `MaxKnownMigration` (0059 for state, 0008 for thread history, 0002 for the others at Codex 0.162.1), queries on that store return `*SchemaTooNewError` until this library is updated. Other stores keep working.
- **Older schemas still read.** Columns added by later migrations come back as zero, nil or their default (for example `HistoryMode` legacy, `Model` nil); `Projects` and `SubAgents` return nil on a schema that has no such table.
- **Legacy threads have no turns here.** Only threads with `HistoryMode` paginated have rows in `thread_history_1.sqlite`; for legacy threads read the rollout ([turns](conversations.md#step-2--read-turns)). `Turn.Error` is nil when the turn has no error; check `len(t.Error) > 0`. JSON encoding leaves a nil `Error` out (`omitempty`) instead of writing `null`.
- **Pass `PathsWith`, not `Paths`.** With `sqlite_home` or `CODEX_SQLITE_HOME` set, `Paths` points at the wrong directory and every query fails with a not-exist error.
- **`RolloutPath` is absolute.** It is why Codex conversations cannot move between homes; do not rewrite it.
- **Per-column degradation.** A column value the library cannot decode (an unseen `sandbox_policy` shape, a garbage `cli_version`) adds an entry to `Thread.Warnings`; the rest of the row still reads.
- **Personal ids redact.** `CreatorUserID` and `CreatorAccountID` are `agentio.Personal`.

## Related

- [Codex conversations and history](conversations.md), [Codex rollout files](rollout.md), [Codex paths](paths.md)
- [Install agentio](../getting-started/install.md#step-1--add-the-module-you-need) (which module pulls which dependency)
- [Conversations (neutral)](../capabilities/conversations.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
