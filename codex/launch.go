package codex

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/ubgo/agentio"
)

// Subcommands and flags, verbatim from `codex --help` and each subcommand's
// `--help` at 0.162.1 (CODEX_API.md §16).
const (
	subResume     = "resume"
	subFork       = "fork"
	subExec       = "exec"
	subReview     = "review"
	subArchive    = "archive"
	subUnarchive  = "unarchive"
	subDelete     = "delete"
	subQueue      = "queue"
	subDoctor     = "doctor"
	subMCPServer  = "mcp-server"
	subAppServer  = "app-server"
	subFeatures   = "features"
	subEnable     = "enable"
	subDisable    = "disable"
	subMCP        = "mcp"
	subAdd        = "add"
	subRemove     = "remove"
	flagVersion   = "--version"
	flagConfig    = "-c"
	flagEnable    = "--enable"
	flagDisable   = "--disable"
	flagStrict    = "--strict-config"
	flagImage     = "-i"
	flagModel     = "-m"
	flagOSS       = "--oss"
	flagLocalProv = "--local-provider"
	flagProfile   = "-p"
	flagSandbox   = "-s"
	flagApproval  = "-a"
	flagApprove   = "--approve-for-me"
	flagBypass    = "--dangerously-bypass-approvals-and-sandbox"
	flagCd        = "-C"
	flagWorktree  = "--worktree"
	flagAddDir    = "--add-dir"
	flagSearch    = "--search"
	flagNoAlt     = "--no-alt-screen"
	flagNoDaemon  = "--no-daemon"
	flagRemote    = "--remote"
	flagRemoteEnv = "--remote-auth-token-env"
	flagLast      = "--last"
	flagAll       = "--all"
	flagOutLast   = "-o"
	flagOutSchema = "--output-schema"
	flagColor     = "--color"
	flagSkipGit   = "--skip-git-repo-check"
	flagEphemeral = "--ephemeral"
	flagIgnoreCfg = "--ignore-user-config"
	flagIgnoreRul = "--ignore-rules"
	flagThreadSrc = "--thread-source"
	flagCyber     = "--cyber-access-program"
	flagUncommit  = "--uncommitted"
	flagBase      = "--base"
	flagCommit    = "--commit"
	flagTitle     = "--title"
	flagForce     = "--force"
	flagThread    = "--thread"
	flagMessage   = "--message"
	flagListen    = "--listen"
	flagURL       = "--url"
	flagBearerEnv = "--bearer-token-env-var"
	flagArgsSep   = "--"
	stdinPrompt   = "-"
	overrideSep   = "="
	keyApproval   = "approval_policy"
)

// Version gates for profile semantics (§0.1).
const (
	profileV2Since   = "0.131.0"
	profileV2Certain = "0.162.1"
)

// Override is one `-c path=value`. Value is TOML source text; build it
// with the typed constructors so quoting is the library's job.
type Override struct {
	Path  string
	Value TOMLRaw
}

// OverrideString renders a TOML string value. Pure.
func OverrideString(path, v string) Override {
	return Override{Path: path, Value: TOMLRaw(strconv.Quote(v))}
}

// OverrideBool renders a TOML boolean. Pure.
func OverrideBool(path string, v bool) Override {
	return Override{Path: path, Value: TOMLRaw(strconv.FormatBool(v))}
}

// OverrideInt renders a TOML integer. Pure.
func OverrideInt(path string, v int64) Override {
	return Override{Path: path, Value: TOMLRaw(strconv.FormatInt(v, 10))}
}

// OverrideStrings renders a TOML array of strings. Pure.
func OverrideStrings(path string, vs []string) Override {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = strconv.Quote(v)
	}
	return Override{Path: path, Value: TOMLRaw("[" + strings.Join(q, ", ") + "]")}
}

// TOML spellings Go's float formatting does not produce (Go prints -Inf
// and drops the decimal point of integral values).
const (
	tomlNegInf   = "-" + tomlInf
	tomlFloatDot = ".0"
)

