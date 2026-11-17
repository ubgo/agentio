package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/ubgo/agentio"
)

// Launch configuration through the neutral Agent (agentio.LaunchConfigOf):
// typed options from the registry over the neutral request's mapping, and
// a settings object whose leaves become `-c path=value` overrides, each
// checked by CheckOverride. Codex merges every -c over config.toml, so
// settings always travel on argv; LaunchConfig.Delivery does not apply.

// optionConcerns names the neutral concern of each option key that
// overlaps a LaunchRequest field (the autonomy keys are the ones the
// neutral mapping sets).
func optionConcerns() map[string]agentio.Concern {
	return map[string]agentio.Concern{
		"model":      agentio.ConcernModel,
		"prompt":     agentio.ConcernPrompt,
		"add_dirs":   agentio.ConcernAddDir,
		"sandbox":    agentio.ConcernAutonomy,
		"approval":   agentio.ConcernAutonomy,
		"bypass_all": agentio.ConcernAutonomy,
	}
}

// Config paths that overlap a neutral concern (CheckSettings).
const (
	settingsKeyModel   = "model"
	settingsKeySandbox = "sandbox_mode"
	settingsPathSep    = "."
)

// Wording of settings findings.
const (
	detailUnknownOverride = "not modelled by agentio; passed to Codex unchecked"
	detailRawOverride     = "a section agentio keeps raw; passed to Codex unchecked"
	detailUnlistedPrefix  = "value not listed (Codex keeps it): "
	suggestionPrefix      = "; did you mean "
)

// OptionKeys is Pure.
func (n neutral) OptionKeys() ([]agentio.OptionKey, error) { return LaunchOptionKeys() }

// OptionsSchema is Pure.
func (n neutral) OptionsSchema() (agentio.SchemaNode, error) { return LaunchOptionsSchema() }

// CheckOptions validates an options object against the registry. cost: Pure.
func (n neutral) CheckOptions(options agentio.Raw) error {
	keys, err := LaunchOptionKeys()
	if err != nil {
		return err
	}
	return agentio.CheckOptionsObject(options, keys)
}

// CheckSettings flattens a settings object to `-c` overrides and checks
// each with CheckOverride: type-error → TypeErrors; unknown, raw and
// unlisted-value → Unknown (passed on); model, sandbox_mode and
// approval_policy → Concerns. cost: Pure.
func (n neutral) CheckSettings(settings agentio.Raw) (agentio.SettingsCheck, error) {
	_, check, err := checkedOverrides(settings)
	return check, err
}

// RawSettingsArg is always false for Codex: raw `-c` args add to the
// launch's overrides (Codex applies every -c, later ones winning per
// path) instead of replacing a settings object, so the launcher keeps
// its own. cost: Pure.
func (n neutral) RawSettingsArg([]string) bool { return false }

// LaunchConfigured is Launch with c applied. cost: Pure.
func (n neutral) LaunchConfigured(t agentio.Target, r agentio.LaunchRequest, c agentio.LaunchConfig) (agentio.Command, error) {
	if err := n.Owns(t.Home); err != nil {
		return agentio.Command{}, err
	}
	return n.a.launchConfigured(t, r, c)
}

// mapRequest is optionsFromRequest as an agentio.RequestMapper; it only
// names a conflict's source, so the target allows every level. cost: Pure.
func mapRequest(r agentio.LaunchRequest) (reflect.Value, error) {
	o, err := optionsFromRequest(agentio.Target{AllowDangerous: true}, r)
	return reflect.ValueOf(o), err
}

// launchConfigured builds the launch with options over the request
// mapping and the settings as -c overrides (before every other flag, as
// the builder renders them). cost: Pure.
func (a *Agent) launchConfigured(t agentio.Target, r agentio.LaunchRequest, c agentio.LaunchConfig) (agentio.Command, error) {
	o, err := optionsFromRequest(t, r)
	if err != nil {
		return agentio.Command{}, err
	}
	keys, err := LaunchOptionKeys()
	if err != nil {
		return agentio.Command{}, err
	}
	if err := agentio.OverlayOptions(c.Options, keys, reflect.ValueOf(&o), r, mapRequest); err != nil {
		return agentio.Command{}, err
	}
	overrides, check, err := checkedOverrides(c.Settings)
	if err != nil {
		return agentio.Command{}, err
	}
	if !check.OK() {
		parts := make([]string, len(check.TypeErrors))
		for i, e := range check.TypeErrors {
			parts[i] = e.Key + ": " + e.Detail
		}
		return agentio.Command{}, fmt.Errorf("%w: settings type errors: %s", agentio.ErrInvalidOption, strings.Join(parts, "; "))
	}
	o.Overrides = overrides
	cmd, err := a.launchOptions(t, r, o)
	if err != nil {
		return agentio.Command{}, err
	}
	for _, u := range check.Unknown {
		detail := u.Detail
		if u.Suggestion != "" {
			detail += suggestionPrefix + u.Suggestion + "?"
		}
		cmd.Warnings.Add(agentio.WarnUnknownKey, "", u.Key, detail)
	}
	return cmd, nil
}

