# Claude writer: settings and hooks

Everything in the `claude` module reads, except the `Writer` door. [`a.Writer()`](ds:block?id=claude-writer-t5gq26t8) changes settings files, adds and removes hooks, and moves conversation history, and every method that changes a file backs its target up first ([`<file>.agentio-bak-<UTC stamp>`](ds:block?id=backuppath-tfjr5hjr), never overwriting an earlier backup) and returns the `agentio.Backup` it made; `ApplyHomeLinks` only adds symlinks, so it has nothing to back up. Use it when your tool installs a hook, flips a setting, or follows a renamed project folder.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent, a home and (for project files) a project (see [The Claude agent](agent.md)).
- Every method here is a Disk write. Run the example's demo only on copies: it copies the fixture home to a temp dir first.
- [The writer never edits `.claude.json` or a project `.mcp.json`.](ds:block?id=claude-writer-t5gq26t8) Those changes are vendor commands you run (see [Claude MCP servers](mcp.md) and [Claude plugins](plugins.md)).

The snippets come from [`examples/claudehooks`](../../examples/claudehooks/).

## Step 1 — Patch a settings file

[`SetUserSetting(h, patch)`](ds:block?id=claude-writer-setusersetting-hfm5v3t7) edits `<home>/settings.json`; [`SetProjectSetting(p, local, patch)`](ds:block?id=claude-writer-setprojectsetting-9xh95gnh) edits `.claude/settings.json` (`local` false) or `.claude/settings.local.json` (`local` true). A `SettingsPatch` sets top-level keys to raw JSON values (`Set`) and removes keys (`Delete`); [every other key is kept as it was](ds:block?id=claude-writer-applypatch-utnp7z3r). [The file is written atomically (temp file, then rename) with its mode kept.](ds:block?id=claude-writer-writerawsettings-6sjvkkgu)

<!-- snippet: examples/claudehooks/main.go#patch -->
```go
// patchSettings changes top-level keys of the user and local settings files.
// Every other key is kept, and the old file is backed up first.
func patchSettings(p printer, a *claude.Agent) error {
	p.section("patch settings")
	w := a.Writer()
	b, err := w.SetUserSetting(a.DefaultHome(), claude.SettingsPatch{
		Set:    map[string]agentio.Raw{"model": agentio.Raw(`"sonnet"`)},
		Delete: []string{"tui"},
	})
	if err != nil {
		return err
	}
	printBackup(p, b)
	proj, err := project(a)
	if err != nil {
		return err
	}
	b, err = w.SetProjectSetting(proj, true, claude.SettingsPatch{ // true: settings.local.json
		Set: map[string]agentio.Raw{"outputStyle": agentio.Raw(`"Explanatory"`)},
	})
	if err != nil {
		return err
	}
	printBackup(p, b)
	user, err := a.UserSettings(a.DefaultHome())
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("user model=%s tui=%q theme=%s hooks kept=%d events", user.Settings.Model, user.Settings.TUI, user.Settings.Theme, len(user.Settings.Hooks)))
	if _, err := w.SetUserSetting(a.DefaultHome(), claude.SettingsPatch{Set: map[string]agentio.Raw{"model": agentio.Raw(`sonnet`)}}); err != nil {
		p.line(fmt.Sprintf("invalid JSON value refused: %v", err))
	}
	return nil
}
```

## Step 2 — Add a hook

[`AddHook(h, p, scope, event, matcher)`](ds:block?id=claude-writer-addhook-y3s8tkg6) appends a matcher group for the event to the settings file of the scope (`ScopeUser` uses `h`; [`ScopeProject` and `ScopeLocal` need `p`](ds:block?id=claude-agent-settingspath-g6smumdx)). Existing hooks and keys stay.

<!-- snippet: examples/claudehooks/main.go#addhook -->
```go
// addHook appends a Stop hook to the user settings; existing hooks stay.
func addHook(p printer, a *claude.Agent) error {
	p.section("add a hook")
	h := a.DefaultHome()
	b, err := a.Writer().AddHook(h, nil, claude.ScopeUser, claude.HookStop, claude.HookMatcher{
		Hooks: []claude.HookHandler{{Type: claude.HookTypeCommand, Command: ourHookCommand, Timeout: ourHookTimeout}},
	})
	if err != nil {
		return err
	}
	printBackup(p, b)
	user, err := a.UserSettings(h)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("Stop matcher groups now=%d", len(user.Settings.Hooks[claude.HookStop])))
	return nil
}
```

## Step 3 — Remove hooks

[`RemoveHooks(h, p, scope, keep)`](ds:block?id=claude-writer-removehooks-x8p95azs) drops every handler for which `keep` returns false, then drops matcher groups and events left empty. It returns how many handlers it removed. When nothing matches, nothing is written and the backup is empty.

