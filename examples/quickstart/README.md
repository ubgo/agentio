# quickstart

Quick start: build a Host, create the Claude Code agent, read the install from disk, list the homes. Also shows reading the Host env, the PATH walk (`LookPath`, `LookPathAll`), `ErrNotInstalled`, the neutral `agentio.Agent`, an agent's `Spec`, and a `Core` built from a made-up Spec.

By default the Host points at the fixture user home `claude/testdata/homes/2.1.325/userhome` and `PATH` points at a fake npm install written to a temp dir (`$TMP` in the output), so nothing on your machine is read and no binary runs. Pass `-os` to print the same report for your real machine through `agentio.HostFromOS` (read-only).

## Run

```sh
cd examples && go run ./quickstart
```

## Output

```text
host: user=user PATH set=true
host: MODE=b EMPTY="" set=true MISSING set=false
install: path=$TMP/bin/claude
install: real=$TMP/lib/node_modules/@anthropic-ai/claude-code/cli.js
install: method=npm-global version=2.1.325
install: shadowed=$TMP/old/bin/claude
home: name=personal default=true dir=~/.claude
home: name=work default=false dir=~/.claude-work
neutral: id=claude name="Claude Code" binary=claude
lookpath: first=$TMP/bin/claude
lookpath: hit[0]=$TMP/bin/claude
lookpath: hit[1]=$TMP/old/bin/claude
empty PATH: agentio: binary not found on PATH: claude
agent: id=claude name="Claude Code"
agent: default home=~/.claude
agent: 2 homes on disk
agent: ParseID("Claude") refused: agentio: invalid agent id: "Claude": only [a-z0-9-] allowed
spec: binary=claude home env=CLAUDE_CONFIG_DIR default home=.claude extra homes=.claude-*
spec: markers=[settings.json projects .credentials.json .claude.json]
spec: resume template=[--resume {id} {args}]
spec: credential env=[CLAUDE_CODE_OAUTH_TOKEN ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN]
spec: process exe="" argv=[/usr/local/bin/claude --resume x] matches=true
spec: process exe="/Applications/Claude.app/Contents/MacOS/claude" argv=[] matches=false
core: invalid spec refused: agentio: invalid spec: agentio: invalid agent id: "Toy": only [a-z0-9-] allowed
core: launch toy --resume abc --model fast hello
core: env TOY_HOME=~/.toy-work
core: headless unsupported=true
core: login toy login
core: check binary-found fail: toy not found on PATH
core: check home warn: ~/.toy-work is not a home (missing or no marker)
```

## Test

```sh
cd examples && go test ./quickstart/
```

The test runs the same `run` function against the fixture and checks the key output lines.

## Guides

- [Your first agent](../../docs/getting-started/first-agent.md)
- [Host: the only door to the machine](../../docs/core/host.md)
- [Agent, Spec and Core](../../docs/core/agent-and-spec.md)
- [Feature index](../../docs/reference/features.md)
