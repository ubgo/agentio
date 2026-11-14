# claudeaccount

Reads a Claude Code login and account without touching the real machine: the credential of each home from the file store and a fake keychain (`Agent.CredentialContext`, `Credential.Expired`, `KeychainServices`), token kind detection (`TokenKinds`, `DetectTokenKind`, `TokenLifetime`, `ErrUnusableToken`), the token environment (`TokenEnv`), login commands (`LoginCommand`, `LogoutCommand`, `SetupTokenCommand`), the identity (`Agent.Identity`), the OAuth account record (`Agent.Account`, `PlanLabel`, `ParseProfile`, `ErrNoOrganization`) and `claude auth status` through the Runner door (`Runner.AuthStatus`, `ParseAuthStatus`).

Reads the checked-in fixture home `claude/testdata/homes/2.1.325/userhome`. The keychain, the process runner and the clock are in-memory fakes; the work home's keychain login holds a placeholder token. No token value is printed: tokens are `agentio.Token` and print as `[redacted]`, personal values are `agentio.Personal`.

## Run

```sh
cd examples && go run ./claudeaccount
```

`go test ./claudeaccount/` runs the same code and checks key output lines.

## Output

```text
personal: login in file (~/.claude/.credentials.json)
  expires 2026-11-13T20:56:01Z, expired now: false
  plan max / default_claude_max_20x, 6 scopes, token [redacted]
  keychain names tried: 2
work: login in keychain (Claude Code-credentials-<hash>)
  expires 2026-11-13T18:00:00Z, expired now: false
  plan pro / default_claude_pro, 2 scopes, token [redacted]
  keychain names tried: 1
token logins: [oauth-token api-key api-key-helper]
  sk-ant-oat01-E oauth-token, valid for 8760h0m0s
  sk-ant-api03-E api-key, valid for until revoked
  sk-ant-ort01-E refused: claude: this token cannot run Claude: it is an OAuth refresh token; use the token `claude setup-token` prints
  sk-ant-admin01 refused: claude: this token cannot run Claude: it is an Admin API key
  not-a-claude-t unknown prefix
token env for oauth-token: CLAUDE_CODE_OAUTH_TOKEN=[redacted]
token env for api-key: ANTHROPIC_API_KEY=[redacted]
command: claude auth login --claudeai                       CLAUDE_CONFIG_DIR set: false
command: claude auth login --console --email user@example.com CLAUDE_CONFIG_DIR set: true
command: claude auth logout                                 CLAUDE_CONFIG_DIR set: true
command: claude setup-token                                 CLAUDE_CONFIG_DIR set: false
identity: logged in true, plan "Max (20x)", billing subscription, credential file
  email [redacted], org [redacted]
  summary: file, expires 2026-11-13T20:56:01Z
account: org type claude_max, billing stripe_subscription, plan Max (20x)
profile: plan Max (20x), status active, billing stripe_subscription, since 2025-01-02
profile without organization: claude: profile response has no organization
auth status: logged in true via claude.ai (firstParty), plan tier max_20x, email [redacted]
parsed offline: subscription max, unknown keys 0
```

## Guides

- [Claude credentials and tokens](../../docs/claude/credential.md)
- [Claude account and identity](../../docs/claude/account.md)
- [Feature index](../../docs/reference/features.md)
