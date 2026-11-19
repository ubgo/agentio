# opencode

Walks the typed opencode API (`agentio/opencode` and `agentio/opencode/store`) on a temp copy of the 1.18.30 fixture user home: the install with its version read from the npm `package.json` (opencode never runs), the data dir as the home with its database and config paths, the legacy `storage/` sessions with their turns and cursor, the sessions and turns of an `opencode.db` built in the temp dir and read read-only, the launch commands for a new run, a resume and a headless run with full autonomy, the level opencode refuses, process classification with a stop plan, the doctor, and a few capability gaps with their reasons.

The fixture is the legacy JSON tree a real machine still holds from opencode 0.14.7 and 1.1.14 (every value replaced). The example adds an npm install layout (`npm/bin/opencode` linking to the `opencode-ai` launcher script) and builds `opencode.db` from the recorded 1.18.30 schema in `opencode/store/testdata` with a few made-up rows, one of them an assistant message still streaming. Nothing logs in and nothing leaves the process.

## Run

```sh
cd examples && go run ./opencode
```

## Output

```text
== install ==
install: ~/npm/bin/opencode -> ~/npm/lib/node_modules/opencode-ai/bin/opencode method=npm version=1.18.30 tested=1.18.30..1.18.30
== homes ==
home: personal default=true dir=~/.local/share/opencode
  database=~/.local/share/opencode/opencode.db config=~/.config/opencode
== legacy storage ==
session ses_0f0000000002ffeFixtureBBBB "New session - 2026-04-02T03:32:00.615Z" dir=/home/alice/other v1.1.14
session ses_0f0000000001ffeFixtureAAAA "Fix the login form" dir=/home/alice/proj v0.14.7
  msg_0f0000000001AAAAAAAAAAAA01 user "fix the login form"
  msg_0f0000000002AAAAAAAAAAAA02 assistant "I fixed the login form." in=1200 cached=1000 out=300 reasoning=120
  msg_0f0000000003AAAAAAAAAAAA03 user "now add a test"
  msg_0f0000000004AAAAAAAAAAAA04 assistant "Added login.test.ts." in=1500 cached=1200 out=400 reasoning=0
  cursor=4, re-read from it: cursor=4
== opencode.db ==
session ses_0f0000000100ffeExampleDB01 "Add a health check" dir=/work/app v1.18.30 agent=build cost=$0.04 in=2300 out=410
  msg_e1 user interim=false model=openai/gpt-5 "add a /health endpoint"
  msg_e2 assistant interim=true model=openai/gpt-5 "Reading the router first."
  msg_e3 assistant interim=false model=openai/gpt-5 "Added GET /health returning 200."
  cursor=3 (msg_e4 is still streaming)
== launch ==
new: opencode --model openai/gpt-5 --prompt "fix the failing tests"
resume: opencode --session ses_0f0000000001ffeFixtureAAAA
headless full: opencode run --format json --auto "run the tests"
ask: agentio: unsupported: opencode: autonomy ask: opencode has no flag for this level; only --auto (full) exists, other levels are permission rules in opencode.json
== processes ==
pid 101: opencode=true kind=cli session=true
pid 102: opencode=true kind=exec session=true
pid 103: opencode=true kind=app-server session=false
pid 104: opencode=true kind=launcher session=false
stop 101: targets=[101] signal=terminate grace=4s resume=opencode --session ses_0f0000000001ffeFixtureAAAA
stop 103: refused: not a conversation process: app-server
== doctor ==
binary-found    ok    ~/npm/bin/opencode
home            ok    ~/.local/share/opencode
version-tested  ok    1.18.30 is tested
database        info  ~/.local/share/opencode/opencode.db holds the sessions since opencode 1.2.0 (read them with opencode/store); this package lists 2 legacy sessions from storage/
auto-update     info  OPENCODE_DISABLE_AUTOUPDATE is unset: opencode updates itself
== capabilities ==
supported: [installer binary-version conversations history-checker process-classifier turns doctor process-sessions stop-planner turn-files]
lacks usage: opencode has no plan or quota of its own; it bills through each provider's key, and per-message tokens and cost are not plan usage
lacks hooks: opencode has no shell-command hooks; its plugins are JavaScript or TypeScript modules
lacks config: opencode's config (opencode.json with instructions, mcp, agents and plugins in the XDG config dir and the project) is not modelled yet
```
