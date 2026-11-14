package agentio

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// Command is a built process invocation: data, not a running process. The
// caller decides how to run it (syscall.Exec, exec.Cmd, or printing it for
// a dry run with Redacted).
type Command struct {
	// Path is the binary name (Spec.Binary) or an absolute path. Building is
	// Pure, so a launch builder never walks PATH; resolve with Agent.LookPath.
	Path string
	// Args excludes argv[0]. Never holds a token (invariant 8).
	Args []string
	// Env is the full child environment ("K=V"). It is the one place a token
	// lives; print it only through Redacted.
	Env []string
	// Dir is the working directory; "" = the caller's.
	Dir string
	// Warnings are drift notes from building (untested or unknown version).
	Warnings Warnings
	// secretNames are env names whose values came from a Token or EnvValue;
	// Redacted masks them even when the name looks harmless.
	secretNames []string
}

// sensitiveNameParts mark an env name as secret-bearing for Redacted, for
// values that did not come through a Token (profile ExtraEnv).
func sensitiveNameParts() []string {
	return []string{"TOKEN", "KEY", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "AUTH"}
}

// isSensitiveName reports an env name that likely holds a secret.
func isSensitiveName(name string) bool {
	up := strings.ToUpper(name)
	for _, p := range sensitiveNameParts() {
		if strings.Contains(up, p) {
			return true
		}
	}
	return false
}

// Redacted returns a copy safe to print in dry runs and errors: env values
// that came from a Token or EnvValue, or whose name looks secret-bearing,
// are replaced by "[redacted]". Pure.
func (c Command) Redacted() Command {
	out := c
	out.Args = slices.Clone(c.Args)
	out.Warnings = slices.Clone(c.Warnings)
	out.secretNames = slices.Clone(c.secretNames)
	out.Env = make([]string, len(c.Env))
	for i, kv := range c.Env {
		name, val, ok := strings.Cut(kv, envSep)
		if ok && val != "" && (slices.Contains(c.secretNames, name) || isSensitiveName(name)) {
			kv = name + envSep + redactedText
		}
		out.Env[i] = kv
	}
	return out
}

// String renders the argv as a shell-quoted line (path and args only; env
// is never included, so no secret can leak through it). The quoting is
// POSIX sh only, for display and logs: it is not valid PowerShell or
// cmd.exe quoting, so never paste it into a Windows shell or pass it to
// one. To run a Command on any OS, use Path, Args, Env and Dir directly
// (exec.Cmd, syscall.Exec): no shell is involved and no quoting is needed.
// Pure.
func (c Command) String() string {
	parts := make([]string, 0, len(c.Args)+1)
	parts = append(parts, shellQuote(c.Path))
	for _, a := range c.Args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// commandJSON is the JSON shape of a Command; Env is always redacted.
type commandJSON struct {
	Path     string   `json:"path"`
	Args     []string `json:"args"`
	Env      []string `json:"env,omitempty"`
	Dir      string   `json:"dir,omitempty"`
	Warnings Warnings `json:"warnings,omitempty"`
}

// MarshalJSON writes the redacted command, so `--json` output never
// carries a token. Pure.
func (c Command) MarshalJSON() ([]byte, error) {
	r := c.Redacted()
	return marshalJSON(commandJSON{Path: r.Path, Args: r.Args, Env: r.Env, Dir: r.Dir, Warnings: r.Warnings})
}

// SecretEnvNames returns the env names whose values came from a Token. Pure.
func (c Command) SecretEnvNames() []string { return slices.Clone(c.secretNames) }

// shellSafe lists bytes that need no quoting in a shell word.
const shellSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=:,+@%"

// shellQuote quotes s for display.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, shellSafe) == "" {
		return s
	}
	return strconv.Quote(s)
}

// marshalJSON is json.Marshal for a concrete shape (named so callers do not
// pass interface values around).
func marshalJSON[T homeJSON | commandJSON](v T) ([]byte, error) { return json.Marshal(v) }

// EnvOp is what an EnvChange does.
type EnvOp uint8

// EnvOp values.
const (
	// EnvSetOp sets a plain value.
	EnvSetOp EnvOp = iota
	// EnvSetSecretOp sets a value from a Token or EnvValue; redacted in output.
	EnvSetSecretOp
	// EnvUnsetOp removes the variable.
	EnvUnsetOp
)

// envOpKind labels EnvOp in errors.
const envOpKind = "env op"

// envOpNames is the wire spelling of each EnvOp. Pure.
func envOpNames() []string { return []string{"set", "set-secret", "unset"} }

// String returns the wire spelling. Pure.
func (o EnvOp) String() string { return enumString(o, envOpKind, envOpNames()) }

// MarshalText encodes the wire spelling. Pure.
func (o EnvOp) MarshalText() ([]byte, error) { return []byte(o.String()), nil }