// OverrideFloat renders a TOML float, always with a decimal point or an
// exponent so TOML never reads it as an integer. Pure.
func OverrideFloat(path string, v float64) Override {
	var s string
	switch {
	case math.IsNaN(v):
		s = tomlNaN
	case math.IsInf(v, 1):
		s = tomlInf
	case math.IsInf(v, -1):
		s = tomlNegInf
	default:
		s = strconv.FormatFloat(v, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += tomlFloatDot
		}
	}
	return Override{Path: path, Value: TOMLRaw(s)}
}

// ErrBadOverride means a `-c` text has no `=`. Like every Codex launch
// refusal below, it also matches agentio.ErrInvalidOption.
var ErrBadOverride = agentio.NewLaunchSentinel("codex: override must be path=value")

// ParseOverride parses "path=value" exactly like Codex: the value is TOML
// when it parses as TOML, else a literal string (then quoted here). Pure.
func ParseOverride(s string) (Override, error) {
	path, val, ok := strings.Cut(s, overrideSep)
	path = strings.TrimSpace(path)
	if !ok || path == "" {
		return Override{}, fmt.Errorf("%w: %q", ErrBadOverride, s)
	}
	val = strings.TrimSpace(val)
	if _, err := parseTOML([]byte("v = " + val)); err == nil && val != "" {
		return Override{Path: path, Value: TOMLRaw(val)}, nil
	}
	return OverrideString(path, val), nil
}

// String renders "path=value" for the -c argument. Pure.
func (o Override) String() string { return o.Path + overrideSep + string(o.Value) }

// LaunchOptions are the global/TUI flags of `codex` (§16). A field tagged
// agentio:"path" takes a file or folder on this machine
// (agentio.OptionKey.Path): a launcher expands "~" in those values only.
type LaunchOptions struct {
	Prompt             string         `json:"prompt,omitempty"`            // positional; "" = interactive
	PromptFromStdin    bool           `json:"prompt_from_stdin,omitempty"` // exec only: "-"
	Model              string         `json:"model,omitempty"`
	Profile            string         `json:"codex_profile,omitempty"` // v2 profile name (-p)
	Sandbox            SandboxMode    `json:"sandbox,omitempty"`
	Approval           ApprovalPolicy `json:"approval,omitempty"` // TUI only; exec renders -c approval_policy
	ApproveForMe       bool           `json:"approve_for_me,omitempty"`
	BypassAll          bool           `json:"bypass_all,omitempty"` // requires Target.AllowDangerous
	Dir                string         `json:"cd,omitempty" agentio:"path"`
	AddDirs            []string       `json:"add_dirs,omitempty" agentio:"path"`
	Images             []string       `json:"images,omitempty" agentio:"path"`
	Search             bool           `json:"search,omitempty"`
	OSS                bool           `json:"oss,omitempty"`
	LocalProvider      string         `json:"local_provider,omitempty"`
	Overrides          []Override     `json:"-"`
	Enable             []string       `json:"enable,omitempty"`
	Disable            []string       `json:"disable,omitempty"`
	StrictConfig       bool           `json:"strict_config,omitempty"`
	Worktree           bool           `json:"worktree,omitempty"`
	NoAltScreen        bool           `json:"no_alt_screen,omitempty"`
	NoDaemon           bool           `json:"no_daemon,omitempty"`
	Remote             string         `json:"remote,omitempty"`
	RemoteAuthTokenEnv string         `json:"remote_auth_token_env,omitempty"` // a variable NAME, never a token
	// ProfileCheck is the home's config.toml (ConfigFile) used to refuse a
	// --profile that collides with a legacy profile before Codex does
	// (Codex 0.162.1 refuses -p NAME while a legacy [profiles.NAME] exists).
	// nil skips the check and adds a warning: building stays Pure.
	ProfileCheck *Config `json:"-"`
	// trailing are LaunchRequest.Args from the neutral launch: appended
	// last, verbatim (SPEC §5.4).
	trailing []string
}

