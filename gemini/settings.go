package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// ApprovalMode is Gemini's approval mode (--approval-mode choices, and
// general.defaultApprovalMode without yolo). Vendor-owned open set: an
// unknown value is kept as read.
type ApprovalMode string

// ApprovalMode values (0.68.0 --help).
const (
	ApprovalDefault  ApprovalMode = "default"
	ApprovalAutoEdit ApprovalMode = "auto_edit"
	ApprovalYolo     ApprovalMode = "yolo"
	ApprovalPlan     ApprovalMode = "plan"
)

// ApprovalModeValues lists the known modes. cost: Pure.
func ApprovalModeValues() []ApprovalMode {
	return []ApprovalMode{ApprovalDefault, ApprovalAutoEdit, ApprovalYolo, ApprovalPlan}
}

// IsKnown reports a documented mode. cost: Pure.
func (m ApprovalMode) IsKnown() bool { return slices.Contains(ApprovalModeValues(), m) }

// Autonomy maps the mode onto agentio's levels; false for an unknown mode.
// cost: Pure.
func (m ApprovalMode) Autonomy() (agentio.Autonomy, bool) {
	switch m {
	case ApprovalDefault:
		return agentio.AutonomyAsk, true
	case ApprovalAutoEdit:
		return agentio.AutonomyAcceptEdits, true
	case ApprovalYolo:
		return agentio.AutonomyFull, true
	case ApprovalPlan:
		return agentio.AutonomyReadOnly, true
	}
	return agentio.AutonomyDefault, false
}

// AuthType is security.auth.selectedType (core AuthType). Vendor-owned
// open set.
type AuthType string

// AuthType values (0.68.0 core contentGenerator).
const (
	AuthGoogleLogin AuthType = "oauth-personal"
	AuthGeminiKey   AuthType = "gemini-api-key"
	AuthVertex      AuthType = "vertex-ai"
	AuthCloudShell  AuthType = "cloud-shell"
	AuthComputeADC  AuthType = "compute-default-credentials"
	AuthGateway     AuthType = "gateway"
)

// AuthTypeValues lists the known auth types. cost: Pure.
func AuthTypeValues() []AuthType {
	return []AuthType{AuthGoogleLogin, AuthGeminiKey, AuthVertex, AuthCloudShell, AuthComputeADC, AuthGateway}
}

// IsKnown reports a documented auth type. cost: Pure.
func (t AuthType) IsKnown() bool { return slices.Contains(AuthTypeValues(), t) }

// StringList is a settings value that is a string or a list of strings
// (context.fileName). It always decodes to a list and encodes a single
// entry back as a string, as the user wrote it most often.
type StringList []string

// UnmarshalJSON accepts "a" or ["a","b"]. cost: Pure.
func (l *StringList) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*l = StringList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("gemini: want a string or a list of strings: %w", err)
	}
	*l = many
	return nil
}

// MarshalJSON writes one entry as a string, else a list. cost: Pure.
func (l StringList) MarshalJSON() ([]byte, error) {
	if len(l) == 1 {
		return json.Marshal(l[0])
	}
	return json.Marshal([]string(l))
}

// Settings is one settings.json (user, workspace or system). The sections
// agentio uses are typed; every other top-level key, and every unmodelled
// key inside a typed section, is kept raw in Unknown, so MarshalJSON
// writes back what was read. A section whose value has another type than
// the schema says is kept raw in Unknown too, with a warning (Gemini CLI
// itself keeps such keys and warns).
type Settings struct {
	General    *GeneralSettings           `json:"general,omitempty"`
	Model      *ModelSettings             `json:"model,omitempty"`
	Context    *ContextSettings           `json:"context,omitempty"`
	Security   *SecuritySettings          `json:"security,omitempty"`
	Privacy    *PrivacySettings           `json:"privacy,omitempty"`
	Tools      *ToolsSettings             `json:"tools,omitempty"`
	MCP        *MCPSettings               `json:"mcp,omitempty"`
	MCPServers map[string]MCPServerConfig `json:"mcpServers,omitempty"`
	// Unknown holds every top-level key not modelled above.
	Unknown agentio.Unknown `json:"-"`
}

