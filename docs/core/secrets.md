# Tokens and redacted values

agentio carries every secret and every piece of personal data in a type that refuses to print it. `Token` holds a credential (OAuth access token, setup-token, API key), `EnvValue` a secret-bearing environment value from a vendor config (an MCP server's env), and `Personal` an email, display name, org name or account id. All three print `[redacted]` through every `fmt` verb, JSON and `slog`, so a log line, an error or a `--json` dump cannot leak them by accident. You want this page whenever you pass a token to a launch or show account data.

## Before you start

- Module: `github.com/ubgo/agentio`; the token launch also uses `claude`.
- Nothing on disk. The values below are made up.

## Step 1 — Wrap a secret in a Token

`agentio.NewToken(value)` wraps a secret. Its only field is unexported, so a Token cannot be read by accident; `Reveal()` is the one deliberate way out, meant for an HTTP auth header or for decoding a credential blob a `SecretStore` returned, never for argv, errors, logs or output. `LogValue` makes `slog` print it redacted.

<!-- snippet: examples/launch/secrets.go#wrap-token -->
```go
// wrapToken shows that a Token never prints its value: not through any fmt
// verb, JSON or slog. Reveal is the one deliberate way out, for an HTTP auth
// header or decoding a credential blob. Pure.
func wrapToken(p printer) error {
	tok := agentio.NewToken(exampleToken)
	p.line(fmt.Sprintf("token: %%s=%s %%v=%v %%q=%q %%#v=%#v", tok, tok, tok, tok))
	js, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	p.line("token: json=" + string(js))

	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{
		// Drop the time so the line is stable.
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
	log.Info("login", "token", tok)
	p.line("token: slog=" + strings.TrimSpace(logs.String()))

	p.line(fmt.Sprintf("token: reveal matches=%t empty=%q", tok.Reveal() == exampleToken, agentio.NewToken("")))
	return nil
}
```

A `Token` also decodes from a JSON string (`UnmarshalJSON`), so a vendor credential struct can hold one directly. `IsZero()` reports an empty token.

## Step 2 — Put a token in a child env

To launch an agent with a token instead of the home's own login, pass an `agentio.TokenCredential` (the `TokenKind` plus the `Token`) in `LaunchRequest.Credential`. The agent maps it to its variable (`CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY` for Claude) with `EnvSetSecret`, scrubs its other credential variables first, and the value ends up only in `Command.Env`:

<!-- snippet: examples/launch/secrets.go#token-launch -->
```go
// tokenLaunch launches with a caller-supplied token instead of the home's
// own login. The token lands only in Command.Env (never argv); the
// vendor's other credential variables are scrubbed first, so the API key
// in the base env cannot win. Pure.
func tokenLaunch(p printer, a agentio.Agent, t agentio.Target) error {
	cred := &agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken(exampleToken)}
	cmd, err := a.Launch(t, agentio.LaunchRequest{Prompt: promptText, Credential: cred})
	if err != nil {
		return err
	}
	p.line("token launch: " + cmd.String())
	p.line(fmt.Sprintf("token launch: secret env names=%v", cmd.SecretEnvNames()))
	p.line(fmt.Sprintf("token launch: argv holds token=%t", slices.Contains(cmd.Args, exampleToken)))
	printEnv(p, "token launch", cmd)
	return nil
}
```

When you build an env yourself, `agentio.EnvSetSecret(name, token)` is the same building block: the value shows as `[redacted]` everywhere except the `Env` that `ApplyEnv` or `NewCommand` builds ([Targets and commands](launch.md#step-4--change-the-child-environment)).

## Step 3 — Redact personal values

`EnvValue` (`NewEnvValue`) and `Personal` (`NewPersonal`) follow the same rules as `Token`. Vendor structs decode emails and org names straight into `Personal` fields, and the neutral `MCPServer` carries env names only while vendor MCP types carry `EnvValue`. A program that must show an email (a `whoami` command) calls `Reveal()` explicitly.

<!-- snippet: examples/launch/secrets.go#personal-values -->
```go
// account is a caller struct holding personal and secret-bearing values;
// encoding it never leaks them.
type account struct {
	Email   agentio.Personal `json:"email"`
	Org     agentio.Personal `json:"org"`
	MCPAuth agentio.EnvValue `json:"mcp_auth"`
}

// personalValues shows EnvValue and Personal: the same redaction as Token,
// for MCP server env values and for emails, names and account ids. An empty
// value prints as "" so a reader never mistakes it for a present one. Pure.
func personalValues(p printer) error {
	acc := account{
		Email:   agentio.NewPersonal("dev@example.com"),
		MCPAuth: agentio.NewEnvValue("Bearer example"),
	}
	js, err := json.Marshal(acc)
	if err != nil {
		return err
	}
	p.line("personal: json=" + string(js))
	p.line(fmt.Sprintf("personal: %%v=%v reveal=%s", acc.Email, acc.Email.Reveal()))

	var decoded account
	if err := json.Unmarshal([]byte(`{"email":"ops@example.com","org":null}`), &decoded); err != nil {
		return err
	}
	p.line(fmt.Sprintf("personal: decoded email set=%t org set=%t", !decoded.Email.IsZero(), !decoded.Org.IsZero()))
	return nil
}
```

## Try it

```sh
cd examples && go run ./launch
```

The lines for this page:

```text
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

The example's test also checks that neither the token nor the shell's API key appears anywhere in the output.

## Pitfalls

- **An empty value prints as empty, not `[redacted]`.** Presence is not a secret, and `[redacted]` for an empty token would suggest a login exists. Check `IsZero()` rather than the printed text.
- **Every verb prints the same bare text.** `Format` ignores the verb, so `%q` gives `[redacted]` without quotes and `%x` does not hex-encode anything.
- **`Reveal()` is a promise you make.** Never pass its result to argv, an error message, a log or a file; the library itself calls it only for an HTTP auth header and when parsing a credential blob.
- **A token never travels on argv.** Builders put it in `Command.Env` only; if you need to pass one to a child process, use `EnvSetSecret`.
- **Never refresh an OAuth token yourself.** The library never does: refreshing rotates the refresh token and logs the vendor CLI out. An expired login is reported as `ErrExpired`; one interactive run of the vendor CLI refreshes it ([Errors](errors.md)).
- **`Command.Env` is the one place with the real value.** Print commands with `Redacted()`, `String()` or JSON.
- **A token kind the agent does not take is refused.** Claude accepts `oauth-token` and `api-key`; any other `TokenKind` returns `ErrUnsupported` from `Launch`.

## Related

- [Targets and commands](launch.md)
- [Logins and tokens](../capabilities/logins.md)
- [Claude credentials](../claude/credential.md)
- [Errors](errors.md)
- Example: [`examples/launch`](../../examples/launch/)
- [Feature index](../reference/features.md)
