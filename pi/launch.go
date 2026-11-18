package pi

import (
	"fmt"
	"slices"

	"github.com/ubgo/agentio"
)

// LaunchOptions are pi's launch flags (PKG/docs/cli.md, pi --help 1.5.2).
// Zero values emit nothing; flags not modelled go through ExtraArgs.
type LaunchOptions struct {
	// Prompt is the initial message, passed after "--" so a leading "-" is
	// never a flag. Without Print or a json/rpc Mode pi stays interactive
	// after answering it (pi --help: "Interactive mode with initial
	// prompt").
	Prompt string
	// Print is -p: process the prompt, print the answer and exit.
	Print bool
	// Mode is --mode; ModeJSON writes JSONL events and exits.
	Mode Mode
	// Model is --model: an id, a fuzzy pattern, "provider/id" and an
	// optional ":<thinking>" suffix.
	Model string
	// Provider is --provider; pi requires --model with it.
	Provider string
	// Thinking is --thinking.
	Thinking ThinkingLevel
	// Tools is the --tools allowlist; ExcludeTools the --exclude-tools
	// denylist (applied last, MCP tools included).
	Tools, ExcludeTools []string
	// NoMCP, NoExtensions and NoContextFiles turn those off for this run.
	NoMCP, NoExtensions, NoContextFiles bool
	// Session is --session: a session file path or a full or partial id.
	// An id found only under another cwd makes pi ask whether to fork it,
	// so pass the path (Session.Path), or run in the session's cwd.
	Session string
	// SessionID is --session-id: open or create the cwd's session with
	// exactly this id.
	SessionID string
	// Continue is --continue (the newest session of the cwd by mtime).
	Continue bool
	// Fork is --fork: copy a session (path or id) into the cwd.
	Fork string
	// NoSession is --no-session (in memory only).
	NoSession bool
	// Name is --name, the session's display name.
	Name string
	// SystemPrompt replaces the system prompt; AppendSystemPrompt entries
	// each add --append-system-prompt (text or a file path).
	SystemPrompt       string
	AppendSystemPrompt []string
	// Approve decides project trust for this run: true --approve, false
	// --no-approve, nil asks (or skips project files without a terminal).
	Approve *bool
	// Offline is --offline: no startup network activity.
	Offline bool
	// ExtraArgs are appended before the prompt, verbatim.
	ExtraArgs []string
}

// ErrInvalidLaunch means a LaunchOptions combination pi rejects. It also
// matches agentio.ErrInvalidOption.
var ErrInvalidLaunch = agentio.NewLaunchSentinel("pi: invalid launch options")

// validate rejects the combinations pi refuses (main.js:274-309,
// PKG/docs/cli.md:105-109).
func (o LaunchOptions) validate() error {
	switch {
	case o.Provider != "" && o.Model == "":
		return fmt.Errorf("%w: --provider needs --model", ErrInvalidLaunch)
	case o.Fork != "" && (o.Session != "" || o.Continue || o.NoSession):
		return fmt.Errorf("%w: --fork cannot be combined with --session, --continue or --no-session", ErrInvalidLaunch)
	case o.SessionID != "" && (o.Session != "" || o.Continue):
		return fmt.Errorf("%w: --session-id cannot be combined with --session or --continue", ErrInvalidLaunch)
	case o.Session != "" && o.Continue:
		return fmt.Errorf("%w: --session and --continue are exclusive", ErrInvalidLaunch)
	case o.Mode != "" && !slices.Contains(ModeValues(), o.Mode):
		return fmt.Errorf("%w: unknown mode %q", ErrInvalidLaunch, o.Mode)
	case o.Thinking != "" && !slices.Contains(ThinkingLevelValues(), o.Thinking):
		return fmt.Errorf("%w: unknown thinking level %q", ErrInvalidLaunch, o.Thinking)
	}
	return nil
}

// argBuilder collects argv and the features each flag needs.
type argBuilder struct {
	args  []string
	feats []Feature
}

func (b *argBuilder) flag(f Feature, args ...string) {
	b.args = append(b.args, args...)
	b.feats = append(b.feats, f)
}

func (b *argBuilder) str(f Feature, flag, v string) {
	if v != "" {
		b.flag(f, flag, v)
	}
}

func (b *argBuilder) on(f Feature, flag string, v bool) {
	if v {
		b.flag(f, flag)
	}
}

// build renders the argv (prompt excluded) and the features it needs.
func (o LaunchOptions) build() (argBuilder, error) {
	var b argBuilder
	if err := o.validate(); err != nil {
		return b, err
	}
	b.on(FeatureFlagPrint, flagPrint, o.Print)
	if o.Mode != "" {
		b.flag(FeatureFlagModeJSON, flagMode, string(o.Mode))
	}
	b.str(FeatureFlagProvider, flagProvider, o.Provider)
	b.str(FeatureFlagModel, flagModel, o.Model)
	b.str(FeatureFlagThinking, flagThinking, string(o.Thinking))
	if len(o.Tools) > 0 {
		b.flag(FeatureFlagTools, flagTools, joinList(o.Tools))
	}
	if len(o.ExcludeTools) > 0 {
		b.flag(FeatureFlagExcludeTools, flagExcludeTools, joinList(o.ExcludeTools))
	}
	b.on(FeatureFlagNoMCP, flagNoMCP, o.NoMCP)
	b.on(FeatureFlagNoExtensions, flagNoExtensions, o.NoExtensions)
	b.on(FeatureFlagNoContextFiles, flagNoContextFiles, o.NoContextFiles)
	b.str(FeatureFlagSession, flagSession, o.Session)
	b.str(FeatureFlagSessionID, flagSessionID, o.SessionID)
	b.on(FeatureFlagContinue, flagContinue, o.Continue)
	b.str(FeatureFlagFork, flagFork, o.Fork)
	b.on(FeatureFlagNoSession, flagNoSession, o.NoSession)
	b.str(FeatureFlagName, flagName, o.Name)
	b.str(FeatureFlagSystemPrompt, flagSystemPrompt, o.SystemPrompt)
	for _, s := range o.AppendSystemPrompt {
		b.flag(FeatureFlagAppendSystemPrompt, flagAppendSystemPrompt, s)
	}
	if o.Approve != nil {
		f := flagNoApprove
		if *o.Approve {
			f = flagApprove
		}
		b.flag(FeatureFlagApprove, f)
	}
	b.on(FeatureFlagOffline, flagOffline, o.Offline)
	b.args = append(b.args, o.ExtraArgs...)
	return b, nil
}

