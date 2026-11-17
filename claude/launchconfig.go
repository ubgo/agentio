package claude

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// Launch configuration through the neutral Agent (agentio.LaunchConfigOf):
// typed options from the registry, applied over the neutral request's
// mapping, and a settings object type-checked by ValidateSettings and
// delivered by LaunchSettings as --settings.

// optionConcerns names the neutral concern of each option key that
// overlaps a LaunchRequest field. allow_dangerously_skip_permissions is
// left out on purpose: it only offers the mode inside the session, it
// does not set the permission level.
func optionConcerns() map[string]agentio.Concern {
	return map[string]agentio.Concern{
		"model":                        agentio.ConcernModel,
		"prompt":                       agentio.ConcernPrompt,
		"add_dirs":                     agentio.ConcernAddDir,
		"permission_mode":              agentio.ConcernAutonomy,
		"dangerously_skip_permissions": agentio.ConcernAutonomy,
		"resume":                       agentio.ConcernResume,
		"continue":                     agentio.ConcernResume,
		"print":                        agentio.ConcernHeadless,
		"output_format":                agentio.ConcernHeadless,
		"settings":                     agentio.ConcernSettings,
	}
}

// Settings key paths that overlap a neutral concern (CheckSettings).
const (
	settingsKeyModel       = "model"
	settingsKeyDefaultMode = keyPermissions + ".defaultMode"
	settingsPathSep        = "."
)

