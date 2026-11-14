package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"

	"github.com/ubgo/agentio"
)

// Settings round trip, validation and schema.
//
// Why: a launcher (agx) merges settings from a profile and from
// --setting key=value, type-checks the keys this library models, and
// hands the result to Claude unchanged. Decoding into Settings alone
// would drop every key the struct does not model, so each settings type
// writes its kept Unknown keys back (MarshalJSON below): ParseSettings
// followed by json.Marshal loses nothing.
//
// Each type has its own MarshalJSON because a generic helper would need
// an `any` constraint, which the library forbids.

// mergeUnknown merges kept unknown keys into an encoded object; modelled
// keys win. Pure.
func mergeUnknown(b []byte, err error, unknown agentio.Unknown) ([]byte, error) {
	if err != nil || len(unknown) == 0 {
		return b, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range unknown {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

// MarshalJSON writes every modelled key and every kept unknown key. cost: Pure.
func (s Settings) MarshalJSON() ([]byte, error) {
	type plain Settings
	b, err := json.Marshal(plain(s))
	return mergeUnknown(b, err, s.Unknown)
}

// MarshalJSON keeps unknown keys. cost: Pure.
func (p Permissions) MarshalJSON() ([]byte, error) {
	type plain Permissions
	b, err := json.Marshal(plain(p))
	return mergeUnknown(b, err, p.Unknown)
}

// MarshalJSON keeps unknown keys. cost: Pure.
func (s StatusLine) MarshalJSON() ([]byte, error) {
	type plain StatusLine
	b, err := json.Marshal(plain(s))
	return mergeUnknown(b, err, s.Unknown)
}

// MarshalJSON keeps unknown keys. cost: Pure.
func (m ModelSetting) MarshalJSON() ([]byte, error) {
	type plain ModelSetting
	b, err := json.Marshal(plain(m))
	return mergeUnknown(b, err, m.Unknown)
}

// MarshalJSON keeps unknown keys. cost: Pure.
func (s Attribution) MarshalJSON() ([]byte, error) {
	type plain Attribution
	b, err := json.Marshal(plain(s))
	return mergeUnknown(b, err, s.Unknown)
}

// MarshalJSON keeps unknown keys (nested filesystem/network/credentials). cost: Pure.
func (s Sandbox) MarshalJSON() ([]byte, error) {
	type plain Sandbox
	b, err := json.Marshal(plain(s))
	return mergeUnknown(b, err, s.Unknown)
}

// MarshalJSON keeps unknown keys. cost: Pure.
func (s WorktreeSettings) MarshalJSON() ([]byte, error) {
	type plain WorktreeSettings
	b, err := json.Marshal(plain(s))
	return mergeUnknown(b, err, s.Unknown)
}

// MarshalJSON keeps unknown keys. cost: Pure.
func (m MarketplaceDecl) MarshalJSON() ([]byte, error) {
	type plain MarketplaceDecl
	b, err := json.Marshal(plain(m))
	return mergeUnknown(b, err, m.Unknown)
}

// MarshalJSON keeps unknown keys. cost: Pure.
func (s MarketplaceSource) MarshalJSON() ([]byte, error) {
	type plain MarketplaceSource
	b, err := json.Marshal(plain(s))
	return mergeUnknown(b, err, s.Unknown)
}

// SettingsCheck is the result of ValidateSettings.
type SettingsCheck struct {
	// Settings is the decoded object; json.Marshal of it writes every key
	// back, modelled or not.
	Settings Settings
	// UnknownKeys are keys this library does not model, dotted for nested
	// objects ("permissions.newKey"), sorted. They are kept and passed on;
	// a caller shows them as warnings (Claude may know them).
	UnknownKeys []string
	// TypeErrors are modelled keys whose value has the wrong type; each
	// names the key and the expected type, never the value.
	TypeErrors agentio.Warnings
}

// OK reports a settings object without type errors (unknown keys are not
// errors). cost: Pure.
func (c SettingsCheck) OK() bool { return len(c.TypeErrors) == 0 }

// ValidateSettings type-checks a settings JSON object (a launcher's merged
// map, encoded) against the keys this library models: wrong types are
// TypeErrors, unknown keys are listed but kept. A body that is not a JSON
// object is an error. cost: Pure.
func ValidateSettings(data []byte) (SettingsCheck, error) {
	s, err := ParseSettings("settings", data)
	if err != nil {
		return SettingsCheck{}, err
	}
	c := SettingsCheck{Settings: s, TypeErrors: s.Warnings}
	add := func(prefix string, u agentio.Unknown) {
		for _, k := range u.Keys() {
			c.UnknownKeys = append(c.UnknownKeys, prefix+k)
		}
	}
	for _, w := range s.Warnings {
		delete(s.Unknown, w.Key) // a type error is reported once, not also as unknown
	}
	add("", s.Unknown)
	if s.Permissions != nil {
		add(keyPermissions+".", s.Permissions.Unknown)
	}
	if s.StatusLine != nil {
		add("statusLine.", s.StatusLine.Unknown)
	}
	if s.Sandbox != nil {
		add("sandbox.", s.Sandbox.Unknown)
	}
	if s.Attribution != nil {
		add("attribution.", s.Attribution.Unknown)
	}
	if s.Worktree != nil {
		add("worktree.", s.Worktree.Unknown)
	}
	sort.Strings(c.UnknownKeys)
	return c, nil
}

// SettingsKeys lists the top-level settings keys this library models,
// sorted. cost: Pure.
func SettingsKeys() []string { return agentio.SchemaKeys(SettingsSchema()) }

// SettingsSchema is the JSON Schema of the modelled settings keys, open to
// other keys (Claude knows more than this library models). cost: Pure.
func SettingsSchema() agentio.SchemaNode {
	return agentio.JSONSchemaFor(reflect.TypeFor[Settings](), schemaEnums, false)
}

// schemaEnums lists the allowed values of Claude's named string types for
// JSON Schema enums; nil for open types. cost: Pure.
func schemaEnums(t reflect.Type) []string {
	switch t {
	case reflect.TypeFor[Effort]():
		return stringsOf(EffortValues())
	case reflect.TypeFor[PermissionMode]():
		return stringsOf(PermissionModeValues())
	case reflect.TypeFor[UpdateChannel]():
		return stringsOf(UpdateChannelValues())
	case reflect.TypeFor[OutputFormat]():
		return stringsOf(OutputFormatValues())
	case reflect.TypeFor[InputFormat]():
		return stringsOf(InputFormatValues())
	case reflect.TypeFor[SettingsScope]():
		return stringsOf(CLISettingSources())
	}
	return nil
}

// stringsOf converts a named string list. cost: Pure.
func stringsOf[S ~string](vs []S) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}

// EffortValues lists the documented effort levels. cost: Pure.
func EffortValues() []Effort {
	return []Effort{EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax}
}

// PermissionModeValues lists the CLI permission modes. cost: Pure.
func PermissionModeValues() []PermissionMode {
	return []PermissionMode{ModeAcceptEdits, ModeAuto, ModeBypassPermissions, ModeManual, ModeDontAsk, ModePlan}
}

// UpdateChannelValues lists the update channels. cost: Pure.
func UpdateChannelValues() []UpdateChannel { return []UpdateChannel{ChannelLatest, ChannelStable} }

// OutputFormatValues lists --output-format values. cost: Pure.
func OutputFormatValues() []OutputFormat {
	return []OutputFormat{OutputText, OutputJSON, OutputStreamJSON}
}

// InputFormatValues lists --input-format values. cost: Pure.
func InputFormatValues() []InputFormat { return []InputFormat{InputText, InputStreamJSON} }

// CLISettingSources are the layers --setting-sources accepts (user,
// project, local; managed and cli are not selectable). cost: Pure.
func CLISettingSources() []SettingsScope {
	return []SettingsScope{ScopeUser, ScopeProject, ScopeLocal}
}

// SettingsVia says how per-launch settings reach Claude: a 0600 file
// whose path goes on argv, or the JSON itself on argv (visible in ps).
type SettingsVia uint8

// SettingsVia values.
const (
	// SettingsViaFile writes <dir>/settings-<run id>.json (0600) and
	// passes its path: nothing secret appears in the process list.
	SettingsViaFile SettingsVia = iota
	// SettingsViaInline passes the JSON on argv: no file to clean up, but
	// every key (an apiKeyHelper command, env values) shows in ps.
	SettingsViaInline
)

// File modes of a per-launch settings file and its dir: owner only,
// because a settings file can hold an apiKeyHelper command or env values.
const (
	launchSettingsDirMode  = 0o700
	launchSettingsFileMode = 0o600
	launchSettingsPrefix   = "settings-"
	launchSettingsSuffix   = ".json"
)

// runIDPattern is the run id charset: it becomes part of a file name.
const runIDPattern = `^[A-Za-z0-9][A-Za-z0-9._-]*$`

// ErrBadRunID means a run id that cannot name a file.
var ErrBadRunID = errors.New("claude: run id must match " + runIDPattern)

// LaunchSettings prepares merged settings for one launch and returns the
// value for LaunchOptions.Settings:
//   - SettingsViaFile: writes data to <dir>/settings-<runID>.json with
//     mode 0600 (dir created 0700), refusing an existing file so two runs
//     never share one, and returns the path;
//   - SettingsViaInline: returns the JSON itself (dir and runID unused).
//
// data must be a JSON object (a caller's merged map, encoded); it is
// written as given, so unknown keys pass through. The caller removes the
// file once the agent has ended (agentio.Host.ProcessAlive tells when).
// cost: Disk · Write for SettingsViaFile, Pure for SettingsViaInline.
func LaunchSettings(data []byte, via SettingsVia, dir, runID string) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil || obj == nil {
		return "", fmt.Errorf("claude: launch settings must be a JSON object: %w", err)
	}
	if via == SettingsViaInline {
		return string(data), nil
	}
	if !regexp.MustCompile(runIDPattern).MatchString(runID) {
		return "", ErrBadRunID
	}
	return writeLaunchSettingsFile(dir, runID, data)
}

// writeLaunchSettingsFile creates <dir>/settings-<runID>.json (0600, dir
// 0700), refusing an existing file. It writes the caller's own run dir,
// never a file Claude owns. cost: Disk · Write.
func writeLaunchSettingsFile(dir, runID string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, launchSettingsDirMode); err != nil {
		return "", fmt.Errorf("claude: launch settings dir: %w", err)
	}
	p := filepath.Join(dir, launchSettingsPrefix+runID+launchSettingsSuffix)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, launchSettingsFileMode)
	if err != nil {
		return "", fmt.Errorf("claude: launch settings: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close() // the write error is the one to report
		return "", fmt.Errorf("claude: launch settings: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("claude: launch settings: %w", err)
	}
	return p, nil
}
