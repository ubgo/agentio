package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// exampleToken is a made-up value shaped like a Claude setup-token; it is
// never sent anywhere.
const exampleToken = "sk-ant-" + "oat01-not-a-real-token"

// snippet:start wrap-token
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

// snippet:end wrap-token

// snippet:start token-launch
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

// snippet:end token-launch

// snippet:start personal-values
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

// snippet:end personal-values
