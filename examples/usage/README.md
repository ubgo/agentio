# Usage example

Reads plan usage through the shared accessors: cached usage from disk (`agentio.CachedUsageOf`), live usage (`agentio.UsageOf`) and the subscription (`agentio.SubscriptionOf`); builds every login and logout command (`agentio.LoginCommanderOf`); maps a token onto a launch without putting it on argv (`agentio.TokenLoginOf`); and probes a token's usage (`agentio.TokenProberOf`).

It starts an in-process fake vendor API serving the recorded response shapes, points the Claude agent at it with `claude.New(h).WithBaseURL(srv.URL)`, and gives the Host an HTTP client that routes every request to that server, so nothing leaves the process. The homes are temp copies of the fixtures; the tokens are sample shapes, never real values.

## Run

```sh
cd examples && go run ./usage
```

Test:

```sh
cd examples && go test ./usage/...
```

## Output

```text
== cached usage ==
claude personal: max 6% source=cache age=387h21m0s
  Current session  short   6.0% resets 2026-10-28T12:59:59Z
  All models       week    2.0% resets 2026-11-04T05:59:59Z
  Fable            week    0.0% resets 2026-11-04T06:00:00Z
claude work: no cached usage
codex personal: max 12% source=transcript age=722h0m0s
  7d               week   12.0% resets 2026-09-21T14:13:20Z
== live usage ==
claude: max 42% billing=subscription source=server
  Current session  short  42.0% resets 2026-11-13T15:00:00Z
  All models       week   18.0% resets 2026-11-18T09:00:00Z
codex: login expired; run the vendor CLI once to refresh it
== subscription ==
claude: plan="Max (20x)" status="active" billing-type="stripe_subscription"
codex: plan="Plus" status="" billing-type=""
== login commands ==
claude browser             claude auth login --claudeai
claude device              unsupported
claude api-key-stdin       unsupported
claude access-token-stdin  unsupported
claude logout              claude auth logout
codex browser             codex login
codex device              codex login --device-auth
codex api-key-stdin       codex login --with-api-key
codex access-token-stdin  codex login --with-access-token
codex logout              codex logout
== token login ==
claude kinds: [oauth-token api-key api-key-helper]
  sk-ant-oat01-... kind=oauth-token usable=true
  sk-ant-api03-... kind=api-key usable=true
  sk-ant-ort01-... kind=unknown usable=false
  token env: [CLAUDE_CODE_OAUTH_TOKEN=[redacted]]
  launch: claude -- hello
  secret env names: [CLAUDE_CODE_OAUTH_TOKEN]
  token on argv: false
  setup: claude setup-token
codex: no token login (Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin))
== token probe ==
claude oauth token: max 37% source=probe
  Current session  short  37.0% resets 2026-11-13T17:00:00Z
  All models       week   21.0% resets 2026-11-13T17:00:00Z
claude api key: billed per token, not probed
```

## Guides

- [Usage and subscription](../../docs/capabilities/usage.md)
- [Logins and tokens](../../docs/capabilities/logins.md)
- [Feature index](../../docs/reference/features.md)