// Args renders o as pi's argv for an installed version v (zero = unknown,
// no check): a flag v provably lacks is a FeatureUnavailableError. Pure.
func (o LaunchOptions) Args(v agentio.Version) ([]string, error) {
	b, err := o.build()
	if err != nil {
		return nil, err
	}
	for _, f := range b.feats {
		if err := checkFeature(v, f); err != nil {
			return nil, err
		}
	}
	args := b.args
	if o.Prompt != "" {
		args = append(args, endOfOptions, o.Prompt)
	}
	return args, nil
}

// Launch builds the command that starts pi on t with o: t.ExtraArgs first,
// then o's flags, then the prompt; the home rule sets PI_CODING_AGENT_DIR
// for a non-default home and removes it for the default. tokenEnv nil =
// the home's own login. Pure.
func (a *Agent) Launch(t agentio.Target, dir string, o LaunchOptions, tokenEnv []agentio.EnvChange) (agentio.Command, error) {
	args, err := o.Args(t.Version)
	if err != nil {
		return agentio.Command{}, err
	}
	argv := append(slices.Clone(t.ExtraArgs), args...)
	return a.core.Command(t, dir, argv, tokenEnv)
}

// launchRequest maps the neutral request onto LaunchOptions:
//   - autonomy: read-only is the documented read-only tool allowlist with
//     MCP and extensions off; full adds nothing (pi never asks; it still
//     needs Target.AllowDangerous); ask and accept-edits are refused;
//   - Headless is --mode json; ResumeID is --session (an id or a path);
//   - AddDirs are refused: pi has no such flag, its tools reach any path
//     the process can.
//
// Pure.
func (a *Agent) launchRequest(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	if err := agentio.CheckAutonomy(t, r.Autonomy); err != nil {
		return agentio.Command{}, err
	}
	o := LaunchOptions{Prompt: r.Prompt, Model: r.Model, Session: r.ResumeID, ExtraArgs: r.Args}
	switch r.Autonomy {
	case agentio.AutonomyDefault, agentio.AutonomyFull:
	case agentio.AutonomyReadOnly:
		o.Tools, o.NoMCP, o.NoExtensions = readOnlyTools(), true, true
	default:
		return agentio.Command{}, agentio.UnsupportedError(ID, "autonomy "+r.Autonomy.String(), noPermissionModesReason)
	}
	if r.Headless {
		o.Mode = ModeJSON
	}
	if len(r.AddDirs) > 0 {
		return agentio.Command{}, agentio.UnsupportedError(ID, "add-dir", "pi has no extra-directory flag; its tools reach any path the process can")
	}
	var tokenEnv []agentio.EnvChange
	if r.Credential != nil {
		env, err := a.TokenEnv(*r.Credential)
		if err != nil {
			return agentio.Command{}, err
		}
		tokenEnv = env
	}
	return a.Launch(t, r.Dir, o, tokenEnv)
}

// TokenKinds lists the token logins pi takes: a provider API key. Pure.
func (a *Agent) TokenKinds() []agentio.TokenKind { return []agentio.TokenKind{agentio.TokenAPIKey} }

// DetectTokenKind cannot tell a key's provider or kind from its shape
// across pi's providers, so it never guesses. Pure.
func (a *Agent) DetectTokenKind(agentio.Token) (agentio.TokenKind, bool) {
	return agentio.TokenKindUnknown, false
}

// TokenEnv sets the provider's key variable (TokenCredential.Provider
// picks it; required, because every provider has its own). Nothing is
// scrubbed: another provider's key never overrides this one. Pure.
func (a *Agent) TokenEnv(c agentio.TokenCredential) ([]agentio.EnvChange, error) {
	if c.Kind != agentio.TokenAPIKey {
		return nil, agentio.UnsupportedError(ID, "token login "+c.Kind.String(), "pi takes provider API keys only; OAuth logins are made with /login inside pi")
	}
	for _, e := range tokenEnv() {
		if e.Provider == c.Provider {
			return []agentio.EnvChange{agentio.EnvSetSecret(e.Env, c.Token)}, nil
		}
	}
	if c.Provider == "" {
		return nil, agentio.UnsupportedError(ID, "token login without a provider", "pi reads a different variable per provider; set TokenCredential.Provider")
	}
	return nil, agentio.UnsupportedError(ID, "token login for provider "+c.Provider, "no single key variable is recorded for that provider")
}

// TokenSetupCommand: pi has no token setup command (keys go in env or
// auth.json via /login). Pure.
func (a *Agent) TokenSetupCommand(Home) (agentio.Command, bool) { return agentio.Command{}, false }
