package agentio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Core is the Spec-driven L1 that every agent shares: homes, home env rule,
// PATH lookup, launch from templates, process matching, home links, login
// commands and the L1 doctor checks. FromSpec wraps it for config-only
// agents; vendor packages embed it in their neutral view so the L1 rules
// (invariants 2, 9, 12) live in one place.
type Core struct {
	spec Spec
	host Host
}

// NewCore validates s and binds it to h. Pure.
func NewCore(s Spec, h Host) (*Core, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &Core{spec: s, host: h}, nil
}

// ID is Pure.
func (c *Core) ID() ID { return c.spec.ID }

// Name is Pure.
func (c *Core) Name() string { return c.spec.Name }

// Spec returns a copy of the spec. Pure.
func (c *Core) Spec() Spec { return c.spec }

// Host returns the bound Host. Pure.
func (c *Core) Host() Host { return c.host }

// LookPath walks PATH from Host.Env (invariant 12). Disk · Read.
// Host.GOOS picks the rules: ';' separators and PATHEXT on Windows.
func (c *Core) LookPath() (string, error) { return c.host.LookPath(c.spec.Binary) }

// envPath is the PATH variable name.
const envPath = "PATH"

// LookPath finds name in the PATH value pathList (os.PathListSeparator
// separated), returning the first runnable file under the running OS's
// rules (LookPathAllFor with runtime.GOOS): on Windows PATHEXT's default
// extensions are tried and no exec bit is needed; elsewhere a regular
// file with an exec bit. Empty and relative entries ("." by shell
// convention) are skipped: running a binary from the current dir is never
// intended here. An absolute name is checked as is. Disk · Read.
func LookPath(pathList, name string) (string, error) {
	if all := LookPathAll(pathList, name); len(all) > 0 {
		return all[0], nil
	}
	return "", fmt.Errorf("%w: %s", ErrNotInstalled, name)
}

// LookPathAll returns every PATH hit for name in order (the first is what
// runs; the rest are shadowed copies), under the running OS's rules. It
// reads no environment: on Windows the default PATHEXT is used (use
// Host.LookPathAll for the Host's PATHEXT). Disk · Read.
func LookPathAll(pathList, name string) []string {
	return LookPathAllFor(runtime.GOOS, pathList, "", name)
}

// execBits are the permission bits that make a file runnable by someone.
const execBits = 0o111

// DefaultHomeName is the Name of every default home.
const DefaultHomeName = "personal"

// homeNameSep joins a default home's base and a suffix (".claude-work").
const homeNameSep = "-"

// defaultHomeDir is the absolute default home dir.
func (c *Core) defaultHomeDir() string {
	return filepath.Join(c.host.UserHome, c.spec.DefaultHome)
}

// DefaultHome is Pure.
func (c *Core) DefaultHome() Home {
	return NewHome(c.spec.ID, c.defaultHomeDir(), true, DefaultHomeName)
}

// HomeAt returns the Home value for dir: the default home when dir cleans
// to the default dir, else a non-default home named from its base name.
// Relative dirs resolve against Host.UserHome. Pure.
func (c *Core) HomeAt(dir string) Home {
	dir = resolveHomeDir(c.host.hostOS(), c.host.UserHome, dir)
	dir = filepath.Clean(dir)
	if dir == filepath.Clean(c.defaultHomeDir()) {
		return c.DefaultHome()
	}
	return NewHome(c.spec.ID, dir, false, c.homeName(dir))
}

