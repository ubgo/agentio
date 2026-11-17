# capmatrix

Which shared capability each built-in agent has (Claude Code, Codex, Gemini CLI, pi), straight from `agentio.Supported`, and the reason each agent gives for a capability it lacks (`agentio.Unsupported`). Nothing is read from disk: support is a property of the agent's type and data.

## Run

```sh
cd examples && go run ./capmatrix
```

## Output

```text
capability          claude  codex   gemini  pi
installer           yes     yes     yes     yes
binary-version      yes     yes     yes     yes
identity            yes     yes     yes     yes
usage               yes     yes     -       -
cached-usage        yes     yes     -       -
subscription        yes     yes     -       -
token-login         yes     -       yes     yes
token-prober        yes     -       -       -
login-commander     yes     yes     -       -
conversations       yes     yes     yes     yes
history-checker     yes     yes     yes     yes
history-mover       yes     -       -       -
conversation-mover  yes     -       -       -
prompt-history      yes     yes     -       -
live-sessions       yes     yes     -       -
process-classifier  yes     yes     yes     yes
turns               yes     yes     -       yes
home-linker         yes     yes     yes     -
instruction-files   yes     yes     yes     yes
mcp-servers         yes     yes     yes     yes
skills              yes     yes     yes     yes
plugins             yes     yes     yes     -
config              yes     yes     yes     yes
models              -       yes     -       -
run-events          yes     yes     -       -
doctor              yes     yes     yes     yes
live-credential     yes     -       -       -
launch-config       yes     yes     -       -
why not:
  gemini usage: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
  pi usage: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
  gemini cached-usage: Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage
  pi cached-usage: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
  gemini subscription: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
  pi subscription: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
  codex token-login: Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
  codex token-prober: Codex has no quota probe: no token login to probe
  gemini token-prober: probing a Gemini key spends quota through a model call that is not modelled
  pi token-prober: pi has no plan or quota to probe
  gemini login-commander: Gemini CLI has no login subcommand; it asks for the auth method in its first interactive run
  pi login-commander: pi logs in with /login inside its interactive UI; there is no login subcommand to build
  codex history-mover: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
  gemini history-mover: Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
  pi history-mover: pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
  codex conversation-mover: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
  gemini conversation-mover: Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
  pi conversation-mover: pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
  gemini prompt-history: the per-project prompt log (tmp/<slug>/logs.json) is recorded only from source, never from a real run
  pi prompt-history: pi keeps no prompt history file; prompts live only in session files
  gemini live-sessions: Gemini CLI writes no session lock or pid file to join a process to a chat
  pi live-sessions: pi writes no live-session registry; only the process table shows a running pi
  gemini turns: Gemini chats rewrite earlier records ($rewindTo, $patch), so an offset-based turn reader would report dropped turns
  pi home-linker: pi has no documented set of files to share between agent dirs
  pi plugins: pi packages and extensions are code loaded into the process, not plugins with a manifest this library models
  claude models: Claude Code keeps no model catalog on disk that has a recorded shape; a static list would rot
  gemini models: Gemini CLI keeps its model catalog in its own defaults, not in a file on disk
  pi models: pi's model catalog (models-store.json) has no recorded shape yet
  gemini run-events: the --output-format stream-json event shape is not recorded from a real run yet
  pi run-events: pi's --mode json event stream is recorded once but not yet modelled as a decoder
  codex live-credential: Codex reads its login from auth.json or the keyring and refreshes it itself; it has no credential-helper setting
  gemini live-credential: Gemini CLI reads its login (oauth_creds.json, the keychain or GEMINI_API_KEY) at start and refreshes OAuth itself; it has no setting that re-reads a key from a caller's command
  pi live-credential: pi can read a key from a !command in auth.json, but when it re-runs that command is not documented or recorded
  gemini launch-config: Gemini CLI has no settings flag and no typed launch options in agentio; pass flags as raw args
  pi launch-config: pi has no settings flag and no typed launch options in agentio; pass flags as raw args
```

## Test

```sh
cd examples && go test ./capmatrix/
```
