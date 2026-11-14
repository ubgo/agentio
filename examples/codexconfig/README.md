# Codex config, install and extensions

Walks the disk-only API of the `codex` module on a temp copy of the checked-in 0.162.1 fixture home: the agent and its homes, the install (a fake npm install of codex on `PATH`) and update cache, the Runner door answered by a canned process runner, paths and inventory, `config.toml` (file, effective layers, profiles, `-c` overrides), MCP servers, instruction files, skills, plugins, hooks, execpolicy rules and the doctor. Nothing reads the real machine, runs a real binary or touches the network.

## Run it

```sh
cd examples && go run ./codexconfig
```

Each topic lives in its own file (`agent.go`, `install.go`, `config.go`, `extensions.go`, `hooks.go`); `main.go` builds the fake machine. `-fixture DIR` copies another Codex home instead of the default `../codex/testdata/homes/0.162.1`.

The test (`go test ./codexconfig/`) runs the same code and checks the key lines below.

## Output

```text
== agent
id=codex name=Codex binary=codex home-env=CODEX_HOME default-home=.codex
default home keeps CODEX_HOME in the child env: false
installation id: [redacted]
default: ~/.codex (default=true)
found home "personal" at ~/.codex
CODEX_HOME set: false
is a home: true
project dir is not a home: true
neutral round trip: true
neutral id=codex capabilities=22
conversation mover unsupported: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
doctor checks via the neutral view: 7
== install
path=~/npm/bin/codex method=npm version=0.162.1
package root=~/npm/lib/node_modules/@openai/codex
parsed 0.158.0-alpha.2.1 pre="alpha.2.1"
update: npm install -g @openai/codex
latest seen=0.156.1 checked=2026-09-24 dismissed=false
binary version: 0.162.1
login status: logged-in=true method=apikey
feature hooks stage="stable" enabled=true
feature unified_exec stage="under development" enabled=false
parsed rows: 1
bundled models: gpt-6-luna
== paths
config=~/.codex/config.toml auth=~/.codex/auth.json
sessions=~/.codex/sessions state db=~/.codex/state_5.sqlite
v2 profile work=~/.codex/work.config.toml
sqlite home with config=~/.codex
present: Config AgentsMD Rules Hooks Auth InstallationID Sessions SessionIndex History ModelsCache VersionCache Skills Plugins ThreadWriterLocks
unknown top-level entries: 0
== config
model=gpt-5.6-terra effort=medium (known=true) warnings=0
sandbox_mode set=false approval_policy set=false
keys total=56 typed=50 raw=3 unknown=3
unknown some_future_key.n = 3
unknown some_future_key.nested = true
approval=on-request known=true
sandbox=brand-new-mode known=false
warning: type-changed: inline.toml: model_context_window: expected integer, found string
layer user         exists=true  applied
layer project      exists=true  applied
effective model from project
effective model_reasoning_effort from user-profile
effective model_verbosity from session
model=project-model not evaluated=[mdm cloud]
trust of /work/proj/sub: trusted
feature overrides: map[hooks:true js_repl:false]
profile ollama-launch kind=legacy table=true file=~/.codex/config.toml
profile work kind=v2 table=false file=~/.codex/work.config.toml
v2 profile files: [work]
legacy names: [ollama-launch]
conflict: codex: profile name is also a legacy [profiles] entry: "ollama-launch" (rename or remove the legacy profile, or use a different v2 profile name)
-c model="gpt-5.5"
-c features.hooks=true
-c model_context_window=200000
-c notify=["/bin/notify", "--quiet"]
-c sandbox_mode="read-only"
-c model_auto_compact_token_limit=1000
rejected: no '='
== mcp
mcp computer-use transport=stdio layer=user enabled-set=true env-keys=[]
mcp node_repl transport=stdio layer=user enabled-set=false env-keys=[CODEX_HOME SECRET_TOKEN]
SECRET_TOKEN prints as [redacted]
MISSING: not in config.toml
codex mcp add docs --url https://mcp.example.test --bearer-token-env-var DOCS_TOKEN
codex mcp add files -- /opt/mcp/files --root /work
codex mcp remove docs
env values refused: true
== instructions and skills
instructions ~/.codex/AGENTS.md scope=user kind=AGENTS.md bytes=0 truncated=false
instructions ~/work/proj/AGENTS.md scope=project kind=AGENTS.md bytes=16 truncated=false
project root=~/work/proj found=true
skill squad scope=user enabled=true plugin=""
skill imagegen scope=system enabled=true plugin=""
skill pr-review scope=plugin enabled=true plugin="github"
plugin github@openai-curated version=26.928.31416 enabled=true display="GitHub"
marketplace openai-bundled type=local source=/opt/test/marketplace
codex plugin list --json --available
== hooks and rules
config hook PermissionRequest matcher="Bash" type=command command="/opt/test/hooks/permission.sh" timeout=10s
config hook PreToolUse matcher="shell" type=command command="/opt/test/hooks/pre-tool.sh" timeout=0s
config hook PreToolUse matcher="shell" type=mcp_tool command="" timeout=0s
config hook SessionStart matcher="" type=command command="/opt/test/hook.sh" timeout=30s
config hook Stop matcher="" type=command command="/opt/test/hooks/stop.sh" timeout=0s
config hook UserPromptSubmit matcher="" type=command command="/opt/test/hooks/prompt.sh" timeout=0s
state /home/test/.codex/hooks.json:session_start:0:0 enabled-set=false enabled=false
state /home/test/.codex/hooks.json:stop:0:0 enabled-set=true enabled=false
hooks.json "test hooks" events=2
parsed Stop groups=1 warnings=0 known=true
hook input event=PreToolUse raw bytes=52
notify type=agent-turn-complete
rule [python3 -m http.server] -> allow (~/.codex/rules/default.rules:1)
rule [ollama show] -> allow (~/.codex/rules/default.rules:2)
rule [git status] -> prompt (~/.codex/rules/default.rules:5)
rule [npm test] -> allow (~/work/proj/.codex/rules/a.rules:1)
issue ~/.codex/rules/default.rules:4 statement is not a prefix_rule call
parsed [rm] -> forbidden
source cli subagent
== doctor
ok    binary-found       ~/npm/bin/codex
ok    home               ~/.codex
ok    version-tested     0.162.1 is tested
ok    config             ~/.codex/config.toml
info  login              access token expired at 2026-09-21T14:13:20Z; codex refreshes it on its next run
warn  profile-conflict   legacy profiles in config.toml: ollama-launch; `codex -p NAME` (profile v2, since 0.131.0) refuses while a legacy [profiles.NAME] table or `profile = "NAME"` exists — move each into <home>/NAME.config.toml and remove the legacy table/selector
info  update-cache       version.json latest 0.156.1 is older than installed 0.162.1 (stale cache, not an update)
```

## Guides

- [The Codex agent](../../docs/codex/agent.md)
- [Codex install and updates](../../docs/codex/install.md)
- [Codex paths and inventory](../../docs/codex/paths.md)
- [Codex config.toml](../../docs/codex/config.md)
- [Codex MCP servers](../../docs/codex/mcp.md)
- [Codex instructions, skills and plugins](../../docs/codex/instructions-skills.md)
- [Codex hooks and rules](../../docs/codex/hooks-rules.md)
- [Codex doctor](../../docs/codex/doctor.md)
- [Feature index](../../docs/reference/features.md)
