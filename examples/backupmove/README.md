# backupmove

The write helpers behind every Writer door: `CopyTree`, `BackupPath`, `BackupFile`, `Backup.Merge`; planning a conversation move between two Claude homes (`MoveRequest`, `MovePlan`) and applying it (`ApplyMovePlan`, `ErrMoveBlocked`); planning and applying home links (`Core.PlanHomeLinks`, `PlanHomeLinks`, `ApplyHomeLinks`).

Copies the fixture user home `claude/testdata/homes/2.1.325/userhome` into a temp dir (`~` in the output) and writes only there; the fixture is never changed.

## Run

```sh
cd examples && go run ./backupmove
```

## Output

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

## Test

```sh
cd examples && go test ./backupmove/
```

The test runs the same `run` function against the fixture and checks the key output lines.

## Guides

- [Backups, moves and home links](../../docs/core/backup-move-link.md)
- [Feature index](../../docs/reference/features.md)
