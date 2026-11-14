# pi

Walks the typed API of the pi coding agent (`agentio/pi`) on a temp copy of the 1.5.2 fixture agent dir: the install with its version read from the npm `package.json` (pi never runs), the homes, the sessions with title, model, turns and token cost, a `settings.json` round trip that keeps the keys this library does not model, the launch commands for a new run, a read-only headless run, a resume by session file and a provider token login, the autonomy levels pi refuses, the doctor, and the capability matrix with the reason for every gap.

The fixture is what pi 1.5.2 wrote on a first start with a throwaway home (`auth.json` and `models-store.json`, both `{}`) plus one session recorded against a local mock provider. The example adds an npm install layout (`npm/bin/pi` linking to `dist/bundle/cli.js`) and a `settings.json`, since a first start writes none. Nothing logs in and nothing leaves the process.

## Run

```sh
cd examples && go run ./pi
```

## Output

```text
== install ==
install: ~/npm/bin/pi -> ~/npm/lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js method=npm version=1.5.2 tested=1.5.2..1.5.2
== homes ==
home: personal default=true dir=~/.pi/agent
== sessions ==
session 0190f0e0 "my name" cwd=/home/fixture/proj model=localmock/mock-1 v3
  first prompt "hi there", last prompt "via partial id"
  turns: 4 user, 4 assistant
  usage: 4 messages, 480 input and 16 output tokens, $0.0000
== settings ==
settings: found=true provider=anthropic model=claude-sonnet unknown=[compaction hideThinkingBlock] warnings=0
round trip: {"compaction":{"enabled":false},"defaultModel":"claude-sonnet","defaultProvider":"anthropic","hideThinkingBlock":true}
default model: anthropic/claude-sonnet
== launch ==
new: pi -- "fix the failing tests"
read-only headless: pi --mode json --tools read,grep,find,ls --no-mcp --no-extensions -- "review src/"
resume: pi --session ~/.pi/agent/sessions/--home-fixture-proj--/2026-11-14T11-41-31-118Z_0190f0e0-0000-7000-8000-000000000001.jsonl
token login: pi ANTHROPIC_API_KEY=[redacted] OPENAI_API_KEY kept=true
ask: agentio: unsupported: pi: autonomy ask: pi has no permission modes; it always runs without prompts
accept-edits: agentio: unsupported: pi: autonomy accept-edits: pi has no permission modes; it always runs without prompts
== doctor ==
binary-found    ok    ~/npm/bin/pi
home            ok    ~/.pi/agent
version-tested  ok    1.5.2 is tested
settings        ok    ~/.pi/agent/settings.json
login           ok    env OPENAI_API_KEY
cache-warming   info  cacheWarming is idle (the default): pi keeps the prompt cache warm for up to 30 minutes after a run and spends tokens doing so
== capabilities ==
supported: [installer binary-version identity token-login conversations history-checker process-classifier turns instruction-files mcp-servers skills config doctor]
lacks usage: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
lacks cached-usage: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
lacks subscription: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
lacks token-prober: pi has no plan or quota to probe
lacks login-commander: pi logs in with /login inside its interactive UI; there is no login subcommand to build
lacks history-mover: pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
lacks conversation-mover: pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
lacks prompt-history: pi keeps no prompt history file; prompts live only in session files
lacks live-sessions: pi writes no live-session registry; only the process table shows a running pi
lacks home-linker: pi has no documented set of files to share between agent dirs
lacks plugins: pi packages and extensions are code loaded into the process, not plugins with a manifest this library models
lacks models: pi's model catalog (models-store.json) has no recorded shape yet
lacks run-events: pi's --mode json event stream is recorded once but not yet modelled as a decoder
```

## Test

```sh
cd examples && go test ./pi/
```

The test runs every section on a fixture copy and checks one line of each, and that the placeholder key is never printed. It is skipped on Windows, where npm installs `pi.cmd` instead of a symlink.

## Guides

- [pi](../../docs/pi/README.md)