// ExecOptions add the `codex exec` flags.
type ExecOptions struct {
	LaunchOptions
	JSON               bool
	OutputLastMessage  string
	OutputSchema       string
	Color              string // always | never | auto
	SkipGitRepoCheck   bool
	Ephemeral          bool
	IgnoreUserConfig   bool
	IgnoreRules        bool
	ThreadSource       string
	CyberAccessProgram string
}

// ReviewTarget selects what `codex exec review` reviews.
type ReviewTarget struct {
	Uncommitted bool
	Base        string // branch
	Commit      string // sha
	Title       string
}

// ErrInvalidOption means an option value Codex would reject.
var ErrInvalidOption = agentio.NewLaunchSentinel("codex: invalid launch option")

// builder accumulates argv, warnings and the first error.
type builder struct {
	t        agentio.Target
	trailing []string
	args     []string
	warns    agentio.Warnings
	err      error
	// feats are the feature gates the emitted flags checked, for the
	// launch option registry (LaunchOptionKeys).
	feats []Feature
}

func newBuilder(t agentio.Target, sub ...string) *builder {
	b := &builder{t: t, args: slices.Clone(sub)}
	if t.Version.IsZero() {
		b.warns.Add(agentio.WarnVersionUnknown, "", "", "installed codex version unknown; flags not checked against it")
	} else if w, ok := Compat().Warning(t.Version); ok {
		b.warns = append(b.warns, w)
	}
	return b
}

// flag appends args when f is available at the target version.
func (b *builder) flag(f Feature, args ...string) {
	if b.err != nil {
		return
	}
	if err := checkFeature(b.t, f); err != nil {
		b.err = err
		return
	}
	b.args = append(b.args, args...)
	b.feats = append(b.feats, f)
}

func (b *builder) fail(err error) {
	if b.err == nil {
		b.err = err
	}
}

// global renders the flags shared by the TUI and exec; exec=true renders
// approval through -c (exec has no -a).
func (b *builder) global(o LaunchOptions, exec bool) {
	b.trailing = o.trailing
	for _, ov := range o.Overrides {
		if ov.Path == "" {
			b.fail(fmt.Errorf("%w: empty override path", ErrInvalidOption))
			return
		}
		b.flag(FeatureFlagConfig, flagConfig, ov.String())
	}
	for _, e := range o.Enable {
		b.flag(FeatureFlagEnable, flagEnable, e)
	}
	for _, d := range o.Disable {
		b.flag(FeatureFlagDisable, flagDisable, d)
	}
	if o.StrictConfig {
		b.flag(FeatureFlagStrictConfig, flagStrict)
	}
	for _, img := range o.Images {
		b.flag(FeatureFlagImage, flagImage, img)
	}
	if o.Model != "" {
		b.flag(FeatureFlagModel, flagModel, o.Model)
	}
	if o.OSS {
		b.flag(FeatureFlagOSS, flagOSS)
	}
	if o.LocalProvider != "" {
		b.flag(FeatureFlagLocalProvider, flagLocalProv, o.LocalProvider)
	}
	if o.Profile != "" {
		b.profile(o)
	}
	if o.Sandbox != "" {
		if !o.Sandbox.IsKnown() || o.Sandbox == ModeExternalSandbox {
			b.fail(fmt.Errorf("%w: sandbox %q", ErrInvalidOption, o.Sandbox))
			return
		}
		b.flag(FeatureFlagSandbox, flagSandbox, string(o.Sandbox))
	}
	if o.Approval != "" {
		if o.Approval != ApprovalOnRequest && o.Approval != ApprovalNever && o.Approval != ApprovalUntrusted && o.Approval != ApprovalOnFailure {
			b.fail(fmt.Errorf("%w: approval %q", ErrInvalidOption, o.Approval))
			return
		}
		if exec {
			b.flag(FeatureFlagConfig, flagConfig, OverrideString(keyApproval, string(o.Approval)).String())
		} else {
			b.flag(FeatureFlagApproval, flagApproval, string(o.Approval))
		}
	}
	if o.ApproveForMe {
		b.flag(FeatureFlagApproveForMe, flagApprove)
	}
	if o.BypassAll {
		if !b.t.AllowDangerous {
			b.fail(agentio.ErrDangerousNotAllowed)
			return
		}
		b.flag(FeatureFlagBypassApprovalsAndSandbox, flagBypass)
	}
	if o.Dir != "" {
		b.flag(FeatureFlagCd, flagCd, o.Dir)
	}
	if o.Worktree {
		b.flag(FeatureFlagWorktree, flagWorktree)
	}
	for _, d := range o.AddDirs {
		b.flag(FeatureFlagAddDir, flagAddDir, d)
	}
	if o.Search {
		b.flag(FeatureFlagSearch, flagSearch)
	}
	if o.NoAltScreen {
		b.flag(FeatureFlagNoAltScreen, flagNoAlt)
	}
	if o.NoDaemon {
		b.flag(FeatureFlagNoDaemon, flagNoDaemon)
	}
	if o.Remote != "" {
		b.flag(FeatureFlagRemote, flagRemote, o.Remote)
	}
	if o.RemoteAuthTokenEnv != "" {
		b.flag(FeatureFlagRemoteAuthTokenEnv, flagRemoteEnv, o.RemoteAuthTokenEnv)
	}
}

