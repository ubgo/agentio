# The opencode agent

`opencode.New` gives you the opencode CLI (npm `opencode-ai`, the native installer and Homebrew) on one `agentio.Host`: its install, the data dir that serves as its home, its sessions and their turns, process classification, the commands that launch or resume it, and the doctor. Use the `opencode` module when your code knows it talks to opencode; use `Neutral()` when the same code loops over every agent.

opencode differs from the other built-ins in three ways that shape this API:
- **Two session stores.** Since 1.2.0 every session lives in a SQLite database, `opencode.db` in the data dir. Before that, sessions were a JSON tree (`storage/session`, `storage/message`, `storage/part`), which 1.2.0 migrated into the database and left on disk. The `opencode` module is stdlib only and reads the legacy tree; the separate `opencode/store` module carries the pure-Go SQLite driver and reads the database. `Neutral()` conversations and turns are the legacy ones; read current sessions with `opencode/store`.
- **No home variable of its own.** The data dir is `$XDG_DATA_HOME/opencode`, else `~/.local/share/opencode`, on every OS. `XDG_DATA_HOME` is a base dir every XDG tool shares, so `Spec().HomeEnv` is empty; `DefaultHome` resolves the variable from the Host env, and a launch on another data dir sets `XDG_DATA_HOME` itself.
- **No plan, one login per provider.** opencode bills through each model provider's key, kept in `auth.json`. There is no account identity or plan usage to report, and this module never reads that file.

## Before you start

- Add the module: `go get github.com/ubgo/agentio/opencode` (stdlib only, plus the root module). For the database add `go get github.com/ubgo/agentio/opencode/store` (adds `modernc.org/sqlite`, pure Go, so `CGO_ENABLED=0` builds work).
- Build a `Host` (`agentio.HostFromOS()` in real programs; see [Host](../core/host.md)).
- A data dir counts as a home when it holds `opencode.db` or `storage/`. A `storage/` folder is a common name, so outside the data dir the Host env selects only `opencode.db` makes a folder an opencode home.

The snippets come from [`examples/opencode`](../../examples/opencode/), which runs on a temp copy of the 1.18.30 fixture user home.

## Step 1 — Install and homes

`Install` finds `opencode` on `PATH` and reads the version from the `opencode-ai` `package.json` above the resolved binary (or the Homebrew Cellar path). The native installer (`~/.opencode/bin/opencode`) leaves no version on disk; `Runner().BinaryVersion` runs `opencode --version` when you need it. The tested range is `opencode.TestedMin`..`opencode.TestedMax` (1.18.30), and `features.tsv` records the bounds a release note names (the database since 1.2.0, `--fork` since 1.1.54).

| Call | Returns | Cost |
|---|---|---|
| `a.DataDir()` | `$XDG_DATA_HOME/opencode` (an absolute value only), else `~/.local/share/opencode` | Pure |
| `a.DefaultHome()` | that data dir, marked default | Pure |
| `a.HomeFromEnv()` | the default home when `XDG_DATA_HOME` selects it; `false` when the variable is absent | Pure |
| `a.HomeAt(dir)` | the data dir at any path | Pure |
| `a.Homes()` | the default data dir when it is a home | Disk read |
| `a.ConfigDir()` | `$XDG_CONFIG_HOME/opencode`, else `~/.config/opencode` (config is not modelled yet) | Pure |
| `a.DatabasePath(h)` | `OPENCODE_DB` (absolute, or relative to the data dir), else `<data dir>/opencode.db`; `false` for `:memory:` | Pure |
| `opencode.ChannelDatabasePath(h, ch)` | `opencode-<channel>.db`, the file a dev or local build uses | Pure |

<!-- snippet: examples/opencode/main.go#homes -->
```go
// homes shows the data dir (the home: XDG_DATA_HOME/opencode, else
// ~/.local/share/opencode), the config dir and the database file.
func homes(p printer, a *opencode.Agent) error {
	p.line("== homes ==")
	hs, err := a.Homes()
	if err != nil {
		return err
	}
	for _, h := range hs {
		db, _ := a.DatabasePath(h)
		p.line(fmt.Sprintf("home: %s default=%t dir=%s", h.Name(), h.IsDefault(), h.Dir()))
		p.line(fmt.Sprintf("  database=%s config=%s", db, a.ConfigDir()))
	}
	return nil
}
```

