package claude

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/khanakia/agentcli"
)

// PermissionMode is a permission mode (open set). OD-13: "manual" and
// "default" decode to the one value ModeManual.
type PermissionMode string

// CLI permission modes at 2.1.323.
const (
	ModeAcceptEdits       PermissionMode = "acceptEdits"
	ModeAuto              PermissionMode = "auto"
	ModeBypassPermissions PermissionMode = "bypassPermissions"
	ModeManual            PermissionMode = "manual"
	ModeDontAsk           PermissionMode = "dontAsk"
	ModePlan              PermissionMode = "plan"
	// modeDefaultSpelling is the hook-input spelling of ModeManual.
	modeDefaultSpelling = "default"
)

// IsKnown reports a documented mode. cost: Pure.
func (m PermissionMode) IsKnown() bool {
	switch m {
	case ModeAcceptEdits, ModeAuto, ModeBypassPermissions, ModeManual, ModeDontAsk, ModePlan:
		return true
	}
	return false
}

// UnmarshalText folds "default" into ModeManual (OD-13). cost: Pure.
func (m *PermissionMode) UnmarshalText(b []byte) error {
	if string(b) == modeDefaultSpelling {
		*m = ModeManual
		return nil
	}
	*m = PermissionMode(b)
	return nil
}

// Rule is one permission rule: "Bash(git *)" → {Tool: Bash, Specifier: "git *"}.
type Rule struct {
	Tool      string
	Specifier string
	Raw       string
}

// Rule syntax delimiters.
const (
	ruleOpen  = "("
	ruleClose = ")"
)

// ParseRule parses a rule; an unbalanced parenthesis is an error. cost: Pure.
func ParseRule(s string) (Rule, error) {
	r := Rule{Raw: s}
	tool, rest, found := strings.Cut(s, ruleOpen)
	if !found {
		if strings.Contains(s, ruleClose) || strings.TrimSpace(s) == "" {
			return r, fmt.Errorf("claude: invalid permission rule %q", s)
		}
		r.Tool = strings.TrimSpace(s)
		return r, nil
	}
	if !strings.HasSuffix(rest, ruleClose) || strings.TrimSpace(tool) == "" {
		return r, fmt.Errorf("claude: invalid permission rule %q", s)
	}
	r.Tool, r.Specifier = strings.TrimSpace(tool), strings.TrimSuffix(rest, ruleClose)
	return r, nil
}

// Permissions is the permissions setting.
type Permissions struct {
	Allow                                []string         `json:"allow,omitempty"`
	Ask                                  []string         `json:"ask,omitempty"`
	Deny                                 []string         `json:"deny,omitempty"`
	AdditionalDirectories                []string         `json:"additionalDirectories,omitempty"`
	DefaultMode                          PermissionMode   `json:"defaultMode,omitempty"`
	DisableBypassPermissionsMode         string           `json:"disableBypassPermissionsMode,omitempty"`
	BlockReadsOutsideWorkingDirectories  *bool            `json:"blockReadsOutsideWorkingDirectories,omitempty"`
	BlockWritesOutsideWorkingDirectories *bool            `json:"blockWritesOutsideWorkingDirectories,omitempty"`
	DenyInsteadOfPrompting               *bool            `json:"denyInsteadOfPrompting,omitempty"`
	Unknown                              agentcli.Unknown `json:"-"`
}

// RuleKind is which list a rule came from.
type RuleKind string

// Rule lists.
const (
	RuleAllow RuleKind = "allow"
	RuleAsk   RuleKind = "ask"
	RuleDeny  RuleKind = "deny"
)

// ScopedRule is a rule with its list and layer.
type ScopedRule struct {
	Rule  Rule
	Kind  RuleKind
	Scope SettingsScope
	Err   error // the rule did not parse; Rule.Raw holds it
}

// EffectivePermissions are the rules of every layer and the winning mode.
type EffectivePermissions struct {
	Rules       []ScopedRule
	DefaultMode PermissionMode
	ModeSource  SettingsScope
}