<!-- snippet: examples/claudehooks/main.go#removehooks -->
```go
// removeHooks drops the handler addHook added. keep returns false for the
// handlers to remove; a run that removes nothing writes nothing.
func removeHooks(p printer, a *claude.Agent) error {
	p.section("remove hooks")
	h := a.DefaultHome()
	keep := func(_ claude.HookEvent, hh claude.HookHandler) bool { return hh.Command != ourHookCommand }
	n, b, err := a.Writer().RemoveHooks(h, nil, claude.ScopeUser, keep)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("removed=%d", n))
	printBackup(p, b)
	n, b, err = a.Writer().RemoveHooks(h, nil, claude.ScopeUser, keep)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("second run removed=%d backup empty=%t", n, b.IsEmpty()))
	return nil
}
```

## Step 4 — Move history

[`MoveHistory(h, from, to)`](ds:block?id=claude-writer-movehistory-3n8zhjz3) re-keys a folder's history after the folder moved on disk, by renaming `<home>/projects/<EncodePath(from)>`. Claude Code finds conversations by the encoded folder, never by the cwd inside old lines, so the rename is enough for `claude --resume` to find them from the new path. It returns an `agentio.HistoryMove`: `Moved` false with a nil error when `from` has no history, otherwise `Moved` true and the `Backup`. An existing destination is `claude.ErrHistoryExists`, and a Claude session running in `from` (or one of its conversations being live) is a [`*claude.LiveSessionError`](ds:block?id=claude-livesessionerror-unwrap-parprkp9) matching `agentio.ErrMoveBlocked`, with the same `Blockers()` text `PlanMove` lists.

<!-- snippet: examples/claudehooks/main.go#movehistory -->
```go
// moveHistory re-keys a folder's history after the folder was renamed, so
// `claude --resume` finds the old conversations from the new path.
func moveHistory(p printer, a *claude.Agent) error {
	p.section("move history")
	h := a.DefaultHome()
	res, err := a.Writer().MoveHistory(h, movedFrom, movedTo)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("moved=%t now at %s", res.Moved, a.HistoryDir(h, movedTo)))
	printBackup(p, res.Backup)
	res, err = a.Writer().MoveHistory(h, "/work/no-history", movedTo)
	p.line(fmt.Sprintf("folder without history: moved=%t err=%v", res.Moved, err))
	if err := os.MkdirAll(a.HistoryDir(h, movedFrom), 0o700); err != nil {
		return err
	}
	_, err = a.Writer().MoveHistory(h, movedFrom, movedTo)
	p.line(fmt.Sprintf("destination exists: ErrHistoryExists=%t", errors.Is(err, claude.ErrHistoryExists)))
	return nil
}
```

To move one folder's conversations to another home (for example from the personal to the work home), plan with [`a.PlanMove`](ds:block?id=claude-agent-planmove-jj5mdn26) (a Disk read that changes nothing and lists blockers such as a live session in the folder), then apply with [`Writer.ApplyMove`](ds:block?id=applymoveplan-sdj62uyw), which backs up every item, renames each into place and rolls back on failure.

<!-- snippet: examples/claudehooks/main.go#moveacross -->
```go
// moveAcrossHomes plans moving one folder's conversations from the default
// home to the work home, then applies the plan through the Writer.
func moveAcrossHomes(p printer, a *claude.Agent) error {
	p.section("move between homes")
	from := a.DefaultHome()
	to := a.HomeAt(filepath.Join(a.Host().UserHome, workHome))
	plan, err := a.PlanMove(agentio.MoveRequest{From: from.Neutral(), To: to.Neutral(), Dir: movedAcrossDir})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("plan: %d conversation(s), %d item(s), blockers=%v", len(plan.Conversations), len(plan.Items), plan.Blockers))
	if len(plan.Blockers) > 0 {
		return nil
	}
	b, err := a.Writer().ApplyMove(plan)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("applied, %d backup entr(ies); work home has history=%t", len(b.Entries), hasHistory(a, to, movedAcrossDir)))
	return nil
}
```

[`Writer.ApplyHomeLinks`](ds:block?id=applyhomelinks-svx7cuvm) makes the symlinks `agentio.PlanHomeLinks` planned, so a second home shares the default home's `CLAUDE.md`, `skills` and `settings.json`. [A target that already exists is planned with `Exists` true and never overwritten.](ds:block?id=planhomelinks-yg65h8jg)