// EnvChange is one edit to a child environment. Fields are unexported so a
// secret value set from a Token can only be read back by ApplyEnv (the root
// env builder, invariant 8).
type EnvChange struct {
	op     EnvOp
	name   string
	value  string
	secret Token
}

// EnvSet sets name to a plain value. Pure.
func EnvSet(name, value string) EnvChange { return EnvChange{op: EnvSetOp, name: name, value: value} }

// EnvSetSecret sets name from a Token; the value shows as redacted
// everywhere except the built Command.Env. Pure.
func EnvSetSecret(name string, t Token) EnvChange {
	return EnvChange{op: EnvSetSecretOp, name: name, secret: t}
}

// EnvUnset removes name. Pure.
func EnvUnset(name string) EnvChange { return EnvChange{op: EnvUnsetOp, name: name} }

// Name returns the variable name. Pure.
func (e EnvChange) Name() string { return e.name }

// Op returns what the change does. Pure.
func (e EnvChange) Op() EnvOp { return e.op }

// Value returns a plain value; "" for a secret or unset change. Pure.
func (e EnvChange) Value() string {
	if e.op == EnvSetOp {
		return e.value
	}
	return ""
}

// String renders "NAME=value", "NAME=[redacted]" or "-NAME". Pure.
func (e EnvChange) String() string {
	switch e.op {
	case EnvUnsetOp:
		return "-" + e.name
	case EnvSetSecretOp:
		return e.name + envSep + e.secret.String()
	}
	return e.name + envSep + e.value
}

// envChangeJSON is the JSON shape of an EnvChange (secret redacted).
type envChangeJSON struct {
	Name  string `json:"name"`
	Unset bool   `json:"unset,omitempty"`
	Value string `json:"value,omitempty"`
}

// MarshalJSON writes {name, unset, value} with a secret redacted. Pure.
func (e EnvChange) MarshalJSON() ([]byte, error) {
	j := envChangeJSON{Name: e.name, Unset: e.op == EnvUnsetOp, Value: e.value}
	if e.op == EnvSetSecretOp {
		j.Value = e.secret.String()
	}
	return json.Marshal(j)
}

// ApplyEnv returns a copy of base with changes applied in order: every
// existing entry of a changed name is removed (also duplicates), then a set
// appends one entry. The input is never modified, so a base env can be
// reused across launches. Pure.
// Names compare case-sensitively (the unix rule); ApplyEnvFor applies a
// Host's OS rule.
func ApplyEnv(base []string, changes ...EnvChange) []string {
	return ApplyEnvFor(OSLinux, base, changes...)
}

// ApplyEnvFor is ApplyEnv under goos's naming rule: on Windows a change to
// PATH also removes an inherited "Path=" entry, so the child never sees two
// spellings of one variable. Pure.
func ApplyEnvFor(goos string, base []string, changes ...EnvChange) []string {
	out := slices.Clone(base)
	for _, c := range changes {
		out = slices.DeleteFunc(out, func(kv string) bool {
			name, _, ok := strings.Cut(kv, envSep)
			return ok && envNameEqual(goos, name, c.name)
		})
		switch c.op {
		case EnvSetOp:
			out = append(out, c.name+envSep+c.value)
		case EnvSetSecretOp:
			out = append(out, c.name+envSep+c.secret.Reveal())
		}
	}
	return out
}

// HomeEnvChange is the home rule (invariant 2): for the default home the
// variable is removed, never set to the default path (Claude Code would
// switch keychain names and look logged out); for any other home it is set
// to the home's dir. ok is false when the agent has no home variable. Pure.
func HomeEnvChange(homeEnv string, h Home) (EnvChange, bool) {
	if homeEnv == "" {
		return EnvChange{}, false
	}
	if h.IsDefault() {
		return EnvUnset(homeEnv), true
	}
	return EnvSet(homeEnv, h.Dir()), true
}

// ScrubEnv returns unset changes for names (every launch removes the
// vendor's inherited credential variables, invariant 9). Pure.
func ScrubEnv(names []string) []EnvChange {
	out := make([]EnvChange, 0, len(names))
	for _, n := range names {
		out = append(out, EnvUnset(n))
	}
	return out
}

// secretNamesOf lists names set from secrets in changes.
func secretNamesOf(changes []EnvChange) []string {
	var out []string
	for _, c := range changes {
		if c.op == EnvSetSecretOp && !slices.Contains(out, c.name) {
			out = append(out, c.name)
		}
	}
	return out
}

// NewCommand builds a Command whose env is base with changes applied, and
// records which names hold secrets so Redacted masks them. Pure.
func NewCommand(path string, args []string, base []string, dir string, changes ...EnvChange) Command {
	return Command{
		Path:        path,
		Args:        slices.Clone(args),
		Env:         ApplyEnv(base, changes...),
		Dir:         dir,
		secretNames: secretNamesOf(changes),
	}
}
