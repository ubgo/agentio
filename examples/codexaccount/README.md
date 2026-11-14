# Codex account: identity, keyring, usage, logins

Reads a Codex login without ever printing a token: the identity decoded from `auth.json`, the same login read from a fake OS keyring (and a broken keyring value), live usage from a local `httptest` server standing in for the vendor endpoint (Codex has no base-URL option, so `Host.HTTP` gets a transport that sends the request there), the subscription, cached usage from the newest rollout, and the `codex login` / `codex logout` commands. The fake clock sits before the fixture token's expiry.

## Run it

```sh
cd examples && go run ./codexaccount
```

`-fixture DIR` copies another Codex home instead of the default `../codex/testdata/homes/0.162.1`.

The test (`go test ./codexaccount/`) runs the same code and checks the key lines below.

## Output

```text
logged-in=true api-key=false plan=plus (Plus) workspace=false
email=[redacted] account=[redacted] store=file
access token expires 2026-09-21T14:13:20Z
neutral plan="Plus" billing=subscription
ParsePlan(hc)=enterprise ParsePlan(Education)=edu
keyring service="Codex Auth" account matches KeyringAccount: true
keyring login: logged-in=true store=keyring source=""
keyring error: value is not auth JSON: true
usage plan=plus limit-reached=false source=server
window primary_window length=5h0m0s used=42.5%
window secondary_window length=168h0m0s used=10.0%
a body without windows is ErrNoWindows
subscription plan=plus source=cache until=2026-11-01
server saw a bearer token: true
cached source=transcript observed=2026-10-14T10:00:08Z
cached window primary limit=codex length=168h0m0s used=12%
browser            codex login
device             codex login --device-auth
api-key-stdin      codex login --with-api-key
access-token-stdin codex login --with-access-token
logout             codex logout
```

## Guides

- [Codex auth, identity and usage](../../docs/codex/auth-usage.md)
- [Feature index](../../docs/reference/features.md)