// checkedOverrides flattens settings and checks every override. cost: Pure.
func checkedOverrides(settings agentio.Raw) ([]Override, agentio.SettingsCheck, error) {
	var c agentio.SettingsCheck
	if agentio.IsEmptyJSON(settings) {
		return nil, c, nil
	}
	overrides, err := SettingsOverrides(settings)
	if err != nil {
		return nil, c, err
	}
	known := configKeyPaths()
	for _, ov := range overrides {
		res, err := CheckOverride(ov)
		if err != nil {
			return nil, c, err
		}
		switch res.Status {
		case OverrideTypeError:
			c.TypeErrors = append(c.TypeErrors, agentio.SettingsIssue{Key: ov.Path, Detail: res.Detail})
		case OverrideUnknown:
			c.Unknown = append(c.Unknown, agentio.SettingsIssue{Key: ov.Path, Detail: detailUnknownOverride, Suggestion: agentio.ClosestKey(ov.Path, known)})
		case OverrideRaw:
			c.Unknown = append(c.Unknown, agentio.SettingsIssue{Key: ov.Path, Detail: detailRawOverride})
		case OverrideUnlistedValue:
			c.Unknown = append(c.Unknown, agentio.SettingsIssue{Key: ov.Path, Detail: detailUnlistedPrefix + res.Detail})
		}
		if concern, ok := settingsConcern(ov.Path); ok {
			if c.Concerns == nil {
				c.Concerns = map[string]agentio.Concern{}
			}
			c.Concerns[ov.Path] = concern
		}
	}
	return overrides, c, nil
}

// settingsConcern is the neutral concern of a config path. cost: Pure.
func settingsConcern(path string) (agentio.Concern, bool) {
	switch path {
	case settingsKeyModel:
		return agentio.ConcernModel, true
	case settingsKeySandbox, keyApproval:
		return agentio.ConcernAutonomy, true
	}
	return agentio.ConcernNone, false
}

// configKeyPaths are the typed config paths without wildcard entries, for
// "did you mean". cost: Pure.
func configKeyPaths() []string {
	var out []string
	for _, k := range ConfigKeys() {
		if !strings.Contains(k.Path, "*") {
			out = append(out, k.Path)
		}
	}
	return out
}

// SettingsOverrides flattens a nested settings object into `-c`
// overrides in sorted path order: objects recurse, every other value is a
// leaf. A JSON string, bool, integer, float or array of strings becomes
// its TOML value through the Override constructors; null, an array of
// anything else, or a key that is empty or holds a "." (Codex splits a
// -c path on dots) is an error naming the path. cost: Pure.
func SettingsOverrides(settings agentio.Raw) ([]Override, error) {
	var out []Override
	if err := flattenSettings("", settings, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// flattenSettings appends the overrides of one JSON value at path.
func flattenSettings(path string, v json.RawMessage, out *[]Override) error {
	t := bytes.TrimSpace(v)
	if len(t) > 0 && t[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(t, &obj); err != nil {
			return fmt.Errorf("%w: settings %s: %w", agentio.ErrInvalidOption, orRoot(path), err)
		}
		names := make([]string, 0, len(obj))
		for k := range obj {
			names = append(names, k)
		}
		slices.Sort(names)
		for _, k := range names {
			if k == "" || strings.Contains(k, settingsPathSep) {
				return fmt.Errorf("%w: settings key %q under %s cannot be a -c path segment", agentio.ErrInvalidOption, k, orRoot(path))
			}
			p := k
			if path != "" {
				p = path + settingsPathSep + k
			}
			if err := flattenSettings(p, obj[k], out); err != nil {
				return err
			}
		}
		return nil
	}
	if path == "" {
		return fmt.Errorf("%w: settings must be a JSON object", agentio.ErrInvalidOption)
	}
	ov, err := leafOverride(path, t)
	if err != nil {
		return err
	}
	*out = append(*out, ov)
	return nil
}

// rootPathName names the top level in errors.
const rootPathName = "(top level)"

// orRoot names path, or the top level. Pure.
func orRoot(path string) string {
	if path == "" {
		return rootPathName
	}
	return path
}

// leafOverride renders one JSON scalar or string array as a TOML value.
func leafOverride(path string, t []byte) (Override, error) {
	bad := func(what string) (Override, error) {
		return Override{}, fmt.Errorf("%w: settings %s: %s has no -c form (string, bool, number or array of strings)", agentio.ErrInvalidOption, path, what)
	}
	if len(t) == 0 {
		return bad("an empty value")
	}
	switch {
	case t[0] == '"':
		var s string
		if err := json.Unmarshal(t, &s); err != nil {
			return bad("a malformed string")
		}
		return OverrideString(path, s), nil
	case string(t) == "true" || string(t) == "false":
		return OverrideBool(path, string(t) == "true"), nil
	case t[0] == '[':
		var items []string
		if err := json.Unmarshal(t, &items); err != nil {
			return bad("an array of non-strings")
		}
		return OverrideStrings(path, items), nil
	case t[0] == '-' || (t[0] >= '0' && t[0] <= '9'):
		num := string(t)
		if !strings.ContainsAny(num, ".eE") {
			if i, err := strconv.ParseInt(num, 10, 64); err == nil {
				return OverrideInt(path, i), nil
			}
		}
		f, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return bad("a number out of range")
		}
		return OverrideFloat(path, f), nil
	}
	return bad("null")
}