// warnProfileUnchecked is the warning for a --profile built without the
// home's config.toml (LaunchOptions.ProfileCheck nil): it says what was
// not checked and what Codex does about it, in the user's terms.
const warnProfileUnchecked = "--profile %s not checked against the legacy [profiles] tables in config.toml; Codex refuses it when one has that name"

// profile emits -p NAME with the version-gated meaning of §0.1 and the
// legacy-conflict refusal of loader/mod.rs:303.
func (b *builder) profile(o LaunchOptions) {
	v := b.t.Version
	legacyOnly := !v.IsZero() && v.Compare(agentio.MustVersion(profileV2Since)) < 0
	switch {
	case legacyOnly:
		// below 0.131.0 -p selects a legacy [profiles.NAME]: no conflict exists
	case o.ProfileCheck == nil:
		b.warns.Add(agentio.WarnVersionUnknown, fileConfig, keyProfiles, fmt.Sprintf(warnProfileUnchecked, o.Profile))
	default:
		if err := o.ProfileCheck.ProfileConflict(o.Profile); err != nil {
			b.fail(err)
			return
		}
	}
	if !legacyOnly && !v.IsZero() && v.Compare(agentio.MustVersion(profileV2Certain)) < 0 {
		b.warns.Add(agentio.WarnVersionUnknown, "", flagProfile, "meaning of --profile between 0.131.0 and 0.162.1 is unverified; built as a v2 profile")
	}
	b.flag(FeatureFlagProfile, flagProfile, o.Profile)
}

func (b *builder) execFlags(o ExecOptions) {
	if o.JSON {
		b.flag(FeatureExecJSON, flagJSON)
	}
	if o.OutputLastMessage != "" {
		b.flag(FeatureFlagOutputLastMessage, flagOutLast, o.OutputLastMessage)
	}
	if o.OutputSchema != "" {
		b.flag(FeatureFlagOutputSchema, flagOutSchema, o.OutputSchema)
	}
	if o.Color != "" {
		b.flag(FeatureFlagColor, flagColor, o.Color)
	}
	if o.SkipGitRepoCheck {
		b.flag(FeatureFlagSkipGitRepoCheck, flagSkipGit)
	}
	if o.Ephemeral {
		b.flag(FeatureFlagEphemeral, flagEphemeral)
	}
	if o.IgnoreUserConfig {
		b.flag(FeatureFlagIgnoreUserConfig, flagIgnoreCfg)
	}
	if o.IgnoreRules {
		b.flag(FeatureFlagIgnoreRules, flagIgnoreRul)
	}
	if o.ThreadSource != "" {
		b.flag(FeatureFlagThreadSource, flagThreadSrc, o.ThreadSource)
	}
	if o.CyberAccessProgram != "" {
		b.flag(FeatureFlagCyberAccessProgram, flagCyber, o.CyberAccessProgram)
	}
}