// Permissions lists every rule of every layer (highest first) and the
// default mode the highest layer sets. cost: Disk · Read.
func (a *Agent) Permissions(h Home, p *Project) (EffectivePermissions, error) {
	eff, err := a.Settings(h, p, nil)
	if err != nil {
		return EffectivePermissions{}, err
	}
	var out EffectivePermissions
	for _, l := range eff.Layers {
		pm := l.Settings.Permissions
		if pm == nil {
			continue
		}
		for _, set := range []struct {
			kind  RuleKind
			rules []string
		}{{RuleAllow, pm.Allow}, {RuleAsk, pm.Ask}, {RuleDeny, pm.Deny}} {
			for _, s := range set.rules {
				r, perr := ParseRule(s)
				out.Rules = append(out.Rules, ScopedRule{Rule: r, Kind: set.kind, Scope: l.Scope, Err: perr})
			}
		}
		if out.DefaultMode == "" && pm.DefaultMode != "" {
			out.DefaultMode, out.ModeSource = pm.DefaultMode, l.Scope
		}
	}
	return out, nil
}

// Each nested settings object keeps its unmodelled keys. Why one method per
// type: a generic helper would need an `any` constraint, which the library
// forbids (SPEC §5.5).

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (p *Permissions) UnmarshalJSON(b []byte) error {
	type plain Permissions
	var v plain
	u, _, err := decodeLenient(keyPermissions, b, reflect.TypeFor[plain](), func(d []byte) error { v = plain{}; return json.Unmarshal(d, &v) })
	*p = Permissions(v)
	p.Unknown = u
	return err
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *StatusLine) UnmarshalJSON(b []byte) error {
	type plain StatusLine
	var v plain
	u, _, err := decodeLenient("statusLine", b, reflect.TypeFor[plain](), func(d []byte) error { v = plain{}; return json.Unmarshal(d, &v) })
	*s = StatusLine(v)
	s.Unknown = u
	return err
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (m *ModelSetting) UnmarshalJSON(b []byte) error {
	type plain ModelSetting
	var v plain
	u, _, err := decodeLenient(keyModelSettings, b, reflect.TypeFor[plain](), func(d []byte) error { v = plain{}; return json.Unmarshal(d, &v) })
	*m = ModelSetting(v)
	m.Unknown = u
	return err
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *Attribution) UnmarshalJSON(b []byte) error {
	type plain Attribution
	var v plain
	u, _, err := decodeLenient("attribution", b, reflect.TypeFor[plain](), func(d []byte) error { v = plain{}; return json.Unmarshal(d, &v) })
	*s = Attribution(v)
	s.Unknown = u
	return err
}

// UnmarshalJSON keeps unknown keys (nested filesystem/network/credentials). cost: Pure.
func (s *Sandbox) UnmarshalJSON(b []byte) error {
	type plain Sandbox
	var v plain
	u, _, err := decodeLenient("sandbox", b, reflect.TypeFor[plain](), func(d []byte) error { v = plain{}; return json.Unmarshal(d, &v) })
	*s = Sandbox(v)
	s.Unknown = u
	return err
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *WorktreeSettings) UnmarshalJSON(b []byte) error {
	type plain WorktreeSettings
	var v plain
	u, _, err := decodeLenient("worktree", b, reflect.TypeFor[plain](), func(d []byte) error { v = plain{}; return json.Unmarshal(d, &v) })
	*s = WorktreeSettings(v)
	s.Unknown = u
	return err
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (m *MarketplaceDecl) UnmarshalJSON(b []byte) error {
	type plain MarketplaceDecl
	var v plain
	u, _, err := decodeLenient(keyMarketplaces, b, reflect.TypeFor[plain](), func(d []byte) error { v = plain{}; return json.Unmarshal(d, &v) })
	*m = MarketplaceDecl(v)
	m.Unknown = u
	return err
}
