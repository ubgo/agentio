# Codex launch, exec and event decoding

Builds Codex command lines without running them: interactive start, resume, resume last, fork, a non-default home (`CODEX_HOME` set), the guard errors (no session id, bypass without `AllowDangerous`, profile conflict), `codex exec`, `exec resume` and `exec review`, the vendor's archive/delete/queue/doctor/server/feature commands, a flag refused for an old version, and the `codex exec --json` decoder (typed and neutral) fed from a recorded stream. A placeholder `codex` on `PATH` gives the builders a binary path; it is never run.

## Run it

```sh
cd examples && go run ./codexlaunch
```

The test (`go test ./codexlaunch/`) runs the same code and checks the key lines below.

## Output

```text
new          codex -c model_reasoning_effort="high" -m gpt-5.5 -s workspace-write -a on-request --add-dir /work/shared "fix the build"  [warning version-unknown]
resume       codex resume 0199e1a0-0000-7000-8000-000000000001
resume last  codex resume --last --all
fork         codex fork 0199e1a0-0000-7000-8000-000000000001 "try another way"
other home   codex   [CODEX_HOME=work-codex]  [warning version-unknown]
resume without an id: ErrNoSessionID
bypass without Target.AllowDangerous: ErrDangerousNotAllowed
profile work: ErrProfileConflict
exec         codex exec -s read-only -c approval_policy="never" --json -o /tmp/last.txt --skip-git-repo-check -
exec resume  codex exec resume --json 0199e1a0-0000-7000-8000-000000000001 continue
review       codex exec review --base main --title "Release review" --json
archive      codex archive 0199e1a0-0000-7000-8000-000000000001
unarchive    codex unarchive 0199e1a0-0000-7000-8000-000000000001
delete       codex delete --force 0199e1a0-0000-7000-8000-000000000001
queue        codex queue --thread 0199e1a0-0000-7000-8000-000000000001 --message "run the tests next"
doctor       codex doctor --json
mcp-server   codex mcp-server
app-server   codex app-server --listen ws://127.0.0.1:8390
feature      codex features enable hooks
feature=--no-daemon installed=0.150.0 since=0.156.0 neutral match=true
span known=true since=0.156.0
decoder written against codex 0.162.1
event thread.started thread=th-1
event turn.started known=true
event item.completed agent_message "done"
event item.completed command "ls" exit=0
event turn.completed input=10 output=3
event future.event known=false
parsed turn.failed error="quota"
run event started
run event other
run event message
run event tool
run event done reasoning=1
run event other
```

## Guides

- [Codex launch, resume and exec](../../docs/codex/launch.md)
- [Codex exec event decoding](../../docs/codex/exec.md)
- [Feature index](../../docs/reference/features.md)
