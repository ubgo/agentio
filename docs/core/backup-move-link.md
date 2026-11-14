# Backups, moves and home links

agentio is read-only by default. The few calls that write a file a vendor CLI owns go through a Writer door, and they all share the helpers on this page: back a file up before changing it, plan a move of one folder's conversations between two homes of the same agent and then apply it ([backup first, rollback on failure](ds:block?id=applymoveplan-sdj62uyw)), and link shared files of the default home (your instructions, skills, settings) into another home. You want this page when you build a tool that moves history between accounts or sets up a new home.

## Before you start

- Modules: `github.com/ubgo/agentio`, plus `claude` for a real move plan.
- Two homes of one agent on one filesystem. The example copies the fixture user home (`claude/testdata/homes/2.1.325/userhome`, with `.claude` and `.claude-work`) into a temp dir and only writes there:

<!-- snippet: examples/backupmove/main.go#copy-fixture -->
```go
// copyFixture copies a whole home tree. CopyTree keeps file modes, copies
// symlinks as links and refuses an existing destination, so it can never
// overwrite anything. Disk · Write.
func copyFixture(src, dst string) error {
	if err := agentio.CopyTree(src, dst); err != nil {
		return fmt.Errorf("copy fixture: %w", err)
	}
	return nil
}
```

## Step 1 — Back up a file

[`BackupFile(path, now)`](ds:block?id=backupfile-bxsxzm67) copies a file or a whole directory tree to [`BackupPath(path, now)`](ds:block?id=backuppath-tfjr5hjr), which is `<path>.agentio-bak-<UTC stamp>`, adding `-2`, `-3` … until the name is free, so a backup never overwrites another. A missing path returns an empty `Backup` ([`IsEmpty()`](ds:block?id=backup-isempty-4q86tar4) true) and no error: there was nothing to save. A `Backup` lists `BackupEntry{Original, Copy}` pairs; [`Merge`](ds:block?id=backup-merge-hg9treax) joins two. [`CopyTree(src, dst)`](ds:block?id=copytree-2u9s9bp6) is the copier underneath: it keeps file modes, copies symlinks as links and refuses an existing destination.

<!-- snippet: examples/backupmove/main.go#backup-file -->
```go
// backupFile saves settings.json before a (pretend) edit. Each backup gets
// a fresh name: a second backup in the same second gets a "-2" suffix
// instead of overwriting the first. A missing file returns an empty Backup.
// Disk · Write.
func backupFile(p printer, home string, now time.Time) error {
	path := filepath.Join(home, settingsFile)
	next, err := agentio.BackupPath(path, now)
	if err != nil {
		return err
	}
	p.line("backup path: " + next)
	first, err := agentio.BackupFile(path, now)
	if err != nil {
		return err
	}
	second, err := agentio.BackupFile(path, now)
	if err != nil {
		return err
	}
	all := first.Merge(second)
	for _, e := range all.Entries {
		p.line("backup: " + e.Original + " -> " + e.Copy)
	}
	missing, err := agentio.BackupFile(filepath.Join(home, "no-such-file.json"), now)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("backup of a missing file: empty=%t", missing.IsEmpty()))
	return nil
}
```

## Step 2 — Plan a move

A [`MoveRequest`](ds:block?id=moverequest-evw38rsj) names the source and target homes (`From`, `To`), the working folder whose conversations move (`Dir`) and optionally `OnlyIDs`. The agent turns it into a [`MovePlan`](ds:block?id=moveplan-3777j6wu) without changing anything: the conversation IDs, every `MoveItem{From, To}` rename, `Blockers` that stop the move (a live session in the folder, a target that already exists), `Left` (entries found but not moved, so nothing is dropped silently) and `Cleanup` (source dirs removed afterwards only if empty).

<!-- snippet: examples/backupmove/main.go#plan-move -->
```go
// planMove asks the agent what moving one folder's conversations from one
// home to another would do. Planning changes nothing: the plan lists every
// rename, what stays behind, and blockers that stop the move. Disk · Read.
func planMove(p printer, a *claude.Agent, from, to agentio.Home) (agentio.MovePlan, error) {
	plan, err := a.PlanMove(agentio.MoveRequest{From: from, To: to, Dir: movedDir})
	if err != nil {
		return agentio.MovePlan{}, fmt.Errorf("plan: %w", err)
	}
	p.line(fmt.Sprintf("plan: agent=%s dir=%s conversations=%v", plan.Agent, plan.Dir, plan.Conversations))
	for _, it := range plan.Items {
		p.line("plan: move " + it.From + " -> " + it.To)
	}
	p.line(fmt.Sprintf("plan: blockers=%d left=%d cleanup=%d", len(plan.Blockers), len(plan.Left), len(plan.Cleanup)))
	return plan, nil
}
```