// finish appends the target's extra args and positionals and builds the
// Command with the home rule.
func (a *Agent) finish(b *builder, dir string, positional ...string) (agentio.Command, error) {
	if b.err != nil {
		return agentio.Command{}, b.err
	}
	args := append(b.args, b.t.ExtraArgs...)
	for _, p := range positional {
		if p != "" {
			args = append(args, p)
		}
	}
	args = append(args, b.trailing...)
	cmd, err := a.core.Command(b.t, dir, args, nil)
	if err != nil {
		return agentio.Command{}, err
	}
	cmd.Warnings = append(cmd.Warnings, b.warns...)
	return cmd, nil
}

func execPrompt(o LaunchOptions) string {
	if o.PromptFromStdin && o.Prompt == "" {
		return stdinPrompt
	}
	return o.Prompt
}

// New builds an interactive `codex [flags] [PROMPT]`. Pure.
func (a *Agent) New(t agentio.Target, o LaunchOptions) (agentio.Command, error) {
	b := newBuilder(t)
	b.global(o, false)
	return a.finish(b, o.Dir, o.Prompt)
}

// ErrNoSessionID means a resume/fork builder needs an id.
var ErrNoSessionID = agentio.NewLaunchSentinel("codex: session id required")

// Resume builds `codex resume [flags] ID [PROMPT]`. Pure.
func (a *Agent) Resume(t agentio.Target, id string, o LaunchOptions) (agentio.Command, error) {
	if id == "" {
		return agentio.Command{}, ErrNoSessionID
	}
	b := newBuilder(t)
	b.flag(FeatureSubResume, subResume)
	b.global(o, false)
	return a.finish(b, o.Dir, id, o.Prompt)
}

// ResumeLast builds `codex resume --last [--all] [flags] [PROMPT]`. Pure.
func (a *Agent) ResumeLast(t agentio.Target, o LaunchOptions, all bool) (agentio.Command, error) {
	b := newBuilder(t)
	b.flag(FeatureSubResume, subResume, flagLast)
	if all {
		b.args = append(b.args, flagAll)
	}
	b.global(o, false)
	return a.finish(b, o.Dir, o.Prompt)
}

// Fork builds `codex fork [flags] ID [PROMPT]`. Pure.
func (a *Agent) Fork(t agentio.Target, id string, o LaunchOptions) (agentio.Command, error) {
	if id == "" {
		return agentio.Command{}, ErrNoSessionID
	}
	b := newBuilder(t)
	b.flag(FeatureSubFork, subFork)
	b.global(o, false)
	return a.finish(b, o.Dir, id, o.Prompt)
}

// Exec builds `codex exec [flags] [PROMPT|-]`. Pure; RUNNING it spends Quota.
func (a *Agent) Exec(t agentio.Target, o ExecOptions) (agentio.Command, error) {
	b := newBuilder(t)
	b.flag(FeatureSubExec, subExec)
	b.global(o.LaunchOptions, true)
	b.execFlags(o)
	return a.finish(b, o.Dir, execPrompt(o.LaunchOptions))
}

// ExecResume builds `codex exec resume [flags] ID [PROMPT|-]`. Pure;
// running it spends Quota.
func (a *Agent) ExecResume(t agentio.Target, id string, o ExecOptions) (agentio.Command, error) {
	if id == "" {
		return agentio.Command{}, ErrNoSessionID
	}
	b := newBuilder(t)
	b.flag(FeatureSubExec, subExec, subResume)
	b.global(o.LaunchOptions, true)
	if o.OutputSchema != "" {
		b.flag(FeatureExecResumeOutputSchema)
	}
	b.execFlags(o)
	return a.finish(b, o.Dir, id, execPrompt(o.LaunchOptions))
}

