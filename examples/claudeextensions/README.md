# claudeextensions

What extends Claude Code in a home and a project: MCP servers (and the `claude mcp` commands, built but never run), memory files and imports, auto memory, skills, slash commands, workflows, the frontmatter parser, plugins, marketplaces, the blocklist and a plugin manifest (and the `claude plugin` commands), subagent definitions, runs and the `--agents` flag, output styles, and local data (file checkpoints, shell snapshots, IDE locks, debug logs, data usage). The example writes the files the fixture lacks (a plugin, auto memory, styles, workflows, local data) into its temp copy, and reads the recorded `--output-format json` result in `claude/testdata/stream/2.1.325/result.json` for the subagent stats.

Everything runs on a temp copy of the 2.1.325 fixture home in this repository (`claude/testdata/homes/2.1.325/userhome`), through a `Host` built by hand: no real home, no `claude` binary, no network. Paths under the temp user home print as `~`.

## Run it

```sh
cd examples && go run ./claudeextensions
```

Test: `cd examples && go test ./claudeextensions/...` runs the same code and checks the output lines.

## Output

```text
== MCP servers
proj-db       scope=project transport=stdio approval="approved" env=map[DB_PASSWORD:[redacted]] headers=map[]
fixture-http  scope=user    transport=http  approval="" env=map[] headers=map[Authorization:[redacted]]
fixture-stdio scope=user    transport=stdio approval="" env=map[FIXTURE_TOKEN:[redacted]] headers=map[]
ParseMCPConfig: docs transport=http oauth client=docs-cli port=8765

== MCP servers needing auth
fixture-http needs auth since 2026-11-06T21:20:00Z

== MCP commands
add:    claude mcp add-json -s user docs "{\"type\":\"http\",\"url\":\"https://mcp.example.com/docs\"}"
remove: claude mcp remove -s user docs
list:   claude mcp list
reset:  claude mcp reset-project-choices
server with env values refused: agentio: unsupported: claude: mcp add-json with env or headers: claude mcp add-json takes the server only on argv; env values and headers are secrets and never go on argv

== memory files
user         ~/.claude/CLAUDE.md size=97 paths=[] imports=2
user-rule    ~/.claude/rules/style.md size=44 paths=[src/**/*.go] imports=0
project      ~/project/CLAUDE.md size=25 paths=[] imports=0

== auto memory
dir=~/.claude/projects/<EncodePath(project dir)>/memory
auto MEMORY.md size=40
auto build.md size=16

== imports
line 5 raw=~/notes.md
CLAUDE.md imports rules/style.md -> ~/.claude/rules/style.md exists=true
CLAUDE.md imports ~/missing-file.md -> ~/missing-file.md exists=false

== skills
demo-skill scope=user    plugin="" model="opus" tools=[Read Grep] desc="A fixture skill folded over two lines."
proj-skill scope=project plugin="" model="" tools=[] desc="Project scoped fixture skill"
plug-skill scope=plugin  plugin="demo-plugin@claude-plugins-official" model="" tools=[] desc="Skill shipped by a plugin"

== slash commands
/hello scope=user hint="<name>" desc="Say hello"
/deploy scope=plugin hint="" desc="Deploy the app"

== workflows
/release scope=user file=~/.claude/workflows/release.js
/triage scope=project file=~/project/.claude/workflows/triage.js

== frontmatter
keys=[name description argument-hint allowed-tools user-invocable model metadata license]
description="A fixture skill folded over two lines."
allowed-tools=[Read Grep] owner=fixture
user-invocable=true
body starts "# Body is never read by the lister"
Split+ParseYAML: tools kind=scalar text="Read, Grep" List=[Read Grep]
Parse without a block: fields=0 err=<nil>

== plugins
demo-plugin@claude-plugins-official (plugin=demo-plugin marketplace=claude-plugins-official) v1.0.0 scope=user enabled=false
  install=~/.claude/plugins/cache/claude-plugins-official/demo-plugin/1.0.0 data=~/.claude/plugins/data/demo-plugin-claude-plugins-official

== marketplaces and blocklist
claude-plugins-official source=github repo=anthropics/claude-plugins-official declared=false updated=2026-11-13T19:17:32Z
blocklist fetched 2026-11-13T10:00:00Z entries=[]

== plugin manifest
name=demo-plugin version=1.0.0 skills=[./skills] agents=[./agents/helper.md] unknown=[homepage]

== plugin commands
claude plugin install demo-plugin@claude-plugins-official -s project
claude plugin enable demo-plugin@claude-plugins-official
claude plugin disable demo-plugin@claude-plugins-official
claude plugin marketplace add anthropics/claude-plugins-official
claude plugin marketplace update claude-plugins-official

== subagent definitions
reviewer scope=user model=sonnet mode=plan tools=[Read Grep Glob] max turns=12 prompt="You review code."

== subagent runs
agent-a1 type=Explore builtin=true depth=1 mode=manual desc="Find the config loader" unknown=[futureKey]

== --agents flag
--agents {"tester":{"description":"Runs the tests","prompt":"Run go test and report failures.","tools":["Bash","Read"],"model":"sonnet"}}

== output styles
terse    scope=user    keep coding instructions=true desc="Short answers"
Teacher  scope=project keep coding instructions=unset desc="Explains every step"

== file history
session=s1 file hash=abc123 version=2 size=18

== shell snapshots and cleanup
shell=zsh created=2026-11-06T21:20:00Z
last cleanup 2026-11-13T19:30:55Z

== IDE locks and debug logs
ide=VS Code port=41234 pid=5 workspace=[/work/project] transport=ws unknown=[authToken]
debug session=s1 size=10 latest=true

== data usage
foreign entry notes-by-another-tool.txt (21 bytes)
entries=[.credentials.json .last-cleanup .last-update-result.json CLAUDE.md agents cache commands debug file-history history.jsonl ide mcp-needs-auth-cache.json notes-by-another-tool.txt output-styles plugins policy-limits.json projects remote-settings.json remote-settings.json.synced-at rules sessions settings.json shell-snapshots skills stats-cache.json workflows]
model catalog file ~/.claude/cache/model-catalog/models.json
changelog first line "# Changelog"

== subagent stats of a run
spawned=0 completed=0 failed=0 max depth=0
```

## Guides

- [Claude MCP servers](../../docs/claude/mcp.md)
- [Claude memory files](../../docs/claude/memory.md)
- [Claude skills, slash commands and workflows](../../docs/claude/skills.md)
- [Claude plugins and marketplaces](../../docs/claude/plugins.md)
- [Claude subagents](../../docs/claude/subagents.md)
- [Claude output styles](../../docs/claude/output-styles.md)
- [Claude local data](../../docs/claude/local-data.md)
- [Feature index](../../docs/reference/features.md)