// GeneralSettings is settings.general.
type GeneralSettings struct {
	// DefaultApprovalMode never holds yolo (the CLI ignores it here).
	DefaultApprovalMode          ApprovalMode             `json:"defaultApprovalMode,omitempty"`
	EnableAutoUpdate             *bool                    `json:"enableAutoUpdate,omitempty"`
	EnableAutoUpdateNotification *bool                    `json:"enableAutoUpdateNotification,omitempty"`
	PreferredEditor              string                   `json:"preferredEditor,omitempty"`
	VimMode                      *bool                    `json:"vimMode,omitempty"`
	Checkpointing                *CheckpointingSettings   `json:"checkpointing,omitempty"`
	SessionRetention             *SessionRetentionSetting `json:"sessionRetention,omitempty"`
	Unknown                      agentio.Unknown          `json:"-"`
}

// CheckpointingSettings is general.checkpointing.
type CheckpointingSettings struct {
	Enabled *bool           `json:"enabled,omitempty"`
	Unknown agentio.Unknown `json:"-"`
}

// SessionRetentionSetting is general.sessionRetention: chats older than
// MaxAge (default "30d") are deleted at the CLI's startup.
type SessionRetentionSetting struct {
	Enabled      *bool           `json:"enabled,omitempty"`
	MaxAge       string          `json:"maxAge,omitempty"`
	MaxCount     *int            `json:"maxCount,omitempty"`
	MinRetention string          `json:"minRetention,omitempty"`
	Unknown      agentio.Unknown `json:"-"`
}

// ModelSettings is settings.model.
type ModelSettings struct {
	// Name is the model; "" = the CLI default ("auto").
	Name                 string          `json:"name,omitempty"`
	MaxSessionTurns      *int            `json:"maxSessionTurns,omitempty"`
	CompressionThreshold *float64        `json:"compressionThreshold,omitempty"`
	Unknown              agentio.Unknown `json:"-"`
}

// ContextSettings is settings.context.
type ContextSettings struct {
	// FileName replaces GEMINI.md as the instruction file name(s).
	FileName                         StringList      `json:"fileName,omitempty"`
	IncludeDirectories               []string        `json:"includeDirectories,omitempty"`
	MemoryBoundaryMarkers            []string        `json:"memoryBoundaryMarkers,omitempty"`
	LoadMemoryFromIncludeDirectories *bool           `json:"loadMemoryFromIncludeDirectories,omitempty"`
	Unknown                          agentio.Unknown `json:"-"`
}

// SecuritySettings is settings.security.
type SecuritySettings struct {
	Auth            *AuthSettings        `json:"auth,omitempty"`
	FolderTrust     *FolderTrustSettings `json:"folderTrust,omitempty"`
	DisableYoloMode *bool                `json:"disableYoloMode,omitempty"`
	Unknown         agentio.Unknown      `json:"-"`
}

// AuthSettings is security.auth: the home's login method.
type AuthSettings struct {
	SelectedType AuthType        `json:"selectedType,omitempty"`
	EnforcedType AuthType        `json:"enforcedType,omitempty"`
	UseExternal  *bool           `json:"useExternal,omitempty"`
	Unknown      agentio.Unknown `json:"-"`
}

// FolderTrustSettings is security.folderTrust (on by default).
type FolderTrustSettings struct {
	Enabled *bool           `json:"enabled,omitempty"`
	Unknown agentio.Unknown `json:"-"`
}

// PrivacySettings is settings.privacy.
type PrivacySettings struct {
	UsageStatisticsEnabled *bool           `json:"usageStatisticsEnabled,omitempty"`
	Unknown                agentio.Unknown `json:"-"`
}

// ToolsSettings is settings.tools.
type ToolsSettings struct {
	// Sandbox is a bool, a sandbox command name or an object: kept raw.
	Sandbox agentio.Raw     `json:"sandbox,omitempty"`
	Core    []string        `json:"core,omitempty"`
	Allowed []string        `json:"allowed,omitempty"`
	Exclude []string        `json:"exclude,omitempty"`
	Unknown agentio.Unknown `json:"-"`
}

// MCPSettings is settings.mcp: global server filters.
type MCPSettings struct {
	ServerCommand string          `json:"serverCommand,omitempty"`
	Allowed       []string        `json:"allowed,omitempty"`
	Excluded      []string        `json:"excluded,omitempty"`
	Unknown       agentio.Unknown `json:"-"`
}

