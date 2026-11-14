# homes

Homes: the default home and its accessors, listing homes on disk by marker files, the home `CLAUDE_CONFIG_DIR` selects, `IsHome`, opening a home by folder (`HomeAt`, `Core.HomeAt`), and the owner check (`CheckOwner`, `ErrForeignHome`).

Reads the fixture user home `claude/testdata/homes/2.1.325/userhome` (`~` in the output); nothing on your machine is touched.

## Run

```sh
cd examples && go run ./homes
```

## Output

```text
default: agent=claude name=personal default=true
default: dir=~/.claude
default: path=~/.claude/settings.json
default: name is DefaultHomeName: true
zero home: IsZero=true
homes: claude:~/.claude name=personal default=true
homes: claude:~/.claude-work name=work default=false
from env: ok=true name=work dir=~/.claude-work
is home: .claude-work=true
is home: project=false
is home: missing=false
home at .claude-work: name=work default=false dir=~/.claude-work
home at /srv/agents/.claude-ci: name=ci default=false dir=/srv/agents/.claude-ci
home at .claude: name=personal default=true dir=~/.claude
core home at .toy-staging: toy:~/.toy-staging name=staging
check owner: foreign=true err=agentio: home belongs to another agent: codex home passed to claude
check owner: own home err=<nil>
```

## Test

```sh
cd examples && go test ./homes/
```

The test runs the same `run` function against the fixture and checks the key output lines.

## Guides

- [Homes](../../docs/core/homes.md)
- [Feature index](../../docs/reference/features.md)