// Review builds `codex exec review [--uncommitted|--base B|--commit SHA]
// [--title T] [flags]`. Pure; running it spends Quota.
func (a *Agent) Review(t agentio.Target, r ReviewTarget, o ExecOptions) (agentio.Command, error) {
	n := 0
	for _, set := range []bool{r.Uncommitted, r.Base != "", r.Commit != ""} {
		if set {
			n++
		}
	}
	if n > 1 {
		return agentio.Command{}, fmt.Errorf("%w: review takes one of uncommitted, base, commit", ErrInvalidOption)
	}
	b := newBuilder(t)
	b.flag(FeatureSubExec, subExec)
	b.flag(FeatureSubReview, subReview)
	switch {
	case r.Uncommitted:
		b.args = append(b.args, flagUncommit)
	case r.Base != "":
		b.args = append(b.args, flagBase, r.Base)
	case r.Commit != "":
		b.args = append(b.args, flagCommit, r.Commit)
	}
	if r.Title != "" {
		b.args = append(b.args, flagTitle, r.Title)
	}
	b.global(o.LaunchOptions, true)
	b.execFlags(o)
	return a.finish(b, o.Dir)
}

// Archive builds `codex archive ID`. Pure.
func (a *Agent) Archive(h Home, id string) agentio.Command {
	return a.plainCommand(h, []string{subArchive, id})
}

// Unarchive builds `codex unarchive ID`. Pure.
func (a *Agent) Unarchive(h Home, id string) agentio.Command {
	return a.plainCommand(h, []string{subUnarchive, id})
}

// Delete builds `codex delete [--force] ID`. Pure.
func (a *Agent) Delete(h Home, id string, force bool) agentio.Command {
	args := []string{subDelete}
	if force {
		args = append(args, flagForce)
	}
	return a.plainCommand(h, append(args, id))
}

// Queue builds `codex queue --thread ID --message TEXT`. Pure.
func (a *Agent) Queue(h Home, threadID, message string) agentio.Command {
	return a.plainCommand(h, []string{subQueue, flagThread, threadID, flagMessage, message})
}

// DoctorCommand builds `codex doctor [--json]` (the vendor's own report;
// may touch the network and auth when run). Pure.
func (a *Agent) DoctorCommand(h Home, json bool) agentio.Command {
	args := []string{subDoctor}
	if json {
		args = append(args, flagJSON)
	}
	return a.plainCommand(h, args)
}

// MCPServer builds `codex mcp-server` (stdio MCP exposing Codex). Pure.
func (a *Agent) MCPServer(h Home) agentio.Command {
	return a.plainCommand(h, []string{subMCPServer})
}

// AppServer builds `codex app-server [--listen URL]`; "" = stdio. Pure.
func (a *Agent) AppServer(h Home, listen string) agentio.Command {
	args := []string{subAppServer}
	if listen != "" {
		args = append(args, flagListen, listen)
	}
	return a.plainCommand(h, args)
}

// FeatureCommand builds `codex features enable|disable NAME`: the vendor
// edits its own config, so the library never writes config.toml. Pure.
func (a *Agent) FeatureCommand(h Home, name string, on bool) agentio.Command {
	verb := subDisable
	if on {
		verb = subEnable
	}
	return a.plainCommand(h, []string{subFeatures, verb, name})
}

// MCPAddSpec describes `codex mcp add`: a URL server or a stdio command.
type MCPAddSpec struct {
	URL               string
	BearerTokenEnvVar string // a variable NAME
	Command           string
	Args              []string
	// Env values would ride on argv (`--env K=V`), which invariant 8
	// forbids for EnvValue; a non-empty Env is refused.
	Env map[string]agentio.EnvValue
}

// MCPAddCommand builds `codex mcp add NAME (--url U [--bearer-token-env-var
// V] | -- CMD ARGS…)`. Pure.
func (a *Agent) MCPAddCommand(h Home, name string, s MCPAddSpec) (agentio.Command, error) {
	if len(s.Env) > 0 {
		return agentio.Command{}, agentio.UnsupportedError(ID, "mcp add with env values", "codex mcp add takes env values on argv (--env K=V), which would expose them; add the server without env and set env in config.toml")
	}
	args := []string{subMCP, subAdd, name}
	switch {
	case s.URL != "" && s.Command != "":
		return agentio.Command{}, fmt.Errorf("%w: mcp add takes a URL or a command, not both", ErrInvalidOption)
	case s.URL != "":
		args = append(args, flagURL, s.URL)
		if s.BearerTokenEnvVar != "" {
			args = append(args, flagBearerEnv, s.BearerTokenEnvVar)
		}
	case s.Command != "":
		args = append(append(args, flagArgsSep, s.Command), s.Args...)
	default:
		return agentio.Command{}, fmt.Errorf("%w: mcp add needs a URL or a command", ErrInvalidOption)
	}
	return a.plainCommand(h, args), nil
}