<!-- snippet: examples/claudehooks/main.go#link -->
```go
// linkHome shares the default home's CLAUDE.md, skills and settings with
// the work home by symlink. Existing targets are never overwritten.
func linkHome(p printer, a *claude.Agent) error {
	p.section("link shared files")
	def := a.DefaultHome()
	work := a.HomeAt(filepath.Join(a.Host().UserHome, workHome))
	links, err := agentio.PlanHomeLinks(def.Neutral(), work.Neutral(), []string{"CLAUDE.md", "skills", "settings.json"})
	if err != nil {
		return err
	}
	for _, l := range links {
		p.line(fmt.Sprintf("plan %s -> %s exists=%t", l.To, l.From, l.Exists))
	}
	if err := a.Writer().ApplyHomeLinks(links); err != nil {
		return err
	}
	target, err := os.Readlink(filepath.Join(work.Dir(), "CLAUDE.md"))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("work CLAUDE.md -> %s", target))
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudehooks
```

The sections this page covers (backup stamps come from the Host clock, so they are stable):

```text
== patch settings
backup: ~/.claude/settings.json saved as <file>.agentio-bak-20261114T120000Z-2
backup: ~/project/.claude/settings.local.json saved as <file>.agentio-bak-20261114T120000Z
user model=sonnet tui="" theme=dark hooks kept=10 events
invalid JSON value refused: claude: settings value for "model" is not valid JSON

== add a hook
backup: ~/.claude/settings.json saved as <file>.agentio-bak-20261114T120000Z-3
Stop matcher groups now=2

== remove hooks
removed=1
backup: ~/.claude/settings.json saved as <file>.agentio-bak-20261114T120000Z-4
second run removed=0 backup empty=true

== move history
moved=true now at ~/.claude/projects/-work-proj12-renamed
backup: ~/.claude/projects/-work-proj12 saved as <file>.agentio-bak-20261114T120000Z
folder without history: moved=false err=<nil>
destination exists: ErrHistoryExists=true

== move between homes
plan: 1 conversation(s), 1 item(s), blockers=[]
applied, 1 backup entr(ies); work home has history=true

== link shared files
plan ~/.claude-work/CLAUDE.md -> ~/.claude/CLAUDE.md exists=false
plan ~/.claude-work/skills -> ~/.claude/skills exists=false
plan ~/.claude-work/settings.json -> ~/.claude/settings.json exists=true
work CLAUDE.md -> ~/.claude/CLAUDE.md
```

The first `settings.json` backup (stamp without a suffix) was taken earlier in the run, when the status-line section set `statusLine`.

## Pitfalls

- **[Backups pile up.](ds:block?id=backuppath-tfjr5hjr)** Each call leaves a `<file>.agentio-bak-<stamp>` next to the file (a `-N` suffix when the stamp is taken); nothing cleans them. Tell your users where they are, or remove them yourself once a change is confirmed.
- **Values in `SettingsPatch.Set` are raw JSON.** A string needs its quotes (`"sonnet"`, not `sonnet`); [an invalid value is refused before anything is written](ds:block?id=claude-writer-applypatch-utnp7z3r).
- A settings file that is not a JSON object, or whose `hooks` key does not decode, [is refused rather than rewritten](ds:block?id=claude-readrawsettings-peagek4s), so a hand-edited file with a mistake is never clobbered.
- [`AddHook` appends a new group every call](ds:block?id=claude-writer-addhook-y3s8tkg6); calling it twice installs the hook twice. Check `a.Hooks` first if your installer may run again.
- [`MoveHistory` moves only the history folder for that path in that home](ds:block?id=claude-writer-movehistory-3n8zhjz3); the folder's state entry in `.claude.json` (trust, last session) stays under the old path.
- [`ApplyMove` refuses a plan made for another agent with `agentio.ErrForeignHome`.](ds:block?id=claude-writer-applymove-tpwp98h7) [`ApplyHomeLinks` returns no `Backup`](ds:block?id=claude-writer-applyhomelinks-39hcpw7p) because it never overwrites.
- `MoveHistory` and `PlanMove` both refuse while a Claude Code session uses the folder: `MoveHistory` returns `*claude.LiveSessionError` (`errors.Is(err, agentio.ErrMoveBlocked)`), [`PlanMove` lists the same text in `Blockers`](ds:block?id=claude-agent-liveblockers-apwxxpeb). [The check reads `Host.Processes`; a Host without a process lister sees no live session.](ds:block?id=claude-agent-processtable-c26mufrf)
- [`ApplyHomeLinks` checks ownership like `ApplyMove`](ds:block?id=checklinksowner-czbw4f2d): a link planned for another agent's homes (`HomeLink.Agent`) is `agentio.ErrForeignHome` and nothing is linked.

## Related

- [Claude hooks](hooks.md), [Claude settings](settings.md), [Claude paths](paths.md), [Claude conversations](conversations.md)
- Neutral view: [The doors](../capabilities/doors.md), [Moves, backups and links](../core/backup-move-link.md)
- Example: [`examples/claudehooks`](../../examples/claudehooks/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
