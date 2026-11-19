# Claude subagents

Subagents are specialised agents Claude Code can hand a task to. Definitions live in `agents/*.md` (user, project and plugin dirs) or arrive on the command line with `--agents`; each session records the subagents it actually started. The `claude` module lists definitions, lists the runs of one session, and encodes the `--agents` flag. Use it to show what helpers a project defines, or to give a headless run its own subagents without files on disk.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent, a home and optionally a project (see [The Claude agent](agent.md)).
- Runs are read from `<home>/projects/<EncodePath(dir)>/<session>/subagents/agent-<id>.meta.json`.

The snippets come from [`examples/claudeextensions`](../../examples/claudeextensions/), which adds one subagent run to a fixture session.

## Step 1 — List subagent definitions

`a.SubagentDefs(h, &p)` returns `[]SubagentDef`: `Name`, `Description`, `Prompt` (the body), `Scope`, `File`, `Tools`, `DisallowedTools`, `Skills`, `Model`, `PermissionMode`, `MaxTurns`, `Effort`, `Isolation`, `Color`, `Background`, `Memory`, `OmitClaudeMD`, `MCPServers`, `Hooks`, other keys in `Unknown`, and `Err` for a frontmatter that did not parse.

<!-- snippet: examples/claudeextensions/main.go#subagentdefs -->
```go
// showSubagentDefs lists agents/*.md definitions.
func showSubagentDefs(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("subagent definitions")
	defs, err := a.SubagentDefs(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, d := range defs {
		turns := "unset"
		if d.MaxTurns != nil { // nil: the file sets no limit
			turns = fmt.Sprint(*d.MaxTurns)
		}
		p.line(fmt.Sprintf("%s scope=%s model=%s mode=%s tools=%v max turns=%s prompt=%q", d.Name, d.Scope, d.Model, d.PermissionMode, d.Tools, turns, d.Prompt))
	}
	return nil
}
```

## Step 2 — List subagent runs

`a.SubagentRuns(h, dir, sessionID)` lists the subagents one session (started in `dir`) launched. A `SubagentRun` has `ID`, `AgentType`, `Name`, `Description`, `Model`, `IsBuiltIn`, `SpawnDepth`, `PermissionMode`, `Transcript`, `HasMeta` and `Unknown`. A transcript `agent-<id>.jsonl` without a `.meta.json` (older Claude Code wrote none) is listed too, with only `ID` and `Transcript` set and `HasMeta` false.

<!-- snippet: examples/claudeextensions/main.go#subagentruns -->
```go
// showSubagentRuns lists the subagents one session started.
func showSubagentRuns(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("subagent runs")
	runs, err := a.SubagentRuns(a.DefaultHome(), historyFolder, sessionID)
	if err != nil {
		return err
	}
	for _, r := range runs {
		p.line(fmt.Sprintf("%s type=%s builtin=%t depth=%d mode=%s desc=%q unknown=%v", r.ID, r.AgentType, r.IsBuiltIn, r.SpawnDepth, r.PermissionMode, r.Description, r.Unknown.Keys()))
	}
	return nil
}
```

A headless run reports a tally instead: `StreamResult.SubagentStats` (`SubagentStats`: spawned, requested by mode, completed, failed, killed, refused, max depth) on the result line of `--output-format json` or `stream-json`.

<!-- snippet: examples/claudeextensions/main.go#subagentstats -->
```go
// showSubagentStats reads the subagent tally of a finished headless run.
func showSubagentStats(p printer, result string) error {
	p.section("subagent stats of a run")
	f, err := os.Open(result)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // read-only; a close error changes nothing
	res, err := claude.DecodeJSONResult(f)
	if err != nil {
		return err
	}
	if s := res.SubagentStats; s != nil {
		p.line(fmt.Sprintf("spawned=%d completed=%d failed=%d max depth=%d", s.Spawned, s.Completed, s.Failed, s.MaxDepth))
	}
	return nil
}
```

## Step 3 — Pass subagents on the command line

`claude.EncodeAgentsFlag(defs)` renders a `map[string]SubagentDef` as the `--agents` JSON. You rarely call it directly: setting `LaunchOptions.Agents` makes `Launch` emit the flag for you, checked against the installed version (see [Claude launch](launch.md)).

<!-- snippet: examples/claudeextensions/main.go#agentsflag -->
```go
// showAgentsFlag renders subagent definitions as the --agents JSON, for a
// session that should get them without files on disk.
func showAgentsFlag(p printer, _ *claude.Agent, _ claude.Project) error {
	p.section("--agents flag")
	flagValue, err := claude.EncodeAgentsFlag(map[string]claude.SubagentDef{
		"tester": {Description: "Runs the tests", Prompt: "Run go test and report failures.", Tools: []string{"Bash", "Read"}, Model: "sonnet"},
	})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("--agents %s", flagValue))
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudeextensions
```

The sections this page covers:

```text
== subagent definitions
reviewer scope=user model=sonnet mode=plan tools=[Read Grep Glob] max turns=12 prompt="You review code."

== subagent runs
agent-a1 type=Explore builtin=true depth=1 mode=manual desc="Find the config loader" unknown=[futureKey]

== --agents flag
--agents {"tester":{"description":"Runs the tests","prompt":"Run go test and report failures.","tools":["Bash","Read"],"model":"sonnet"}}

== subagent stats of a run
spawned=0 completed=0 failed=0 max depth=0
```

## Pitfalls

- The definition name is the frontmatter `name`, falling back to the file name without `.md`.
- `Tools` accepts both `Read, Grep` and a YAML list in the file; the lister splits either into a slice.
- `MaxTurns` is a pointer: nil means no limit set, not zero.
- A run's `permissionMode: "default"` decodes to `claude.ModeManual`, as everywhere in the module.
- `SubagentRuns` needs the folder the session started in, because runs sit under that folder's history dir; after a history move pass the new folder.
- `SubagentStats.ByType` was empty in the recorded run, so its values stay raw JSON per key.
- `--agents` JSON travels on argv: never put a secret in a subagent prompt or its MCP server env.

## Related

- [Claude skills, slash commands and workflows](skills.md), [Claude plugins](plugins.md), [Claude launch](launch.md), [Claude stream](stream.md), [Claude conversations](conversations.md)
- Example: [`examples/claudeextensions`](../../examples/claudeextensions/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