// Wording of launch-config findings and conflicts.
const (
	unknownSettingDetail = "not modelled by agentio; passed to Claude unchecked"
	suggestionPrefix     = "; did you mean "
	settingsFlagEq       = flagSettings + "="
	whyHelperLogin       = "the request's api-key-helper login (LaunchRequest.Credential)"
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

// CheckSettings type-checks a settings object with ValidateSettings and
// names the keys that overlap a neutral concern: model, the default
// permission mode, apiKeyHelper and env credential variables. cost: Pure.
func (n neutral) CheckSettings(settings agentio.Raw) (agentio.SettingsCheck, error) {
	return checkSettings(settings)
}

// checkSettings is CheckSettings without the agent: it reads no state.
// cost: Pure.
func checkSettings(settings agentio.Raw) (agentio.SettingsCheck, error) {
	if agentio.IsEmptyJSON(settings) {
		return agentio.SettingsCheck{}, nil
	}
	v, err := ValidateSettings(settings)
	if err != nil {
		return agentio.SettingsCheck{}, err
	}
	known := agentio.SchemaPaths(SettingsSchema())
	var c agentio.SettingsCheck
	for _, w := range v.TypeErrors {
		c.TypeErrors = append(c.TypeErrors, agentio.SettingsIssue{Key: w.Key, Detail: w.Detail})
	}
	for _, k := range v.UnknownKeys {
		c.Unknown = append(c.Unknown, agentio.SettingsIssue{Key: k, Detail: unknownSettingDetail, Suggestion: agentio.ClosestKey(k, known)})
	}
	c.Concerns, err = settingsConcerns(settings)
	return c, err
}

// settingsConcerns maps the given key paths that overlap a neutral
// concern. cost: Pure.
func settingsConcerns(settings agentio.Raw) (map[string]agentio.Concern, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(settings, &top); err != nil {
		return nil, fmt.Errorf("claude: settings: %w", err)
	}
	out := map[string]agentio.Concern{}
	if _, ok := top[settingsKeyModel]; ok {
		out[settingsKeyModel] = agentio.ConcernModel
	}
	if _, ok := top[keyAPIKeyHelper]; ok {
		out[keyAPIKeyHelper] = agentio.ConcernCredential
	}
	var perms map[string]json.RawMessage
	if json.Unmarshal(top[keyPermissions], &perms) == nil {
		if _, ok := perms["defaultMode"]; ok {
			out[settingsKeyDefaultMode] = agentio.ConcernAutonomy
		}
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(top[keyEnv], &env) == nil {
		for _, name := range credentialEnv() {
			if _, ok := env[name]; ok {
				out[keyEnv+settingsPathSep+name] = agentio.ConcernCredential
			}
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// RawSettingsArg reports a raw --settings (or --settings=…) among args.
// cost: Pure.
func (n neutral) RawSettingsArg(args []string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		return a == flagSettings || strings.HasPrefix(a, settingsFlagEq)
	})
}

// LaunchConfigured is Launch with c applied. cost: Pure; Disk · Write
// when c.Delivery is SettingsFile and settings are delivered.
func (n neutral) LaunchConfigured(t agentio.Target, r agentio.LaunchRequest, c agentio.LaunchConfig) (agentio.Command, error) {
	return n.a.launchConfigured(t, r, c)
}

// requestOptions is the neutral half every launch shares: the autonomy
// rule, the request mapping and the credential env. cost: Pure.
func (a *Agent) requestOptions(t agentio.Target, r agentio.LaunchRequest) (LaunchOptions, []agentio.EnvChange, error) {
	if err := agentio.CheckAutonomy(t, r.Autonomy); err != nil {
		return LaunchOptions{}, nil, err
	}
	o, err := launchFromRequest(r)
	if err != nil {
		return LaunchOptions{}, nil, err
	}
	var tokenEnv []agentio.EnvChange
	if r.Credential != nil {
		if tokenEnv, err = a.TokenEnv(*r.Credential); err != nil {
			return LaunchOptions{}, nil, err
		}
	}
	return o, tokenEnv, nil
}

// mapRequest is launchFromRequest as an agentio.RequestMapper. cost: Pure.
func mapRequest(r agentio.LaunchRequest) (reflect.Value, error) {
	o, err := launchFromRequest(r)
	return reflect.ValueOf(o), err
}

// launchConfigured builds the launch with options over the request
// mapping and settings delivered. Every check (options, conflicts,
// settings types, flags and version gates) runs before the settings file
// is written, so a refused launch leaves no file. cost: Pure; Disk ·
// Write for a settings file.
func (a *Agent) launchConfigured(t agentio.Target, r agentio.LaunchRequest, c agentio.LaunchConfig) (agentio.Command, error) {
	o, tokenEnv, err := a.requestOptions(t, r)
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
	settings := c.Settings
	if r.Credential != nil && r.Credential.Kind == agentio.TokenAPIKeyHelper {
		helper, err := HelperSettings(*r.Credential)
		if err != nil {
			return agentio.Command{}, err
		}
		if settings, err = agentio.MergeSettings(settings, helper, whyHelperLogin); err != nil {
			return agentio.Command{}, err
		}
	}
	if agentio.IsEmptyJSON(settings) {
		return a.launchAt(t, o, tokenEnv, r.Dir)
	}
	if o.Settings != "" {
		return agentio.Command{}, fmt.Errorf("%w: option %q conflicts with LaunchConfig.Settings", agentio.ErrInvalidOption, "settings")
	}
	warns, err := settingsWarnings(settings)
	if err != nil {
		return agentio.Command{}, err
	}
	// Build once with the settings inline: it runs every flag and version
	// check, so nothing is written for a launch that would be refused.
	o.Settings = string(settings)
	if _, err := a.launchAt(t, o, tokenEnv, r.Dir); err != nil {
		return agentio.Command{}, err
	}
	if o.Settings, err = deliverSettings(settings, c); err != nil {
		return agentio.Command{}, err
	}
	cmd, err := a.launchAt(t, o, tokenEnv, r.Dir)
	if err != nil {
		return agentio.Command{}, err
	}
	cmd.Warnings = append(cmd.Warnings, warns...)
	return cmd, nil
}

// launchAt is launch with the request's working dir. cost: Pure.
func (a *Agent) launchAt(t agentio.Target, o LaunchOptions, tokenEnv []agentio.EnvChange, dir string) (agentio.Command, error) {
	cmd, err := a.launch(t, o, tokenEnv)
	if err != nil {
		return agentio.Command{}, err
	}
	cmd.Dir = dir
	return cmd, nil
}

// settingsWarnings refuses settings with type errors and turns unknown
// keys into warnings. cost: Pure.
func settingsWarnings(settings agentio.Raw) (agentio.Warnings, error) {
	check, err := checkSettings(settings)
	if err != nil {
		return nil, err
	}
	if !check.OK() {
		parts := make([]string, len(check.TypeErrors))
		for i, e := range check.TypeErrors {
			parts[i] = e.Key + ": " + e.Detail
		}
		return nil, fmt.Errorf("%w: settings type errors: %s", agentio.ErrInvalidOption, strings.Join(parts, "; "))
	}
	var warns agentio.Warnings
	for _, u := range check.Unknown {
		detail := u.Detail
		if u.Suggestion != "" {
			detail += suggestionPrefix + u.Suggestion + "?"
		}
		warns.Add(agentio.WarnUnknownKey, "", u.Key, detail)
	}
	return warns, nil
}

// deliverSettings writes the settings file or returns the inline JSON,
// per c.Delivery. cost: Disk · Write for a file; Pure inline.
func deliverSettings(settings agentio.Raw, c agentio.LaunchConfig) (string, error) {
	switch c.Delivery {
	case agentio.SettingsInline:
		return LaunchSettings(settings, SettingsViaInline, "", "")
	case agentio.SettingsFile:
		if c.RunDir == "" {
			return "", fmt.Errorf("%w: settings file delivery needs LaunchConfig.RunDir", agentio.ErrInvalidOption)
		}
		return LaunchSettings(settings, SettingsViaFile, c.RunDir, c.RunID)
	}
	return "", fmt.Errorf("%w: settings delivery %s", agentio.ErrInvalidOption, c.Delivery)
}
