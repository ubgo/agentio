package gemini

import (
	"slices"

	"github.com/ubgo/agentio"
)

// Launch builds the gemini command for r on t from Spec() (agentio.Core):
//   - Prompt is an initial prompt in an interactive session
//     (--prompt-interactive); with Headless it is the headless prompt
//     (--prompt), which is what makes the run non-interactive;
//   - Headless adds --output-format stream-json; without a Prompt the CLI
//     reads the prompt from stdin (a non-TTY stdin is headless too);
//   - Autonomy maps onto --approval-mode (read-only plan, ask default,
//     accept-edits auto_edit, full yolo; full needs AllowDangerous);
//     AutonomyDefault adds nothing, so general.defaultApprovalMode applies;
//   - ResumeID is a full session id (--resume <uuid>). Resume is per
//     project: set Dir to the session's project dir;
//   - a token credential is an API key: GEMINI_API_KEY, or GOOGLE_API_KEY
//     for provider "vertex";
//   - a non-default home sets GEMINI_CLI_HOME to the home's PARENT; the
//     default home removes it.
//
// With t.Version known, a flag the installed version provably lacks is a
// FeatureUnavailableError. cost: Pure.
func (a *Agent) Launch(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	if err := a.checkLaunchFeatures(t.Version, r); err != nil {
		return agentio.Command{}, err
	}
	if r.Headless && r.Prompt != "" {
		r.Args = append(slices.Clone(r.Args), flagPrompt, r.Prompt)
		r.Prompt = ""
	}
	return a.core.Launch(t, r)
}

// checkLaunchFeatures checks every flag r needs against v. cost: Pure.
func (a *Agent) checkLaunchFeatures(v agentio.Version, r agentio.LaunchRequest) error {
	var feats []Feature
	if r.Autonomy != agentio.AutonomyDefault {
		feats = append(feats, FeatureFlagApprovalMode)
	}
	if r.Prompt != "" && !r.Headless {
		feats = append(feats, FeatureFlagPromptInteractive)
	}
	if len(r.AddDirs) > 0 {
		feats = append(feats, FeatureFlagIncludeDirectories)
	}
	if r.Headless {
		feats = append(feats, FeatureFlagOutputFormatStreamJSON)
	}
	if r.ResumeID != "" {
		feats = append(feats, FeatureFlagResumeUUID)
	}
	for _, f := range feats {
		if err := checkFeature(v, f); err != nil {
			return err
		}
	}
	return nil
}
