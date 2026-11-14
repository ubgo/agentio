package agentcli

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

// redactedText replaces every secret or personal value in output. An empty
// value prints as "" because presence is not a secret and "[redacted]" for
// an empty token would mislead a reader into thinking a login exists.
const redactedText = "[redacted]"

// redact returns redactedText, or "" for an empty value.
func redact(v string) string {
	if v == "" {
		return ""
	}
	return redactedText
}

// redactedJSON is the JSON form of a redacted value.
func redactedJSON(v string) ([]byte, error) { return json.Marshal(redact(v)) }

// unmarshalString decodes a JSON string (or null, as "") into dst. Used by
// the redacting types so vendor structs can decode a credential or email
// straight into a redacting field.
func unmarshalString(b []byte, dst *string) error {
	if string(b) == jsonNull {
		*dst = ""
		return nil
	}
	return json.Unmarshal(b, dst)
}

// jsonNull is the JSON null literal.
const jsonNull = "null"

// Token is a secret credential value (OAuth access token, setup-token, API
// key). Its value never appears in String, GoString, any fmt verb, JSON or
// slog output (invariant 8). The field is unexported so a Token cannot be
// built or read by accident.
//
// Who reads the value: the root env builder (ApplyEnv puts it into
// Command.Env, the one place it must be), and Reveal for the two other
// places a value must leave the type — an HTTP auth header in a usage or
// probe client, and parsing a credential blob a SecretStore returned.
type Token struct{ v string }

// NewToken wraps a secret value. Pure.
func NewToken(v string) Token { return Token{v: v} }

// IsZero reports an empty token. Pure.
func (t Token) IsZero() bool { return t.v == "" }

// Reveal returns the secret. Only for an HTTP auth header or decoding a
// credential blob; never for argv, errors, logs or output. Pure.
func (t Token) Reveal() string { return t.v }

// String redacts. Pure.
func (t Token) String() string { return redact(t.v) }

// GoString redacts %#v. Pure.
func (t Token) GoString() string { return redact(t.v) }

// Format redacts every fmt verb (%s, %v, %+v, %q, %x, …). Pure.
func (t Token) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redact(t.v)) }

// MarshalJSON redacts. Pure.
func (t Token) MarshalJSON() ([]byte, error) { return redactedJSON(t.v) }

// UnmarshalJSON reads a JSON string, so vendor credential structs can hold
// a Token field directly. Pure.
func (t *Token) UnmarshalJSON(b []byte) error { return unmarshalString(b, &t.v) }

// LogValue redacts in slog. Pure.
func (t Token) LogValue() slog.Value { return slog.StringValue(redact(t.v)) }

// TokenCredential is a token login supplied by the caller (agx tokens.yaml),
// as opposed to the home's own login.
type TokenCredential struct {
	Kind  TokenKind
	Token Token
}

// EnvValue is a secret-bearing environment value from a vendor config (MCP
// server env). Same redaction rules as Token; the neutral MCPServer carries
// names only and vendor types carry EnvValue.
type EnvValue struct{ v string }

// NewEnvValue wraps a value. Pure.
func NewEnvValue(v string) EnvValue { return EnvValue{v: v} }

// IsZero reports an empty value. Pure.
func (e EnvValue) IsZero() bool { return e.v == "" }

// Reveal returns the value, for building the vendor's own command. Pure.
func (e EnvValue) Reveal() string { return e.v }

// String redacts. Pure.
func (e EnvValue) String() string { return redact(e.v) }

// GoString redacts. Pure.
func (e EnvValue) GoString() string { return redact(e.v) }

// Format redacts every fmt verb. Pure.
func (e EnvValue) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redact(e.v)) }

// MarshalJSON redacts. Pure.
func (e EnvValue) MarshalJSON() ([]byte, error) { return redactedJSON(e.v) }

// UnmarshalJSON reads a JSON string. Pure.
func (e *EnvValue) UnmarshalJSON(b []byte) error { return unmarshalString(b, &e.v) }

// LogValue redacts. Pure.
func (e EnvValue) LogValue() slog.Value { return slog.StringValue(redact(e.v)) }

// Personal is a personal value (email, display name, org name, account or
// org id). OD-6 adopted: redacted in every output like Token, so a caller's
// no-leak sweep covers identity; a caller that must print it (agx whoami)
// calls Reveal explicitly.
type Personal struct{ v string }

// NewPersonal wraps a value. Pure.
func NewPersonal(v string) Personal { return Personal{v: v} }

// IsZero reports an empty value. Pure.
func (p Personal) IsZero() bool { return p.v == "" }

// Reveal returns the value, for a caller that deliberately prints it. Pure.
func (p Personal) Reveal() string { return p.v }

// String redacts. Pure.
func (p Personal) String() string { return redact(p.v) }

// GoString redacts. Pure.
func (p Personal) GoString() string { return redact(p.v) }

// Format redacts every fmt verb. Pure.
func (p Personal) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redact(p.v)) }

// MarshalJSON redacts. Pure.
func (p Personal) MarshalJSON() ([]byte, error) { return redactedJSON(p.v) }

// UnmarshalJSON reads a JSON string, so vendor structs decode emails
// straight into Personal. Pure.
func (p *Personal) UnmarshalJSON(b []byte) error { return unmarshalString(b, &p.v) }

// LogValue redacts. Pure.
func (p Personal) LogValue() slog.Value { return slog.StringValue(redact(p.v)) }
