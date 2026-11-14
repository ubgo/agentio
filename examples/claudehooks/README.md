# claudehooks

Hooks, the status line and the `Writer` door: listing the hooks of every settings layer, matcher rules, a complete `PreToolUse` hook program and a complete status-line program (each decoding a sample input from `testdata/`), then patching settings files, adding and removing a hook, moving a folder's history, moving conversations to another home and linking shared files. Every write backs its file up first; the demo only writes inside its temp copy.

Everything runs on a temp copy of the 2.1.325 fixture home in this repository (`claude/testdata/homes/2.1.325/userhome`), through a `Host` built by hand: no real home, no `claude` binary, no network. Paths under the temp user home print as `~`.

## Run it

```sh
cd examples && go run ./claudehooks
```

The same binary is also a real hook and a real status-line command:

```sh
cd examples && go run ./claudehooks hook < claudehooks/testdata/pretooluse.json
cd examples && go run ./claudehooks statusline < claudehooks/testdata/statusline.json
```

```text
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"recursive delete is blocked by policy"}}
[Opus] src $0.42 +12 -3
```

The two inputs follow the documented hook and status-line shapes with values replaced; the status-line shape is not verified against a recording.

Test: `cd examples && go test ./claudehooks/...` runs the same code and checks the output lines.

## Output

```text
== configured hooks
project Stop              matcher=""                   handlers=1
user    Elicitation       matcher=""                   handlers=1
user    Notification      matcher=""                   handlers=1
user    PermissionRequest matcher=""                   handlers=1
user    PostToolUse       matcher=""                   handlers=1
user    PostToolUse       matcher="Write|Edit|MultiEdit" handlers=1
user    PreToolUse        matcher="Bash"               handlers=4
user    SessionEnd        matcher=""                   handlers=1
user    SessionStart      matcher=""                   handlers=1
user    Stop              matcher=""                   handlers=1
user    StopFailure       matcher=""                   handlers=1
user    UserPromptSubmit  matcher=""                   handlers=1
first handler: type=command command=/usr/local/bin/fixture-hook timeout=10s

== events and matchers
documented events=33, 17 take a matcher, e.g. [SessionStart PreToolUse PermissionRequest]
Stop takes a matcher: false
matcher "Write|Edit|MultiEdit" tool Edit                 -> true
matcher "Write|Edit|MultiEdit" tool NotebookEdit         -> false
matcher "mcp__.*__write.*"     tool mcp__fs__write_file  -> true
matcher ""                     tool Bash                 -> true

== hook program
event=PreToolUse tool=Bash mode=manual effort=high unknown=[new_field]
reply: {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"recursive delete is blocked by policy"}}

== status line
setting: type=command command=/usr/local/bin/my-statusline
input: model=claude-opus-5-5 project=/work/project style=default unknown=[exceeds_200k_tokens]
prints: [Opus] src $0.42 +12 -3

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

## Guides

- [Claude hooks](../../docs/claude/hooks.md)
- [Claude status line](../../docs/claude/statusline.md)
- [Claude writer: settings and hooks](../../docs/claude/writer.md)
- [Feature index](../../docs/reference/features.md)