// MCPServerConfig is one mcpServers entry (cli MCPServerConfig schema).
// Env and Headers values may be secrets: they are agentio.EnvValue, which
// redacts when printed.
type MCPServerConfig struct {
	Command      string                      `json:"command,omitempty"`
	Args         []string                    `json:"args,omitempty"`
	Env          map[string]agentio.EnvValue `json:"env,omitempty"`
	Cwd          string                      `json:"cwd,omitempty"`
	URL          string                      `json:"url,omitempty"`
	HTTPURL      string                      `json:"httpUrl,omitempty"`
	TCP          string                      `json:"tcp,omitempty"`
	Type         string                      `json:"type,omitempty"`
	Headers      map[string]agentio.EnvValue `json:"headers,omitempty"`
	Timeout      *int64                      `json:"timeout,omitempty"`
	Trust        *bool                       `json:"trust,omitempty"`
	Description  string                      `json:"description,omitempty"`
	IncludeTools []string                    `json:"includeTools,omitempty"`
	ExcludeTools []string                    `json:"excludeTools,omitempty"`
	Unknown      agentio.Unknown             `json:"-"`
}

// Section decoders: each typed struct keeps its unmodelled keys. The
// pattern is the same for every section; a local wire type without
// methods avoids recursion.

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *GeneralSettings) UnmarshalJSON(b []byte) error {
	type wire GeneralSettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = GeneralSettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s GeneralSettings) MarshalJSON() ([]byte, error) {
	type wire GeneralSettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *CheckpointingSettings) UnmarshalJSON(b []byte) error {
	type wire CheckpointingSettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = CheckpointingSettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s CheckpointingSettings) MarshalJSON() ([]byte, error) {
	type wire CheckpointingSettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *SessionRetentionSetting) UnmarshalJSON(b []byte) error {
	type wire SessionRetentionSetting
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = SessionRetentionSetting(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s SessionRetentionSetting) MarshalJSON() ([]byte, error) {
	type wire SessionRetentionSetting
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *ModelSettings) UnmarshalJSON(b []byte) error {
	type wire ModelSettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = ModelSettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s ModelSettings) MarshalJSON() ([]byte, error) {
	type wire ModelSettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *ContextSettings) UnmarshalJSON(b []byte) error {
	type wire ContextSettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = ContextSettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s ContextSettings) MarshalJSON() ([]byte, error) {
	type wire ContextSettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *SecuritySettings) UnmarshalJSON(b []byte) error {
	type wire SecuritySettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = SecuritySettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s SecuritySettings) MarshalJSON() ([]byte, error) {
	type wire SecuritySettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *AuthSettings) UnmarshalJSON(b []byte) error {
	type wire AuthSettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = AuthSettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s AuthSettings) MarshalJSON() ([]byte, error) {
	type wire AuthSettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *FolderTrustSettings) UnmarshalJSON(b []byte) error {
	type wire FolderTrustSettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = FolderTrustSettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s FolderTrustSettings) MarshalJSON() ([]byte, error) {
	type wire FolderTrustSettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *PrivacySettings) UnmarshalJSON(b []byte) error {
	type wire PrivacySettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = PrivacySettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s PrivacySettings) MarshalJSON() ([]byte, error) {
	type wire PrivacySettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *ToolsSettings) UnmarshalJSON(b []byte) error {
	type wire ToolsSettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = ToolsSettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s ToolsSettings) MarshalJSON() ([]byte, error) {
	type wire ToolsSettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *MCPSettings) UnmarshalJSON(b []byte) error {
	type wire MCPSettings
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = MCPSettings(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s MCPSettings) MarshalJSON() ([]byte, error) {
	type wire MCPSettings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *MCPServerConfig) UnmarshalJSON(b []byte) error {
	type wire MCPServerConfig
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*s, s.Unknown = MCPServerConfig(w), u
	return errors.Join(err, uerr)
}

// MarshalJSON writes typed and unknown keys. cost: Pure.
func (s MCPServerConfig) MarshalJSON() ([]byte, error) {
	type wire MCPServerConfig
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// withUnknown merges unknown back into the typed object b (typed keys win
// on a clash, which cannot happen for decoded values). Pure.
func withUnknown(b []byte, err error, unknown agentio.Unknown) ([]byte, error) {
	if err != nil || len(unknown) == 0 {
		return b, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	for k, raw := range unknown {
		if _, typed := obj[k]; !typed {
			obj[k] = raw
		}
	}
	return json.Marshal(obj)
}

// Top-level settings keys agentio types (the settings schema).
const (
	secGeneral    = "general"
	secModel      = "model"
	secContext    = "context"
	secSecurity   = "security"
	secPrivacy    = "privacy"
	secTools      = "tools"
	secMCP        = "mcp"
	secMCPServers = "mcpServers"
)

// UnmarshalJSON decodes every top-level key on its own: a modelled key
// with an unexpected type moves to Unknown instead of failing the file.
// Use DecodeSettings to also get the warnings. cost: Pure.
func (s *Settings) UnmarshalJSON(b []byte) error {
	out, _, err := decodeSettings(b)
	*s = out
	return err
}

// MarshalJSON writes typed sections and every unknown key. cost: Pure.
func (s Settings) MarshalJSON() ([]byte, error) {
	type wire Settings
	b, err := json.Marshal(wire(s))
	return withUnknown(b, err, s.Unknown)
}

// DecodeSettings decodes a settings.json body (comments allowed, as the
// CLI strips them; a UTF-8 BOM is dropped). Warnings name modelled keys
// kept raw because their type differs from the schema. cost: Pure.
func DecodeSettings(data []byte) (Settings, []string, error) {
	return decodeSettings(data)
}

// decodeSettings is DecodeSettings.
func decodeSettings(data []byte) (Settings, []string, error) {
	clean := StripJSONComments(agentio.StripBOM(data))
	var s Settings
	if len(bytes.TrimSpace(clean)) == 0 {
		return s, nil, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(clean, &top); err != nil {
		return s, nil, fmt.Errorf("gemini: settings: %w", err)
	}
	var warns []string
	keep := func(k string, raw json.RawMessage, err error) {
		warns = append(warns, fmt.Sprintf("settings key %q kept raw: %v", k, err))
		if s.Unknown == nil {
			s.Unknown = agentio.Unknown{}
		}
		s.Unknown[k] = raw
	}
	for k, raw := range top {
		var err error
		switch k {
		case secGeneral:
			err = json.Unmarshal(raw, &s.General)
		case secModel:
			err = json.Unmarshal(raw, &s.Model)
		case secContext:
			err = json.Unmarshal(raw, &s.Context)
		case secSecurity:
			err = json.Unmarshal(raw, &s.Security)
		case secPrivacy:
			err = json.Unmarshal(raw, &s.Privacy)
		case secTools:
			err = json.Unmarshal(raw, &s.Tools)
		case secMCP:
			err = json.Unmarshal(raw, &s.MCP)
		case secMCPServers:
			err = json.Unmarshal(raw, &s.MCPServers)
		default:
			if s.Unknown == nil {
				s.Unknown = agentio.Unknown{}
			}
			s.Unknown[k] = raw
			continue
		}
		if err != nil {
			s.clearSection(k)
			keep(k, raw, err)
		}
	}
	slices.Sort(warns)
	return s, warns, nil
}

// clearSection drops a typed section that failed to decode (json leaves a
// half-filled value behind), so it lives only in Unknown. cost: Pure.
func (s *Settings) clearSection(k string) {
	switch k {
	case secGeneral:
		s.General = nil
	case secModel:
		s.Model = nil
	case secContext:
		s.Context = nil
	case secSecurity:
		s.Security = nil
	case secPrivacy:
		s.Privacy = nil
	case secTools:
		s.Tools = nil
	case secMCP:
		s.MCP = nil
	case secMCPServers:
		s.MCPServers = nil
	}
}

// StripJSONComments removes // line and /* block */ comments outside JSON
// strings, keeping newlines so error offsets still point at the right
// line. Gemini CLI reads settings.json through strip-json-comments, so a
// commented file is valid there and must be valid here. cost: Pure.
func StripJSONComments(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inString, escaped := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			if i < len(b) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i < len(b) && (b[i] != '*' || i+1 >= len(b) || b[i+1] != '/') {
				if b[i] == '\n' {
					out = append(out, '\n')
				}
				i++
			}
			i++ // skip the closing '/'
		default:
			out = append(out, c)
		}
	}
	return out
}

// SettingsFile is one settings layer as read.
type SettingsFile struct {
	Scope agentio.ConfigScope
	Path  string
	// Exists is false for a layer whose file is absent (Settings is zero).
	Exists   bool
	Settings Settings
	// Warnings are keys kept raw (DecodeSettings).
	Warnings []string
}

// ReadSettingsFile reads one settings.json; a missing file is Exists
// false with no error. cost: Disk · Read.
func ReadSettingsFile(scope agentio.ConfigScope, path string) (SettingsFile, error) {
	f := SettingsFile{Scope: scope, Path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, fmt.Errorf("gemini: read %s: %w", path, err)
	}
	f.Exists = true
	s, warns, err := DecodeSettings(data)
	if err != nil {
		return f, fmt.Errorf("gemini: %s: %w", path, err)
	}
	f.Settings, f.Warnings = s, warns
	return f, nil
}

// UserSettingsPath is <home>/settings.json. cost: Pure.
func (h Home) UserSettingsPath() string { return h.Path(settingsFile) }

// WorkspaceSettingsPath is <dir>/.gemini/settings.json. cost: Pure.
func WorkspaceSettingsPath(dir string) string {
	return joinPath(dir, defaultDirName, settingsFile)
}

// systemDir is the system settings dir for goos. cost: Pure.
func systemDir(goos string) string {
	switch goos {
	case agentio.OSDarwin:
		return systemDirDarwin
	case agentio.OSWindows:
		return systemDirWindows
	}
	return systemDirLinux
}

// SystemSettingsPath is the system settings file for the Host: the
// GEMINI_CLI_SYSTEM_SETTINGS_PATH value when set, else the per-OS path.
// cost: Pure.
func (a *Agent) SystemSettingsPath() string {
	if p, ok := a.host.Getenv(SystemSettingsEnv); ok && p != "" {
		return p
	}
	return joinFor(a.goos(), systemDir(a.goos()), settingsFile)
}

// SystemDefaultsPath is the system defaults file (lowest-precedence
// layer): GEMINI_CLI_SYSTEM_DEFAULTS_PATH, else system-defaults.json next
// to the system settings. cost: Pure.
func (a *Agent) SystemDefaultsPath() string {
	if p, ok := a.host.Getenv(SystemDefaultsEnv); ok && p != "" {
		return p
	}
	return joinFor(a.goos(), systemDir(a.goos()), systemDefaultsFile)
}

// goos is the Host's OS, the running one when unset.
func (a *Agent) goos() string {
	if a.host.GOOS != "" {
		return a.host.GOOS
	}
	return hostGOOS()
}

// joinFor joins a fixed per-OS dir and a file name with that OS's
// separator (the system dirs are absolute paths of the target OS).
// cost: Pure.
func joinFor(goos, dir, name string) string {
	if goos == agentio.OSWindows {
		return strings.TrimRight(dir, `\`) + `\` + name
	}
	return strings.TrimRight(dir, "/") + "/" + name
}

// Settings reads the layers that apply to h and dir, highest precedence
// first as agentio.ConfigReader orders them: system (forced by an admin),
// workspace (only when dir is not the home's parent, and only applied by
// the CLI in a trusted folder), user, system defaults. dir "" skips the
// workspace layer. cost: Disk · Read.
func (a *Agent) Settings(h Home, dir string) ([]SettingsFile, error) {
	type layer struct {
		scope agentio.ConfigScope
		path  string
	}
	layers := []layer{{agentio.ConfigManaged, a.SystemSettingsPath()}}
	if ws, ok := workspaceSettings(h, dir); ok {
		layers = append(layers, layer{agentio.ConfigProject, ws})
	}
	layers = append(layers, layer{agentio.ConfigUser, h.UserSettingsPath()}, layer{agentio.ConfigManaged, a.SystemDefaultsPath()})
	out := make([]SettingsFile, 0, len(layers))
	for _, l := range layers {
		f, err := ReadSettingsFile(l.scope, l.path)
		if err != nil {
			return out, err
		}
		out = append(out, f)
	}
	return out, nil
}

// workspaceSettings is dir's .gemini/settings.json unless it is the home's
// own settings file (the CLI skips the workspace layer when the workspace
// is the home dir, so the user file is not read twice). cost: Pure.
func workspaceSettings(h Home, dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	ws := WorkspaceSettingsPath(dir)
	if samePath(ws, h.UserSettingsPath()) {
		return "", false
	}
	return ws, true
}