// MCPRemoveCommand builds `codex mcp remove NAME`. Pure.
func (a *Agent) MCPRemoveCommand(h Home, name string) agentio.Command {
	return a.plainCommand(h, []string{subMCP, subRemove, name})
}

// TargetFor fills a launch target with the installed version from disk
// (package.json), never by running codex. Disk · Read.
func (a *Agent) TargetFor(h Home) (agentio.Target, error) {
	t := h.Target()
	in, err := a.Install()
	if err != nil && !errors.Is(err, agentio.ErrNotInstalled) {
		return t, err
	}
	t.Version = in.Version
	return t, nil
}

// launchNeutral maps agentio.LaunchRequest onto the builders (SPEC §5.4).
func (a *Agent) launchNeutral(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	o, err := optionsFromRequest(t, r)
	if err != nil {
		return agentio.Command{}, err
	}
	return a.launchOptions(t, r, o)
}

// optionsFromRequest maps the neutral request's option fields (prompt,
// model, extra dirs, autonomy, raw args) onto LaunchOptions; resume and
// headless pick the builder instead (launchOptions). A token login and a
// level Codex cannot express exactly are refused. Pure.
func optionsFromRequest(t agentio.Target, r agentio.LaunchRequest) (LaunchOptions, error) {
	if r.Credential != nil {
		return LaunchOptions{}, agentio.UnsupportedError(ID, "token login", "Codex has no token login; log in with codex login")
	}
	o := LaunchOptions{Prompt: r.Prompt, Model: r.Model, AddDirs: r.AddDirs}
	switch r.Autonomy {
	case agentio.AutonomyDefault:
	case agentio.AutonomyReadOnly:
		o.Sandbox, o.Approval = ModeReadOnly, ApprovalOnRequest
	case agentio.AutonomyAsk:
		o.Sandbox, o.Approval = ModeWorkspaceWrite, ApprovalOnRequest
	case agentio.AutonomyAcceptEdits:
		return LaunchOptions{}, agentio.UnsupportedError(ID, "autonomy accept-edits", "Codex has no edits-only mode; never mapped to a more permissive one")
	case agentio.AutonomyFull:
		if err := agentio.CheckAutonomy(t, r.Autonomy); err != nil {
			return LaunchOptions{}, err
		}
		o.BypassAll = true
	default:
		return LaunchOptions{}, agentio.UnsupportedError(ID, "autonomy "+r.Autonomy.String(), "unknown autonomy level")
	}
	o.trailing = slices.Clone(r.Args)
	return o, nil
}

// launchOptions picks the builder for the request (exec for headless,
// resume for an id) and builds o with it, in the request's dir. Pure.
func (a *Agent) launchOptions(t agentio.Target, r agentio.LaunchRequest, o LaunchOptions) (agentio.Command, error) {
	var (
		cmd agentio.Command
		err error
	)
	switch {
	case r.Headless && r.ResumeID != "":
		cmd, err = a.ExecResume(t, r.ResumeID, ExecOptions{LaunchOptions: o, JSON: true})
	case r.Headless:
		cmd, err = a.Exec(t, ExecOptions{LaunchOptions: o, JSON: true})
	case r.ResumeID != "":
		cmd, err = a.Resume(t, r.ResumeID, o)
	default:
		cmd, err = a.New(t, o)
	}
	if err != nil {
		return agentio.Command{}, err
	}
	cmd.Dir = r.Dir
	return cmd, nil
}
