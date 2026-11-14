# Gemini CLI

Walks the disk-only API of the `gemini` module on a temp copy of the checked-in 0.68.0 fixture home (`gemini/testdata/homes/0.68.0`): the homes and the `GEMINI_CLI_HOME` rule, the install read from a fake npm package on `PATH`, the settings layers with every key kept, conversations per project and the resume command, instruction files, MCP servers, skills and extensions, the login, the launch commands and the capability matrix. Nothing reads the real machine, runs a binary or touches the network.

## Run it

```sh
cd examples && go run ./gemini
```

`host.go` builds the fake machine (it fills the fixture's `@PROJECT@` placeholders with the temp project path, JSON-encoded inside JSON files); `main.go` holds one function per guide step. The test (`go test ./gemini/`) runs the same code and checks the key lines below.

## Output

The system layer paths are the Linux ones; macOS and Windows print their own.

```text
== homes
default: ~/.gemini name=personal
work: ~/gemini-work/.gemini name=gemini-work launches with GEMINI_CLI_HOME=~/gemini-work
== install
install: gemini method=npm version=0.68.0
tested range: 0.68.0..0.68.0
== settings
layer managed  exists=false /etc/gemini-cli/settings.json
layer project  exists=true  <project>/.gemini/settings.json
layer user     exists=true  ~/.gemini/settings.json
layer managed  exists=false /etc/gemini-cli/system-defaults.json
user auth=oauth-personal approval=auto_edit unknown keys=[skills ui]
effective model=gemini-2.5-flash (workspace wins) approval=auto_edit -> autonomy accept-edits
== conversations
project slug=demo (Slugify of the base name: my-project)
  22222222  "Changelog entry for the parser fix" legacy=false updated 2026-11-02T09:31Z
  11111111  "fix the flaky test in parser_test.go" legacy=false updated 2026-11-01T10:05Z
  44444444  "explain the build script"           legacy=true  updated 2026-10-20T12:10Z
resume: gemini --resume 22222222-2222-4222-8222-222222222222 (in <project>)
== instructions and mcp
instructions user    ~/.gemini/GEMINI.md
instructions project <project>/GEMINI.md
instructions project <project>/sub/GEMINI.md
mcp disabled-one    stdio scope=user    enabled=false env=[]
mcp ext-server      stdio scope=plugin  enabled=-     env=[]
mcp fixture-http    http  scope=user    enabled=-     env=[]
mcp fixture-stdio   stdio scope=user    enabled=-     env=[FIXTURE_TOKEN]
mcp project-server  sse   scope=project enabled=-     env=[]
skill old-skill  scope=user    enabled=false
skill review     scope=user    enabled=true
skill ext-skill  scope=plugin  enabled=true extension=demo-ext
== login
auth=oauth-personal logged-in=true credential=file email=[redacted] expires=2026-11-14
== launch
launch: gemini --prompt-interactive "explain the parser"
launch: gemini --approval-mode plan --model gemini-2.5-flash
launch: gemini --approval-mode yolo --output-format stream-json --include-directories ../lib --prompt "fix the failing tests"
token launch sets GEMINI_API_KEY (value redacted in output: true)
full without AllowDangerous: agentio: full autonomy needs Target.AllowDangerous
== capabilities
supported: [installer binary-version identity token-login conversations history-checker process-classifier home-linker instruction-files mcp-servers skills plugins config doctor]
usage: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
login-commander: Gemini CLI has no login subcommand; it asks for the auth method in its first interactive run
run-events: the --output-format stream-json event shape is not recorded from a real run yet
```
