package gemini

import (
	"context"
	"strings"

	"github.com/ubgo/agentio"
)

// Gemini-specific doctor check IDs (open set beside the neutral ones).
const (
	// CheckSettingsParse reports a settings.json that does not decode, or
	// keys kept raw because their type differs from the schema.
	CheckSettingsParse agentio.CheckID = "settings"
	// CheckAutoUpdate notes that the CLI updates itself (general.
	// enableAutoUpdate defaults to true for npm, pnpm, yarn, bun and volta
	// installs), so the installed version can move under a caller.
	CheckAutoUpdate agentio.CheckID = "auto-update"
)

// Checks runs the doctor for h: binary, home, home env, API billing env
// (shared), installed version against the tested range, the user settings
// parse, the login, and auto-update. Never runs the binary.
// cost: Disk · Read.
func (a *Agent) Checks(ctx context.Context, h Home) []agentio.Check {
	out := a.core.Checks(ctx, h.Neutral())
	in, err := a.Install()
	if err != nil {
		in = agentio.Install{}
	}
	out = append(out, agentio.VersionCheck(Compat(), in.Version))
	user, err := ReadSettingsFile(agentio.ConfigUser, h.UserSettingsPath())
	switch {
	case err != nil:
		out = append(out, agentio.Check{ID: CheckSettingsParse, Status: agentio.CheckFail, Message: err.Error()})
	case len(user.Warnings) > 0:
		out = append(out, agentio.Check{ID: CheckSettingsParse, Status: agentio.CheckWarn, Message: strings.Join(user.Warnings, "; ")})
	case user.Exists:
		out = append(out, agentio.Check{ID: CheckSettingsParse, Status: agentio.CheckOK, Message: user.Path})
	}
	out = append(out, a.loginCheck(h))
	if g := user.Settings.General; g == nil || g.EnableAutoUpdate == nil || *g.EnableAutoUpdate {
		out = append(out, agentio.Check{ID: CheckAutoUpdate, Status: agentio.CheckInfo, Message: "general.enableAutoUpdate is on: Gemini CLI updates its own npm-family install"})
	}
	return out
}

// loginCheck reports the login without the network. cost: Disk · Read.
func (a *Agent) loginCheck(h Home) agentio.Check {
	id, err := a.Identity(h)
	switch {
	case err != nil:
		return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckFail, Message: err.Error()}
	case id.LoggedIn:
		return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckOK, Message: "auth " + string(id.AuthType) + " (" + id.Credential.String() + ")"}
	case id.AuthType == "":
		return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckWarn, Message: "no auth method chosen (security.auth.selectedType unset); the first interactive run asks"}
	}
	return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckWarn, Message: "auth " + string(id.AuthType) + ": no login found on disk or in the environment"}
}