[`claude.Agent.PlanMove`](ds:block?id=claude-agent-planmove-jj5mdn26) (also reachable as the `ConversationMover` capability, see [Conversations](../capabilities/conversations.md#step-4--check-and-move-history)) plans transcripts, per-conversation folders and, on a full move, the folder's auto memory.

## Step 3 — Apply a move plan

[`ApplyMovePlan(plan, now)`](ds:block?id=applymoveplan-sdj62uyw) executes a plan the same way for every agent. It refuses a plan with blockers (`ErrMoveBlocked`) or with homes of two different agents (`ErrForeignHome`). Otherwise it copies every source item into [one backup folder beside the source home](ds:block?id=backupmoveitems-g7vzvm4y) (`<from home>.agentio-bak-<stamp>/`, paths relative to the home), then renames each item into place. [If a rename fails](ds:block?id=rollbackmove-6gfza3bz), the items already moved are renamed back, so a failed move leaves both homes as they were.

<!-- snippet: examples/backupmove/main.go#apply-move -->
```go
// applyMove executes the plan: every source item is first copied into one
// backup dir beside the source home, then renamed into place; if a rename
// fails, the items already moved are renamed back. Disk · Write.
func applyMove(p printer, plan agentio.MovePlan, now time.Time) error {
	backup, err := agentio.ApplyMovePlan(plan, now)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	for _, e := range backup.Entries {
		p.line("apply: backed up " + e.Original + " -> " + e.Copy)
	}
	for _, it := range plan.Items {
		_, err := os.Stat(it.To)
		p.line(fmt.Sprintf("apply: %s present=%t", it.To, err == nil))
	}
	return nil
}
```

[A blocker stops the move before a single file is touched](ds:block?id=applymoveplan-sdj62uyw):

<!-- snippet: examples/backupmove/main.go#blocked-move -->
```go
// blockedMove plans a move whose target already holds the conversation.
// The plan carries a blocker, and ApplyMovePlan refuses it with
// ErrMoveBlocked before a single file is touched. Disk · Write (setup).
func blockedMove(p printer, a *claude.Agent, from, to claude.Home, now time.Time) error {
	src := filepath.Join(a.HistoryDir(from, blockedDir), blockedConvFile)
	dst := filepath.Join(a.HistoryDir(to, blockedDir), blockedConvFile)
	if err := agentio.CopyTree(src, dst); err != nil {
		return err
	}
	plan, err := a.PlanMove(agentio.MoveRequest{From: from.Neutral(), To: to.Neutral(), Dir: blockedDir})
	if err != nil {
		return err
	}
	for _, b := range plan.Blockers {
		p.line("blocked: " + b)
	}
	_, err = agentio.ApplyMovePlan(plan, now)
	p.line(fmt.Sprintf("blocked: apply refused=%t", errors.Is(err, agentio.ErrMoveBlocked)))
	return nil
}
```

In an application, apply through the agent's Writer door ([`claude.Agent.Writer().ApplyMove(plan)`](ds:block?id=claude-writer-applymove-tpwp98h7)), which also checks the plan belongs to Claude and uses the Host clock ([Claude writer](../claude/writer.md#step-4--move-history)).

## Step 4 — Plan and apply home links

A new home starts empty. [`PlanHomeLinks(def, h, names)`](ds:block?id=planhomelinks-yg65h8jg) decides which entries of the default home to symlink into `h`; [`Core.PlanHomeLinks(def, h)`](ds:block?id=core-planhomelinks-zybhmbpb) does the same for the Spec's `SharedHomeFiles` (`CLAUDE.md`, `skills`, [`settings.json`](ds:block?id=claude-sharedhomefiles-3vpa9mqk) for Claude). A name missing from the default home is left out; a target that already exists (even a broken symlink) is planned with `Exists` true. [`ApplyHomeLinks`](ds:block?id=applyhomelinks-svx7cuvm) makes every link that does not exist yet and never replaces anything, so there is nothing to back up.

<!-- snippet: examples/backupmove/main.go#home-links -->
```go
// homeLinks links the shared files of the default home (CLAUDE.md,
// skills/, settings.json for Claude) into the work home. The plan marks a
// target that already exists; applying never replaces it, so there is
// nothing to back up. Plan is Disk · Read, apply is Disk · Write.
func homeLinks(p printer, h agentio.Host, def, work agentio.Home) error {
	core, err := agentio.NewCore(claude.Spec(), h)
	if err != nil {
		return err
	}
	links, err := core.PlanHomeLinks(def, work)
	if err != nil {
		return err
	}
	for _, l := range links {
		p.line(fmt.Sprintf("link plan: %s exists=%t", l.Name, l.Exists))
	}
	if err := core.ApplyHomeLinks(links); err != nil {
		return err
	}
	for _, l := range links {
		if target, err := os.Readlink(l.To); err == nil {
			p.line("linked: " + l.To + " -> " + target)
		}
	}
	// The package function works on any name list, for two homes of one agent.
	again, err := agentio.PlanHomeLinks(def, work, []string{"CLAUDE.md"})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("link plan again: %s exists=%t", again[0].Name, again[0].Exists))
	return nil
}
```

The neutral capability for this is [`HomeLinker`](ds:block?id=homelinker-ek5zfyg7) ([`agentio.HomeLinkerOf`](ds:block?id=homelinkerof-fk3qdrnr)), described in [Doors](../capabilities/doors.md#step-4--link-shared-home-files).

## Try it

```sh
cd examples && go run ./backupmove
```

```text
backup path: ~/.claude/settings.json.agentio-bak-20261114T120000Z
backup: ~/.claude/settings.json -> ~/.claude/settings.json.agentio-bak-20261114T120000Z
backup: ~/.claude/settings.json -> ~/.claude/settings.json.agentio-bak-20261114T120000Z-2
backup of a missing file: empty=true
plan: agent=claude dir=/work/proj12 conversations=[f85efbdb-fc2f-1e8e-7748-9f915edc8e2d]
plan: move ~/.claude/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl -> ~/.claude-work/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl
plan: blockers=0 left=0 cleanup=2
apply: backed up ~/.claude/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl -> ~/.claude.agentio-bak-20261114T120000Z/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl
apply: ~/.claude-work/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl present=true
blocked: ~/.claude-work/projects/-work-proj13/3f474440-903e-5410-8bfb-d537b89f9547.jsonl already exists in the target home
blocked: apply refused=true
link plan: CLAUDE.md exists=false
link plan: skills exists=false
link plan: settings.json exists=true
linked: ~/.claude-work/CLAUDE.md -> ~/.claude/CLAUDE.md
linked: ~/.claude-work/skills -> ~/.claude/skills
link plan again: CLAUDE.md exists=true
```

`~` is the temp copy of the fixture user home; the fixture itself is never changed.

## Pitfalls

- **[`BackupPath`](ds:block?id=backuppath-tfjr5hjr) does not reserve the name.** It returns the first free name at call time; `BackupFile` picks its own name the same way, so call `BackupFile` and read the name from the returned [`Backup`](ds:block?id=backupfile-bxsxzm67).
- **Backups use the clock you pass.** [Two backups in the same second get](ds:block?id=backuppath-tfjr5hjr) `-2`, `-3`; [the stamp is UTC](ds:block?id=backupstamp-ur2d5sev) and has no `:` so it is a valid Windows file name.
- [**Moves need one filesystem.**](ds:block?id=applymoveplan-sdj62uyw) Items are renamed, not copied; a target home on another device fails the first rename and triggers the rollback. The backup folder stays either way.
- [**A failed rollback is reported, not hidden.**](ds:block?id=applymoveplan-sdj62uyw) If renaming back also fails, the error says so and points you at the backup folder.
- [**Cleanup only removes empty folders.**](ds:block?id=applymoveplan-sdj62uyw) A source folder with entries listed in `Left` stays.
- [**Codex conversations cannot move between homes.**](ds:block?id=codex-agent-moveunsupportedreason-yn4v8j7h) Codex indexes them in SQLite with absolute paths, so the Codex agent does not implement the move capabilities; [`agentio.Unsupported`](ds:block?id=unsupported-zhw69bda) tells you why.
- [**`ApplyHomeLinks` never overwrites.**](ds:block?id=applyhomelinks-svx7cuvm) A file that appeared after planning makes `os.Symlink` fail rather than being replaced. Each planned [`HomeLink`](ds:block?id=homelink-a86nv2s4) carries its `Agent`; [`Core.ApplyHomeLinks`](ds:block?id=core-applyhomelinks-q5mmu4xp) and [`claude.Writer.ApplyHomeLinks`](ds:block?id=claude-writer-applyhomelinks-39hcpw7p) refuse a link of another agent with `ErrForeignHome` (checked with [`CheckLinksOwner`](ds:block?id=checklinksowner-czbw4f2d)), as `ApplyMove` does. The free function `ApplyHomeLinks` has no agent to check against. On Windows a symlink needs Developer Mode or an elevated process; without it the link fails with a `*agentio.SymlinkPrivilegeError`, which matches `ErrUnsupported`.
- **Call these from a Writer door in application code.** The package functions are the shared implementation; [the vendor Writer adds the owner check and the Host clock](ds:block?id=claude-writer-applymove-tpwp98h7).

## Related

- [Homes](homes.md)
- [Doors: Runner, Prober and Writer](../capabilities/doors.md)
- [Conversations, turns and prompt history](../capabilities/conversations.md)
- [Claude writer: settings and hooks](../claude/writer.md)
- Example: [`examples/backupmove`](../../examples/backupmove/)
- [Feature index](../reference/features.md)
