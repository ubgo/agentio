# launch

Building launch commands without running them: a `Target` from a home, the neutral `LaunchRequest` and `Agent.Launch`, the default-home rule, the autonomy guard (`ErrDangerousNotAllowed`), child env edits (`EnvSet`, `EnvUnset`, `EnvSetSecret`, `ApplyEnv`, `HomeEnvChange`, `ScrubEnv`, `NewCommand`), and in `secrets.go` the redacting `Token`, `TokenCredential`, `EnvValue` and `Personal` types.

Everything is Pure: commands are data and are only printed, redacted. Paths use the fixture user home (`~` in the output). The token values are made up; the test checks that none of them reaches the output.

## Run

```sh
cd examples && go run ./launch
```

## Output

```text
new: claude --verbose --model sonnet --add-dir /work/shared-lib -- "fix the failing tests"
new: dir=/work/project
new: env ANTHROPIC_API_KEY=[redacted]
new: env TEAM=platform
new: env CLAUDE_CONFIG_DIR=~/.claude-work
new: env -CLAUDE_CODE_OAUTH_TOKEN
new: warnings=0
resume: claude --verbose --resume f85efbdb-fc2f-1e8e-7748-9f915edc8e2d
default: claude
default: env ANTHROPIC_API_KEY=[redacted]
default: env -CLAUDE_CONFIG_DIR
default: env -CLAUDE_CODE_OAUTH_TOKEN
default: env -TEAM
default: warning version-unknown: installed Claude Code version unknown; flags not checked
autonomy full: refused=true
autonomy full allowed: claude --verbose --dangerously-skip-permissions
autonomy read-only: claude --verbose --permission-mode plan
home rule for default: -CLAUDE_CONFIG_DIR
change: op=set DEBUG=0
change: op=unset -PATH
change: op=set CLAUDE_CONFIG_DIR=~/.claude-work
change: op=unset -ANTHROPIC_API_KEY
change: op=unset -ANTHROPIC_AUTH_TOKEN
change: op=set-secret CLAUDE_CODE_OAUTH_TOKEN=[redacted]
base unchanged: true
secret names: [CLAUDE_CODE_OAUTH_TOKEN]
redacted env: [DEBUG=0 CLAUDE_CONFIG_DIR=~/.claude-work CLAUDE_CODE_OAUTH_TOKEN=[redacted]]
json: {"path":"claude","args":["--print"],"env":["DEBUG=0","CLAUDE_CONFIG_DIR=~/.claude-work","CLAUDE_CODE_OAUTH_TOKEN=[redacted]"]}
raw env holds the token: true
token: %s=[redacted] %v=[redacted] %q=[redacted] %#v=[redacted]
token: json="[redacted]"
token: slog=level=INFO msg=login token=[redacted]
token: reveal matches=true empty=
token launch: claude --verbose -- "fix the failing tests"
token launch: secret env names=[CLAUDE_CODE_OAUTH_TOKEN]
token launch: argv holds token=false
token launch: env TEAM=platform
token launch: env CLAUDE_CODE_OAUTH_TOKEN=[redacted]
token launch: env CLAUDE_CONFIG_DIR=~/.claude-work
token launch: env -ANTHROPIC_API_KEY
personal: json={"email":"[redacted]","org":"","mcp_auth":"[redacted]"}
personal: %v=[redacted] reveal=dev@example.com
personal: decoded email set=true org set=false
```

## Test

```sh
cd examples && go test ./launch/
```

The test runs the same `run` function against the fixture and checks the key output lines.

## Guides

- [Targets and commands](../../docs/core/launch.md)
- [Tokens and redacted values](../../docs/core/secrets.md)
- [Feature index](../../docs/reference/features.md)