// resolveHomeDir makes a home variable's value absolute the way goos
// would: a relative dir is under userHome; on Windows a rooted dir without
// a drive (\work\h, /work/h) is on the drive of userHome (Windows resolves
// it against the current drive, which for a user's tools is that one), not
// under userHome. Each OS rule uses that OS's path syntax (package path
// for POSIX, backslash rules for Windows), never the runner's filepath, so
// the rules give the same answer on every host. HomeAt then cleans the
// result with filepath for the host it runs on. Pure.
func resolveHomeDir(goos, userHome, dir string) string {
	if goos != OSWindows {
		if path.IsAbs(dir) {
			return dir
		}
		return path.Join(userHome, dir)
	}
	if windowsAbs(dir) {
		return dir
	}
	win := strings.ReplaceAll(dir, "/", `\`)
	if strings.HasPrefix(win, `\`) {
		return windowsVolume(userHome) + win
	}
	return strings.TrimRight(userHome, `\/`) + `\` + win
}

// homeName derives "work" from ~/.claude-work, else the base without a dot.
func (c *Core) homeName(dir string) string {
	base := filepath.Base(dir)
	if c.spec.HomeEnvNamesParent && base == c.spec.DefaultHome {
		// <parent>/.gemini: the parent names the home (~/gemini-work/.gemini
		// is "gemini-work").
		base = filepath.Base(filepath.Dir(dir))
		return strings.TrimPrefix(base, ".")
	}
	defBase := filepath.Base(c.spec.DefaultHome)
	if rest, ok := strings.CutPrefix(base, defBase+homeNameSep); ok && rest != "" {
		return rest
	}
	return strings.TrimPrefix(base, ".")
}

// HomeFromEnv returns the home Host.Env selects through Spec.HomeEnv. An
// empty value counts as absent (the vendor CLIs fall back to the default).
// Pure.
func (c *Core) HomeFromEnv() (Home, bool) {
	if c.spec.HomeEnv == "" {
		return Home{}, false
	}
	v, ok := c.host.Getenv(c.spec.HomeEnv)
	if !ok || v == "" {
		return Home{}, false
	}
	if c.spec.HomeEnvNamesParent {
		v = filepath.Join(resolveHomeDir(c.host.hostOS(), c.host.UserHome, v), c.spec.DefaultHome)
	}
	return c.HomeAt(v), true
}

// Homes lists homes on disk: the default first, then ExtraHomes glob hits
// sorted, then the env-selected home if not already listed; only dirs
// that pass IsHome. Disk · Read.
func (c *Core) Homes() ([]Home, error) {
	var out []Home
	seen := map[string]bool{}
	add := func(h Home) error {
		if seen[h.Dir()] {
			return nil
		}
		ok, err := c.IsHome(h.Dir())
		if err != nil {
			return err
		}
		if ok {
			seen[h.Dir()] = true
			out = append(out, h)
		}
		return nil
	}
	if c.spec.DefaultHome != "" {
		if err := add(c.DefaultHome()); err != nil {
			return nil, err
		}
	}
	if c.spec.ExtraHomes != "" {
		matches, err := filepath.Glob(filepath.Join(c.host.UserHome, c.spec.ExtraHomes))
		if err != nil {
			return nil, fmt.Errorf("agentio: %s homes: %w", c.spec.ID, err)
		}
		slices.Sort(matches)
		for _, m := range matches {
			if err := add(c.HomeAt(m)); err != nil {
				return nil, err
			}
		}
	}
	if h, ok := c.HomeFromEnv(); ok {
		if err := add(h); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// IsHome reports whether dir is a directory holding any HomeMarker (any
// directory when the spec names none). A missing dir is (false, nil).
// Disk · Read.
func (c *Core) IsHome(dir string) (bool, error) {
	fi, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("agentio: %s home %s: %w", c.spec.ID, dir, err)
	}
	if !fi.IsDir() {
		return false, nil
	}
	if len(c.spec.HomeMarkers) == 0 {
		return true, nil
	}
	for _, m := range c.spec.HomeMarkers {
		if _, err := os.Lstat(filepath.Join(dir, m)); err == nil {
			return true, nil
		}
	}
	return false, nil
}

// Owns returns ErrForeignHome when h is not this agent's home. Pure.
func (c *Core) Owns(h Home) error { return CheckOwner(c.spec.ID, h) }

// EnvChanges are the changes a launch on t applies to Host.Env, in order:
//   - the home's own login (tokenEnv nil): Spec.CredentialEnv is scrubbed,
//     because those variables would override the stored login;
//   - a token login: only the variables that would override that token are
//     scrubbed (tokenScrub, from TokenEnv.Scrub or CredentialEnv minus the
//     token's own variable), again after Target.ExtraEnv, then the token is
//     set. A variable that is itself the login mechanism is never listed,
//     so it survives;
//   - the home variable last (HomeEnvChange, or its parent for
//     Spec.HomeEnvNamesParent).
//
// Pure.
func (c *Core) EnvChanges(t Target, tokenEnv []EnvChange) []EnvChange {
	return c.envChanges(t, tokenEnv, c.tokenScrub(tokenEnv))
}

// tokenScrub is CredentialEnv without the names tokenEnv sets: what a
// token login removes when the agent did not describe the login per kind.
// nil tokenEnv = the full list (home login). Pure.
func (c *Core) tokenScrub(tokenEnv []EnvChange) []string {
	if tokenEnv == nil {
		return c.spec.CredentialEnv
	}
	var out []string
	for _, n := range c.spec.CredentialEnv {
		if !slices.ContainsFunc(tokenEnv, func(e EnvChange) bool { return e.name == n }) {
			out = append(out, n)
		}
	}
	return out
}

// envChanges is EnvChanges with an explicit scrub list for the token login.
func (c *Core) envChanges(t Target, tokenEnv []EnvChange, scrub []string) []EnvChange {
	var out []EnvChange
	if tokenEnv == nil {
		out = ScrubEnv(c.spec.CredentialEnv)
	} else {
		out = ScrubEnv(scrub)
	}
	out = append(out, t.ExtraEnv...)
	if tokenEnv != nil {
		out = append(out, ScrubEnv(scrub)...)
		out = append(out, tokenEnv...)
	}
	if hc, ok := c.homeEnvChange(t.Home); ok {
		out = append(out, hc)
	}
	return out
}

// homeEnvChange is the home variable for h: HomeEnvChange, or for
// Spec.HomeEnvNamesParent the dir that contains h (unset for the default
// home in both cases). Pure.
func (c *Core) homeEnvChange(h Home) (EnvChange, bool) {
	if !c.spec.HomeEnvNamesParent || h.IsDefault() || c.spec.HomeEnv == "" {
		return HomeEnvChange(c.spec.HomeEnv, h)
	}
	return EnvSet(c.spec.HomeEnv, filepath.Dir(h.Dir())), true
}

// checkHomeSelectable refuses a home the home variable cannot select: with
// Spec.HomeEnvNamesParent every non-default home must be named
// DefaultHome inside its parent (<parent>/.gemini). Pure.
func (c *Core) checkHomeSelectable(h Home) error {
	if !c.spec.HomeEnvNamesParent || h.IsDefault() || filepath.Base(h.Dir()) == c.spec.DefaultHome {
		return nil
	}
	return fmt.Errorf("%w: %s: %s selects <dir>/%s, so the home %s cannot be launched", ErrUnsupported, c.spec.ID, c.spec.HomeEnv, c.spec.DefaultHome, h.Dir())
}

// Command builds the Command for argv on t: owner check, Host.Env with
// EnvChanges applied, Path = Spec.Binary. tokenEnv nil = the home's own
// login. Pure.
func (c *Core) Command(t Target, dir string, argv []string, tokenEnv []EnvChange) (Command, error) {
	return c.command(t, dir, argv, tokenEnv, c.tokenScrub(tokenEnv))
}

// command is Command with an explicit token scrub list.
func (c *Core) command(t Target, dir string, argv []string, tokenEnv []EnvChange, scrub []string) (Command, error) {
	if err := c.Owns(t.Home); err != nil {
		return Command{}, err
	}
	if err := c.checkHomeSelectable(t.Home); err != nil {
		return Command{}, err
	}
	return c.host.NewCommand(c.spec.Binary, argv, dir, c.envChanges(t, tokenEnv, scrub)...), nil
}

// CheckAutonomy applies the rule every builder shares: AutonomyFull needs
// Target.AllowDangerous. Pure.
func CheckAutonomy(t Target, a Autonomy) error {
	if a == AutonomyFull && !t.AllowDangerous {
		return ErrDangerousNotAllowed
	}
	return nil
}

// Launch builds a command from the Spec templates (config-only agents).
// Each request field needs its template: autonomy levels come from
// Spec.AutonomyArgs, Headless from HeadlessArgs, a token login from
// TokenEnv; a field without one is ErrUnsupported (never silently
// dropped). AutonomyFull still needs Target.AllowDangerous. Pure.
func (c *Core) Launch(t Target, r LaunchRequest) (Command, error) {
	if err := CheckAutonomy(t, r.Autonomy); err != nil {
		return Command{}, err
	}
	s := c.spec
	unsupported := func(what string) (Command, error) {
		return Command{}, UnsupportedError(s.ID, what, "not described by the agent spec")
	}
	autonomy, autonomyOK := s.AutonomyArgs[r.Autonomy]
	switch {
	case r.Autonomy != AutonomyDefault && !autonomyOK:
		return unsupported("autonomy " + r.Autonomy.String())
	case r.Headless && len(s.HeadlessArgs) == 0:
		return unsupported("headless")
	case r.Model != "" && len(s.ModelArgs) == 0:
		return unsupported("model")
	case len(r.AddDirs) > 0 && len(s.AddDirArgs) == 0:
		return unsupported("add-dir")
	case r.Prompt != "" && len(s.PromptArgs) == 0:
		return unsupported("prompt")
	case r.ResumeID != "" && len(s.ResumeArgs) == 0:
		return unsupported("resume")
	}
	var tokenEnv []EnvChange
	var scrub []string
	if r.Credential != nil {
		e, ok := s.tokenEnvFor(*r.Credential)
		if !ok {
			what := "token login " + r.Credential.Kind.String()
			if r.Credential.Provider != "" {
				what += " for provider " + r.Credential.Provider
			}
			return unsupported(what)
		}
		tokenEnv = []EnvChange{EnvSetSecret(e.Env, r.Credential.Token)}
		scrub = s.scrubFor(e)
	}
	args := slices.Clone(t.ExtraArgs)
	args = append(args, autonomy...)
	if r.Headless {
		args = append(args, s.HeadlessArgs...)
	}
	if r.Model != "" {
		args = append(args, expandTemplate(s.ModelArgs, placeholderModel, r.Model)...)
	}
	for _, d := range r.AddDirs {
		args = append(args, expandTemplate(s.AddDirArgs, placeholderDir, d)...)
	}
	args = append(args, r.Args...)
	if r.Prompt != "" {
		args = append(args, expandTemplate(s.PromptArgs, placeholderPrompt, r.Prompt)...)
	}
	tpl := s.NewArgs
	if r.ResumeID != "" {
		tpl = expandTemplate(s.ResumeArgs, placeholderID, r.ResumeID)
	}
	return c.command(t, r.Dir, spliceArgs(tpl, args), tokenEnv, scrub)
}

// Install finds Spec.Binary on Host.Env PATH and reads its version from
// install metadata (InstallFromPath with Spec.Packages), never running it.
// Disk · Read.
func (c *Core) Install() (Install, error) {
	in, err := InstallFromPath(c.host.LookPathAll(c.spec.Binary), c.spec.Packages)
	if err != nil {
		return Install{}, fmt.Errorf("%w: %s", err, c.spec.Binary)
	}
	return in, nil
}

// ClassifyProcess matches p against Spec.Process; config agents report
// Kind cli and Session true. Vendors refine Kind on top. Pure.
func (c *Core) ClassifyProcess(p Process) (ProcessInfo, bool) {
	if !c.spec.Process.Match(p) {
		return ProcessInfo{}, false
	}
	return ProcessInfo{Agent: c.spec.ID, Kind: ProcessCLI, Session: true}, true
}

// PlanHomeLinks plans links of Spec.SharedHomeFiles from def into h.
// Disk · Read.
func (c *Core) PlanHomeLinks(def, h Home) ([]HomeLink, error) {
	if err := c.Owns(def); err != nil {
		return nil, err
	}
	if err := c.Owns(h); err != nil {
		return nil, err
	}
	return PlanHomeLinks(def, h, c.spec.SharedHomeFiles)
}

// ApplyHomeLinks makes the planned symlinks after checking every link
// belongs to this agent (ErrForeignHome otherwise, nothing linked).
// Disk · Write.
func (c *Core) ApplyHomeLinks(links []HomeLink) error {
	if err := CheckLinksOwner(c.spec.ID, links); err != nil {
		return err
	}
	return ApplyHomeLinks(links)
}

// LoginCommand builds Spec.LoginArgs on h; only LoginBrowser is
// describable by a spec. Pure.
func (c *Core) LoginCommand(h Home, f LoginFlow) (Command, error) {
	if len(c.spec.LoginArgs) == 0 || f != LoginBrowser {
		return Command{}, UnsupportedError(c.spec.ID, "login flow "+f.String(), "not described by the agent spec")
	}
	return c.Command(h.Target(), "", c.spec.LoginArgs, nil)
}

// LogoutCommand builds Spec.LogoutArgs on h; a zero Command when the spec
// has none or h is foreign. Pure.
func (c *Core) LogoutCommand(h Home) Command {
	if len(c.spec.LogoutArgs) == 0 {
		return Command{}
	}
	cmd, err := c.Command(h.Target(), "", c.spec.LogoutArgs, nil)
	if err != nil {
		return Command{}
	}
	return cmd
}

// Runner is the door to Process calls that run the agent's binary through
// Host.Exec. Holding one is the opt-in (SPEC §4); vendor Runners embed it.
type Runner struct{ core *Core }

// Runner returns the Process door. Pure.
func (c *Core) Runner() Runner { return Runner{core: c} }

// Run runs cmd through Host.Exec. Process · Read; may touch the login.
func (r Runner) Run(ctx context.Context, cmd Command, stdin io.Reader) (ProcessResult, error) {
	if r.core == nil || r.core.host.Exec == nil {
		return ProcessResult{}, fmt.Errorf("agentio: no process runner in host")
	}
	return r.core.host.Exec.Run(ctx, cmd, stdin)
}

// Version runs `<binary> <Spec.VersionArgs>` on the default home and
// parses the first version in stdout (stderr as fallback). Process · Read.
func (r Runner) Version(ctx context.Context) (Version, error) {
	c := r.core
	if c == nil || len(c.spec.VersionArgs) == 0 {
		return Version{}, UnsupportedError(r.id(), "binary version", "no version args")
	}
	cmd, err := c.Command(c.DefaultHome().Target(), "", c.spec.VersionArgs, nil)
	if err != nil {
		return Version{}, err
	}
	res, err := r.Run(ctx, cmd, nil)
	if err != nil {
		return Version{}, fmt.Errorf("agentio: %s version: %w", c.spec.ID, err)
	}
	if res.ExitCode != 0 {
		return Version{}, fmt.Errorf("agentio: %s version: exit %d", c.spec.ID, res.ExitCode)
	}
	for _, out := range [][]byte{res.Stdout, res.Stderr} {
		if v, ok := ParseVersion(string(out)); ok {
			return v, nil
		}
	}
	return Version{}, fmt.Errorf("agentio: %s version: no version in output", c.spec.ID)
}

// id is the core's ID or "" for a zero Runner.
func (r Runner) id() ID {
	if r.core == nil {
		return ""
	}
	return r.core.spec.ID
}

// Checks runs the L1 doctor checks on h: binary on PATH, home present,
// home variable in the base env, API-billing variables set. Names only,
// never values. Disk · Read.
func (c *Core) Checks(_ context.Context, h Home) []Check {
	var out []Check
	if p, err := c.LookPath(); err != nil {
		out = append(out, Check{ID: CheckBinary, Status: CheckFail, Message: c.spec.Binary + " not found on PATH"})
	} else {
		out = append(out, Check{ID: CheckBinary, Status: CheckOK, Message: p})
	}
	if err := c.Owns(h); err != nil {
		out = append(out, Check{ID: CheckHome, Status: CheckFail, Message: err.Error()})
	} else if ok, err := c.IsHome(h.Dir()); err != nil {
		out = append(out, Check{ID: CheckHome, Status: CheckFail, Message: err.Error()})
	} else if !ok {
		out = append(out, Check{ID: CheckHome, Status: CheckWarn, Message: h.Dir() + " is not a home (missing or no marker)"})
	} else {
		out = append(out, Check{ID: CheckHome, Status: CheckOK, Message: h.Dir()})
	}
	if c.spec.HomeEnv != "" {
		if v, ok := c.host.Getenv(c.spec.HomeEnv); ok {
			out = append(out, Check{ID: CheckHomeEnv, Status: CheckInfo, Message: c.spec.HomeEnv + " is set in the base env (selects " + v + "); launches of the default home remove it"})
		}
	}
	var billing []string
	for _, n := range c.spec.APIBillingEnv {
		if _, ok := c.host.Getenv(n); ok {
			billing = append(billing, n)
		}
	}
	if len(billing) > 0 {
		out = append(out, Check{ID: CheckAPIBillingEnv, Status: CheckWarn, Message: strings.Join(billing, ", ") + " set: launches bill per token"})
	}
	return out
}

// VersionCheck is the shared `version-tested` check for a built-in:
// installed version against its Compat; warn when newer, older or unknown.
// Pure.
func VersionCheck(compat Compat, installed Version) Check {
	switch compat.Check(installed) {
	case CompatInRange:
		return Check{ID: CheckVersionTested, Status: CheckOK, Message: installed.String() + " is tested"}
	case CompatUnknown:
		return Check{ID: CheckVersionTested, Status: CheckWarn, Message: "installed version unknown; tested " + compat.Min.String() + ".." + compat.Max.String()}
	}
	return Check{ID: CheckVersionTested, Status: CheckWarn, Message: installed.String() + " is untested (tested " + compat.Min.String() + ".." + compat.Max.String() + ")"}
}
