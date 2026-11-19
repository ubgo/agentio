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
	// credentialNames are names the build scrubbed (a credential variable,
	// Spec.CredentialEnv or a token's scrub list) and then set again with a
	// plain value (a caller's ExtraEnv key): Redacted masks them.
	credentialNames []string
	// plainNames are names the build set with a plain value (EnvSet) and
	// never scrubbed: their source chose to show them, so Redacted prints
	// them whatever the name (CLAUDE_CODE_API_KEY_HELPER_TTL_MS).
	plainNames []string
	// homeNames are names the home rule set (HomeEnvChange: the home
	// variable of a non-default home), the only env ShellLine prints.
	homeNames []string
}

// sensitiveNameParts mark an env name as secret-bearing for Redacted, for
// values inherited unchanged from the base env: their source is unknown,
// so the name is the only guard (a user's GITHUB_TOKEN). Values the build
// set are judged by their source instead (Command.Redacted).
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

// Redacted returns a copy safe to print in dry runs and errors. Each env
// value is judged by its source, replaced by "[redacted]" when:
//   - it came from a Token or EnvValue (EnvSetSecret);
//   - the build scrubbed its name as a credential and then set it again
//     with a plain value (a caller's ExtraEnv API key);
//   - it was inherited unchanged from the base env and its name looks
//     secret-bearing (sensitiveNameParts): the source is unknown there.
//
// A plain value the build set (EnvSet) on a name it did not scrub is
// shown as written, whatever the name: its source chose plain. Pure.
func (c Command) Redacted() Command {
	out := c
	out.Args = slices.Clone(c.Args)
	out.Warnings = slices.Clone(c.Warnings)
	out.secretNames = slices.Clone(c.secretNames)
	out.credentialNames = slices.Clone(c.credentialNames)
	out.plainNames = slices.Clone(c.plainNames)
	out.homeNames = slices.Clone(c.homeNames)
	out.Env = make([]string, len(c.Env))
	for i, kv := range c.Env {
		name, val, ok := strings.Cut(kv, envSep)
		if ok && val != "" && c.redacts(name) {
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

// shellLineDirSep joins the cd prefix to the command in ShellLine.
const shellLineDirSep = " && "

// shellLineCd starts the directory prefix of ShellLine.
const shellLineCd = "cd "

// ShellLine renders the line a person types to run the command, for
// printing (agentop's "resume with" hint): "cd <Dir> && " when Dir is
// set, then the home variable the build set for a non-default home
// ("CLAUDE_CONFIG_DIR=<home>"; nothing for the default home, whose rule
// removes the variable), then the path and args as String quotes them.
// The home variable is the only env entry it prints: a token, an API key
// or any other env value never appears, whatever its name. The quoting is
// POSIX sh only, like String. Pure.
func (c Command) ShellLine() string {
	var parts []string
	if c.Dir != "" {
		parts = append(parts, shellLineCd+shellQuote(c.Dir)+shellLineDirSep)
	}
	for _, name := range c.homeNames {
		if slices.Contains(c.secretNames, name) || slices.Contains(c.credentialNames, name) {
			continue
		}
		if v, ok := envValue(c.Env, name); ok && v != "" {
			parts = append(parts, name+envSep+shellQuote(v)+" ")
		}
	}
	return strings.Join(parts, "") + c.String()
}

// envValue returns the value of the last name= entry of env. Pure.
func envValue(env []string, name string) (string, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if n, v, ok := strings.Cut(env[i], envSep); ok && n == name {
			return v, true
		}
	}
	return "", false
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
	// home marks the home rule's set (HomeEnvChange), which
	// Command.ShellLine may print.
	home bool
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
	return envSetHome(homeEnv, h.Dir()), true
}

// envSetHome is EnvSet marked as the home rule's variable (the one env
// entry Command.ShellLine prints). Pure.
func envSetHome(name, value string) EnvChange {
	c := EnvSet(name, value)
	c.home = true
	return c
}

// homeSources lists the names whose last change is a home-rule set: a
// later plain, secret or unset change of the same name takes it out. Pure.
func homeSources(changes []EnvChange) []string {
	var out []string
	for _, c := range changes {
		out = slices.DeleteFunc(out, func(n string) bool { return n == c.name })
		if c.op == EnvSetOp && c.home {
			out = append(out, c.name)
		}
	}
	return out
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

// redacts reports whether Redacted masks name's value, by its source
// (see Redacted). Pure.
func (c Command) redacts(name string) bool {
	switch {
	case slices.Contains(c.secretNames, name), slices.Contains(c.credentialNames, name):
		return true
	case slices.Contains(c.plainNames, name):
		return false
	}
	return isSensitiveName(name)
}

// envSources sorts the names changes set by source: from a Token or
// EnvValue (secret), plain after the same changes scrubbed the name
// (credential), or plain on a name never scrubbed (plain). A name set
// from a secret anywhere stays secret. Pure.
func envSources(changes []EnvChange) (secret, credential, plain []string) {
	var scrubbed []string
	add := func(list *[]string, name string) {
		if !slices.Contains(*list, name) {
			*list = append(*list, name)
		}
	}
	for _, c := range changes {
		switch c.op {
		case EnvUnsetOp:
			add(&scrubbed, c.name)
		case EnvSetSecretOp:
			add(&secret, c.name)
		case EnvSetOp:
			if slices.Contains(scrubbed, c.name) {
				add(&credential, c.name)
			} else {
				add(&plain, c.name)
			}
		}
	}
	plain = slices.DeleteFunc(plain, func(n string) bool {
		return slices.Contains(secret, n) || slices.Contains(credential, n)
	})
	return secret, credential, plain
}

// NewCommand builds a Command whose env is base with changes applied, and
// records which names hold secrets so Redacted masks them. Pure.
func NewCommand(path string, args []string, base []string, dir string, changes ...EnvChange) Command {
	secret, credential, plain := envSources(changes)
	return Command{
		Path:            path,
		Args:            slices.Clone(args),
		Env:             ApplyEnv(base, changes...),
		Dir:             dir,
		secretNames:     secret,
		credentialNames: credential,
		plainNames:      plain,
		homeNames:       homeSources(changes),
	}
}