## Step 2 — Legacy sessions and turns

`Sessions` reads the small session files (`storage/session/<projectID>/<id>.json`: id, version, directory, title, times, share link), newest first, filtered by directory, time and limit. `Turns` reads a session's messages and their parts in file-name order (opencode ids are ascending, so that is creation order):
- one turn per user or assistant message, `Turn.ID` the message id;
- `Text` joins the message's text parts and skips synthetic text opencode inserted itself (plan-mode reminders, compaction prompts);
- `Model` is `providerID/modelID`, `StopReason` the finish reason, `Tokens` the message's tokens with cache read as `CachedInput`, cache write as `CacheWrite` and reasoning separate;
- `Interim` is set on an assistant message whose step ended in tool calls (finish `tool-calls`) and on a compaction summary, so a caller that stores answers keeps only the final message of a prompt.

`TurnsAt` reads the same turns from a path you already hold (the session file, the session's message dir or one message file) and `TranscriptOwner` maps such a path to its home and session.

<!-- snippet: examples/opencode/main.go#legacy -->
```go
// legacy lists the sessions of the storage/ tree that opencode 1.2.0
// migrated into opencode.db, then reads one session's turns twice: the
// second read resumes from the cursor and finds nothing new.
func legacy(ctx context.Context, p printer, a *opencode.Agent) error {
	p.line("== legacy storage ==")
	h := a.DefaultHome()
	ss, err := a.Sessions(ctx, h, agentio.ConversationQuery{})
	if err != nil {
		return err
	}
	for _, s := range ss {
		p.line(fmt.Sprintf("session %s %q dir=%s v%s", s.ID, s.Title, s.Directory, s.Version))
	}
	show := func(t agentio.Turn) error {
		tokens := ""
		if t.Tokens != nil {
			tokens = fmt.Sprintf(" in=%d cached=%d out=%d reasoning=%d", t.Tokens.Input, t.Tokens.CachedInput, t.Tokens.Output, t.Tokens.Reasoning)
		}
		p.line(fmt.Sprintf("  %s %s %q%s", t.ID, t.Role, t.Text, tokens))
		return nil
	}
	cursor, err := a.Turns(h, legacySession, 0, show)
	if err != nil {
		return err
	}
	again, err := a.Turns(h, legacySession, cursor, show)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("  cursor=%d, re-read from it: cursor=%d", cursor, again))
	return nil
}
```

### The cursor

Both stores use the same `agentio.Offset` rule: the cursor is the number of the session's messages already passed, in the store's stable order (file names for the legacy tree, `time_created` then `id` in the database). Store the value a read returns and pass it back; never compute one.
- Reading stops before the newest message while opencode is still writing it: an assistant message without `time.completed` or an error, or a user message without parts. Its text and usage are reported once, when it is complete.
- A message followed by another is complete even if opencode never finished it (a provider that never answered), so one abandoned message never stalls the cursor.
- A revert deletes messages. A cursor past the end then emits nothing and returns the current count; a caller that must not miss a message dedups on `Turn.ID` and re-reads from 0 now and then.

## Step 3 — The database (opencode/store)

`store.OpenHome(a, h)` opens the file `DatabasePath` names; `store.Open(h, path)` opens any path. Nothing is read until a query runs. The connection is read-only (`mode=ro`, `query_only`, a 2 second busy timeout) and never `immutable`, so rows opencode has written to the WAL are seen. The schema is read by column presence: columns a newer release adds are ignored, and the session totals that older databases lack read as zero.

| Call | Returns | Cost |
|---|---|---|
| `db.Sessions(ctx, store.SessionQuery{...})` | session rows: title, directory, version, slug, agent, model, cost and token totals, share link, archived time, parent | Disk read |
| `db.Session(ctx, id)` | one row; `agentio.ErrNotFound` when absent | Disk read |
| `db.Conversations(ctx, q)` | the resumable sessions (no child or archived ones) as `agentio.Conversation` | Disk read |
| `db.Turns(ctx, id, from, fn)` | turns with the cursor rule above, from one read transaction | Disk read |
| `db.Messages(ctx, id)` | every message with its parts (all part types, unknown keys kept) | Disk read |

<!-- snippet: examples/opencode/main.go#store -->
```go
// database opens opencode.db read-only with opencode/store, lists the
// resumable sessions with opencode's own totals, and reads one session's
// turns. The newest assistant message is still streaming, so the cursor
// stops before it.
func database(ctx context.Context, p printer, a *opencode.Agent) error {
	p.line("== opencode.db ==")
	db, err := store.OpenHome(a, a.DefaultHome())
	if err != nil {
		return err
	}
	// Read-only handle: a close error loses nothing.
	defer func() { _ = db.Close() }()
	ss, err := db.Sessions(ctx, store.SessionQuery{Roots: true, Active: true})
	if err != nil {
		return err
	}
	for _, s := range ss {
		p.line(fmt.Sprintf("session %s %q dir=%s v%s agent=%s cost=$%.2f in=%d out=%d", s.ID, s.Title, s.Directory, s.Version, s.Agent, s.Cost, s.Tokens.Input, s.Tokens.Output))
	}
	cursor, err := db.Turns(ctx, dbSession, 0, func(t agentio.Turn) error {
		p.line(fmt.Sprintf("  %s %s interim=%t model=%s %q", t.ID, t.Role, t.Interim, t.Model, t.Text))
		return nil
	})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("  cursor=%d (msg_e4 is still streaming)", cursor))
	return nil
}
```

## Step 4 — Launch and resume

`Launch` maps the neutral request onto opencode's flags:
- the TUI: `Prompt` is `--prompt`, `ResumeID` is `--session <id>`, `Model` is `--model provider/model`;
- `Headless` runs `opencode run --format json` with the same flags and the prompt as the message (after `--` when it starts with `-`);
- `AutonomyFull` adds `--auto` and needs `Target.AllowDangerous`; read-only, ask and accept-edits are refused, because opencode sets them only through permission rules in `opencode.json`;
- extra dirs and token logins are refused: opencode has no flag for extra dirs and keeps provider logins in `auth.json`;
- the default home leaves `XDG_DATA_HOME` as inherited; another home must be named `<base>/opencode` and sets `XDG_DATA_HOME=<base>`, which every XDG-aware program the session starts sees too.

<!-- snippet: examples/opencode/main.go#launch -->
```go
// launch builds commands through the neutral view: a new TUI run with a
// prompt, a resume, a headless run with full autonomy, and the levels
// opencode refuses.
func launch(p printer, a *opencode.Agent) error {
	p.line("== launch ==")
	n := a.Neutral()
	def := n.DefaultHome().Target()
	cmd, err := n.Launch(def, agentio.LaunchRequest{Prompt: "fix the failing tests", Model: "openai/gpt-5"})
	if err != nil {
		return err
	}
	p.line("new: " + cmd.String())
	cmd, err = n.Launch(def, agentio.LaunchRequest{ResumeID: legacySession, Dir: "/home/alice/proj"})
	if err != nil {
		return err
	}
	p.line("resume: " + cmd.String())
	full := def
	full.AllowDangerous = true
	cmd, err = n.Launch(full, agentio.LaunchRequest{Headless: true, Autonomy: agentio.AutonomyFull, Prompt: "run the tests"})
	if err != nil {
		return err
	}
	p.line("headless full: " + cmd.String())
	if _, err := n.Launch(def, agentio.LaunchRequest{Autonomy: agentio.AutonomyAsk}); err != nil {
		p.line("ask: " + err.Error())
	}
	return nil
}
```

## Step 5 — Processes, process sessions and stop plans

`ClassifyProcess` recognises the native binary (`opencode`, `opencode.exe`): no subcommand (the TUI) is `cli` and `run` is `exec`, both sessions; `serve`, `web` and `acp` are `app-server` and `attach` is a `bridge`, none a session; management subcommands (`providers`, `session`, `export`, `db`, ...) are `launcher`. An npm install started through node or bun runs the `opencode-ai` launcher script, which starts the native binary as its child, so it is a `launcher` too.

`ProcessSession` joins a session process to its conversation from the command line: `--session <id>`, `-s <id>` or `--session=<id>` gives the id with `JoinArgv`, unless `--fork` makes a new session. `--continue` and a new session name nothing (`JoinUnknown`). The home comes from the process's own `XDG_DATA_HOME` where its start environment is readable (linux, darwin), else from the Host env. `StopPlan` is the shared base plan over this classification: terminate the session process alone, resume it with `--session`.

<!-- snippet: examples/opencode/main.go#processes -->
```go
// processes classifies a few command lines and plans a stop for the TUI.
func processes(p printer, a *opencode.Agent) error {
	p.line("== processes ==")
	n := a.Neutral()
	rows := []agentio.Process{
		{PID: 101, Argv: []string{"/home/u/.opencode/bin/opencode", "-s", legacySession}},
		{PID: 102, Argv: []string{"opencode", "run", "fix it"}},
		{PID: 103, Argv: []string{"opencode", "serve", "--port", "4096"}},
		{PID: 104, Argv: []string{"node", "/usr/lib/node_modules/opencode-ai/bin/opencode"}},
	}
	for _, r := range rows {
		info, ok := a.ClassifyProcess(r)
		p.line(fmt.Sprintf("pid %d: opencode=%t kind=%s session=%t", r.PID, ok, info.Kind, info.Session))
	}
	planner, ok := agentio.StopPlannerOf(n)
	if !ok {
		return fmt.Errorf("opencode has no stop planner")
	}
	s := agentio.ProcessSession{Agent: opencode.ID, PID: 101, Home: n.DefaultHome(), ConversationID: legacySession, Dir: "/home/alice/proj"}
	plan, err := planner.StopPlan(rows[0], rows, s)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("stop 101: targets=%v signal=%s grace=%s resume=%s", plan.Targets, plan.Graceful, plan.Grace, plan.Resume))
	plan, err = planner.StopPlan(rows[2], rows, agentio.ProcessSession{})
	if err != nil {
		return err
	}
	p.line("stop 103: refused: " + plan.Refuse)
	return nil
}
```

## What is not supported, and why

| Capability | Reason |
|---|---|
| identity | one credential per model provider in `auth.json`; no single account identity, and this module never reads that file |
| usage, cached usage, subscription | no plan or quota of its own; per-message tokens and cost are not plan usage |
| token login, token prober | provider logins live in `auth.json` (`opencode providers login`); a per-provider token variable is not modelled |
| login commander | `opencode providers login` asks for a provider interactively; its flow is not modelled |
| history mover, conversation mover | the session directory sits inside database rows and legacy files; rewriting them is not modelled |
| prompt history | no prompt log apart from the sessions |
| live sessions | no lock or pid file per session; `ProcessSession` joins a process by its command line instead |
| home linker | the shareable files live in the XDG config dir, apart from the data dir |
| instruction files, MCP servers, skills, plugins, config | `opencode.json` and its directories are not modelled yet |
| models | the catalog is fetched from models.dev at run time |
| run events | the `opencode run --format json` event shape is not recorded from a real run yet |
| live credential | provider keys are read at start; no setting re-reads a key from a command |
| launch config | options beyond the neutral request (agent, variant, port, attach) are not typed; pass them as raw args |
| hooks | no shell-command hooks; plugins are JavaScript or TypeScript modules |

## Try it

```sh
cd examples && go run ./opencode
```

The full output is in [`examples/opencode/README.md`](../../examples/opencode/README.md).

## Pitfalls

- **The neutral view lists only legacy sessions.** Sessions created by opencode 1.2.0 or later are in `opencode.db`; read them with `opencode/store`. The doctor's `database` check says which store holds what.
- **A relative `XDG_DATA_HOME` is ignored.** opencode uses any value as is, so a relative one resolves against its working directory; the XDG spec says such a value is invalid, and this module falls back to `~/.local/share/opencode`.
- **Channel builds use another database.** A dev or local build writes `opencode-<channel>.db` unless `OPENCODE_DISABLE_CHANNEL_DB` is set; `DatabasePath` names the release file.
- **Do not count tokens twice.** Whether `Output` already includes `Reasoning` depends on the provider; the numbers are copied as opencode records them. The session totals in the database are opencode's own running sums.
- **The Windows data dir is unverified.** The bundled source resolves `~/.local/share/opencode` on every OS, while the 1.2.0 release note mentions `%APPDATA%` on Windows; no Windows install was recorded.

## Related

- [Targets and commands](../core/launch.md), [Homes](../core/homes.md), [Host](../core/host.md)
- Example: [`examples/opencode`](../../examples/opencode/)
