package opencode

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// Launch builds the opencode command for r on t:
//   - interactive (the TUI): Prompt is the initial prompt (--prompt),
//     ResumeID continues a session (--session <id>), Model is
//     "provider/model" (--model);
//   - Headless runs `opencode run --format json` (raw JSON events) with
//     the same --session and --model, and the prompt as the message
//     (after "--" when it starts with '-'); without a Prompt opencode
//     reads the message from stdin;
//   - Autonomy: full adds --auto (needs AllowDangerous); read-only, ask
//     and accept-edits are refused, since opencode sets them only through
//     permission rules in its config; AutonomyDefault adds nothing;
//   - AddDirs and token logins are refused: opencode has no flag for extra
//     dirs, and its provider logins live in auth.json;
//   - the default home leaves XDG_DATA_HOME as inherited; another home
//     must be named <base>/opencode and sets XDG_DATA_HOME=<base>, which
//     every XDG-aware program the session starts sees too.
//
// With t.Version known, a flag the installed version provably lacks is a
// FeatureUnavailableError. cost: Pure.
func (a *Agent) Launch(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	if err := a.checkLaunch(t, r); err != nil {
		return agentio.Command{}, err
	}
	t, err := a.homeTarget(t)
	if err != nil {
		return agentio.Command{}, err
	}
	if !r.Headless {
		return a.core.Launch(t, r)
	}
	if err := agentio.CheckAutonomy(t, r.Autonomy); err != nil {
		return agentio.Command{}, err
	}
	args := append([]string{cmdRun, flagFormat, formatJSON}, t.ExtraArgs...)
	args = append(args, autonomyArgs()[r.Autonomy]...)
	if r.ResumeID != "" {
		args = append(args, flagSession, r.ResumeID)
	}
	if r.Model != "" {
		args = append(args, flagModel, r.Model)
	}
	args = append(args, r.Args...)
	if r.Prompt != "" {
		if strings.HasPrefix(r.Prompt, "-") {
			args = append(args, argEnd)
		}
		args = append(args, r.Prompt)
	}
	return a.core.Command(t, r.Dir, args, nil)
}

// checkLaunch refuses what opencode cannot express and checks every flag
// r needs against t.Version. cost: Pure.
func (a *Agent) checkLaunch(t agentio.Target, r agentio.LaunchRequest) error {
	if err := agentio.CheckOwner(ID, t.Home); err != nil {
		return err
	}
	switch {
	case r.Credential != nil:
		return agentio.UnsupportedError(ID, "token login", "opencode keeps provider logins in auth.json (opencode providers login); a per-provider token variable is not modelled")
	case len(r.AddDirs) > 0:
		return agentio.UnsupportedError(ID, "add-dir", "opencode has no flag for extra directories")
	case r.Autonomy != agentio.AutonomyDefault && r.Autonomy != agentio.AutonomyFull:
		return agentio.UnsupportedError(ID, "autonomy "+r.Autonomy.String(), "opencode has no flag for this level; only --auto (full) exists, other levels are permission rules in opencode.json")
	}
	var feats []Feature
	if r.Autonomy == agentio.AutonomyFull {
		feats = append(feats, FeatureFlagAuto)
	}
	if r.ResumeID != "" {
		feats = append(feats, FeatureFlagSession)
	}
	if r.Model != "" {
		feats = append(feats, FeatureFlagModel)
	}
	if r.Headless {
		feats = append(feats, FeatureRunFormatJSON)
	} else if r.Prompt != "" {
		feats = append(feats, FeatureFlagPrompt)
	}
	for _, f := range feats {
		if err := checkFeature(t.Version, f); err != nil {
			return err
		}
	}
	return nil
}

// homeTarget adds XDG_DATA_HOME for a non-default home: opencode reads
// its data dir only from it. A home not named <base>/opencode cannot be
// selected. cost: Pure.
func (a *Agent) homeTarget(t agentio.Target) (agentio.Target, error) {
	if t.Home.IsDefault() {
		return t, nil
	}
	dir := t.Home.Dir()
	if filepath.Base(dir) != appDir {
		return t, fmt.Errorf("%w: opencode: %s selects <dir>/%s, so the home %s cannot be launched", agentio.ErrUnsupported, XDGDataHomeEnv, appDir, dir)
	}
	t.ExtraEnv = append(slices.Clone(t.ExtraEnv), agentio.EnvSet(XDGDataHomeEnv, filepath.Dir(dir)))
	return t, nil
}
