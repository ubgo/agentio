# claudesettings

The read-only side of the `claude` module for one home and one project: creating the agent and picking a home, the install read from disk (a fake native install is added to the copy), paths, every settings layer and their merge, managed and remote settings, the environment in effect, the global state file `.claude.json`, permissions and policy limits.

Everything runs on a temp copy of the 2.1.325 fixture home in this repository (`claude/testdata/homes/2.1.325/userhome`), through a `Host` built by hand: no real home, no `claude` binary, no network. Paths under the temp user home print as `~`.

## Run it

```sh
cd examples && go run ./claudesettings
```

Test: `cd examples && go test ./claudesettings/...` runs the same code and checks the output lines.

## Output

```text
== agent
id=claude name="Claude Code" binary=claude home env=CLAUDE_CONFIG_DIR
spec binary=claude home markers=[settings.json projects .credentials.json .claude.json]
home ~/.claude name=personal default=true
home ~/.claude-work name=work default=false
HomeAt claude:~/.claude-work is a home=true markers=[settings.json projects .claude.json] (of [settings.json projects .credentials.json .claude.json])
HomeFromEnv: CLAUDE_CONFIG_DIR is absent, so the default home is in use
HomeOf refuses a Codex home: agentio: home belongs to another agent: codex home passed to claude
project dir=~/project repo root=""
neutral id=claude default home=claude:~/.claude agent of home=claude

== install
path=~/.local/bin/claude real=~/.local/share/claude/versions/2.1.325 method=native version=2.1.325
raw method="native" auto updates=false protected for native=true
last update 2026-11-12T22:39:35Z: 2.1.323 -> 2.1.325 outcome=success
cached changelog: 0 bytes
ParseVersion -> 2.1.325
update command: claude update
install command: claude install stable

== paths
global config=~/.claude.json settings=~/.claude/settings.json
projects=~/.claude/projects sessions=~/.claude/sessions
plugins installed=~/.claude/plugins/installed_plugins.json
project settings=~/project/.claude/settings.json local=~/project/.claude/settings.local.json mcp=~/project/.mcp.json
project memory files=[~/project/CLAUDE.md ~/project/.claude/CLAUDE.md ~/project/CLAUDE.local.md ~/project/AGENTS.md]
managed settings (fixture host)=""
managed settings (linux host)=/etc/claude-code/managed-settings.json
EncodePath("/work/proj12") = -work-proj12
HistoryDir = ~/.claude/projects/-work-proj12

== settings layers
user present=true file=~/.claude/settings.json model="opus" unknown keys=[]
project present=true file=~/project/.claude/settings.json model="" unknown keys=[futureKey]
local present=true file=~/project/.claude/settings.local.json model="sonnet" unknown keys=[]
ParseSettings: theme="dark" model kept in Unknown=[model] warnings=1

== effective settings
layer local present=true
layer project present=true
layer user present=true
model="sonnet" from local
default mode=bypassPermissions allow=[Bash(go test *)]
Stop hook groups=2 (user + project)
env=map[FIXTURE_FLAG:1] unknown keys=[futureKey]

== managed and remote settings
remote present=true synced=2026-11-13T19:38:51Z

== environment in effect
FIXTURE_FLAG=[redacted] from=local secret=false
known variables=56, secret ones=[CLAUDE_CODE_OAUTH_TOKEN ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN ANTHROPIC_FOUNDRY_API_KEY ANTHROPIC_FOUNDRY_AUTH_TOKEN AWS_BEARER_TOKEN_BEDROCK ANTHROPIC_AWS_API_KEY CLAUDE_CODE_MESSAGING_TOKEN]

== global config
path=~/.claude.json install method="native" startups=1072
projects=3 user MCP servers=2 unmodelled keys=58
account email=[redacted] org type=claude_max billing=stripe_subscription
ParseGlobalConfig: first start version=2.1.296

== project state
dir=/work/proj2 trusted=false last session cost=$6.07 models=2
/never/used: no entry
cached usage fetched 2026-10-28T08:38:37Z, 3787 bytes

== config backups
backup ~/.claude/backups/.claude.json.backup.1794000000000

== permissions
default mode=bypassPermissions set by user
project allow tool=Bash specifier="go test *"

== parse rules
"Bash(git *)" -> tool=Bash specifier="git *"
"Read" -> tool=Read specifier=""
"mcp__github__get_issue" -> tool=mcp__github__get_issue specifier=""
"Bash(git *": claude: invalid permission rule "Bash(git *"

== policy limits
restriction allow_connector_file_downloads allowed=false
restriction allow_quick_web_setup allowed=false
restriction allow_remote_control allowed=false
restriction enforce_web_search_mcp_isolation allowed=false
defaults=1
stamp kind=org identity=[redacted] confirmed=2026-11-01T10:00:00Z
```

## Guides

- [The Claude agent](../../docs/claude/agent.md)
- [Claude install and updates](../../docs/claude/install.md)
- [Claude paths](../../docs/claude/paths.md)
- [Claude settings](../../docs/claude/settings.md)
- [Claude global state (.claude.json)](../../docs/claude/global-config.md)
- [Claude permissions](../../docs/claude/permissions.md)
- [Claude policy limits](../../docs/claude/policy.md)
- [Feature index](../../docs/reference/features.md)
