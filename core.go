package agentio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
func (c *Core) LookPath() (string, error) {
	path, _ := c.host.Getenv(envPath)
	return LookPath(path, c.spec.Binary)
}

// envPath is the PATH variable name.
const envPath = "PATH"

// LookPath finds name in the PATH value pathList (os.PathListSeparator
// separated), returning the first regular executable file. Empty entries
// ("." by shell convention) are skipped: running a binary from the
// current dir is never intended here. An absolute name is checked as is.
// Disk · Read.
func LookPath(pathList, name string) (string, error) {
	if all := LookPathAll(pathList, name); len(all) > 0 {
		return all[0], nil
	}
	return "", fmt.Errorf("%w: %s", ErrNotInstalled, name)
}

// LookPathAll returns every PATH hit for name in order (the first is what
// runs; the rest are shadowed copies). Disk · Read.
func LookPathAll(pathList, name string) []string {
	if name == "" {
		return nil
	}
	if filepath.IsAbs(name) {
		if isExecutable(name) {
			return []string{name}
		}
		return nil
	}
	var out []string
	for _, dir := range filepath.SplitList(pathList) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		p := filepath.Join(dir, name)
		if isExecutable(p) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// execBits are the permission bits that make a file runnable by someone.
const execBits = 0o111

// isExecutable reports a regular file (after symlinks) with an exec bit.
func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&execBits != 0
}

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
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(c.host.UserHome, dir)
	}
	dir = filepath.Clean(dir)
	if dir == filepath.Clean(c.defaultHomeDir()) {
		return c.DefaultHome()
	}
	return NewHome(c.spec.ID, dir, false, c.homeName(dir))
}

// homeName derives "work" from ~/.claude-work, else the base without a dot.
func (c *Core) homeName(dir string) string {
	base := filepath.Base(dir)
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

// EnvChanges is the ordered env edit list for a launch (invariant 9):
//  1. a scrub of Spec.CredentialEnv, for every login: an inherited
//     credential variable overrides the home's stored login without a
//     word, so the child never inherits one;
//  2. Target.ExtraEnv, so a credential the caller sets on purpose is kept
//     for a home login;
//  3. token login only: the scrub again (an ExtraEnv credential must not
//     beat the token) followed by the token's own changes;
//  4. the home rule last so nothing can override it (invariant 2).
//
// Pure.
func (c *Core) EnvChanges(t Target, tokenEnv []EnvChange) []EnvChange {
	out := ScrubEnv(c.spec.CredentialEnv)
	out = append(out, t.ExtraEnv...)
	if tokenEnv != nil {
		out = append(out, ScrubEnv(c.spec.CredentialEnv)...)
		out = append(out, tokenEnv...)
	}
	if hc, ok := HomeEnvChange(c.spec.HomeEnv, t.Home); ok {
		out = append(out, hc)
	}
	return out
}

// Command builds the Command for argv on t: owner check, Host.Env with
// EnvChanges applied, Path = Spec.Binary. tokenEnv nil = the home's own
// login. Pure.
func (c *Core) Command(t Target, dir string, argv []string, tokenEnv []EnvChange) (Command, error) {
	if err := c.Owns(t.Home); err != nil {
		return Command{}, err
	}
	return NewCommand(c.spec.Binary, argv, c.host.Env, dir, c.EnvChanges(t, tokenEnv)...), nil
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
// Refusals: AutonomyFull without AllowDangerous; any other non-default
// Autonomy, Headless, Credential, and any field without its template are
// ErrUnsupported (never silently dropped). Pure.
func (c *Core) Launch(t Target, r LaunchRequest) (Command, error) {
	if err := CheckAutonomy(t, r.Autonomy); err != nil {
		return Command{}, err
	}
	s := c.spec
	unsupported := func(what string) (Command, error) {
		return Command{}, UnsupportedError(s.ID, what, "not described by the agent spec")
	}
	switch {
	case r.Autonomy != AutonomyDefault:
		return unsupported("autonomy " + r.Autonomy.String())
	case r.Headless:
		return unsupported("headless")
	case r.Credential != nil:
		return unsupported("token login")
	case r.Model != "" && len(s.ModelArgs) == 0:
		return unsupported("model")
	case len(r.AddDirs) > 0 && len(s.AddDirArgs) == 0:
		return unsupported("add-dir")
	case r.Prompt != "" && len(s.PromptArgs) == 0:
		return unsupported("prompt")
	case r.ResumeID != "" && len(s.ResumeArgs) == 0:
		return unsupported("resume")
	}
	args := slices.Clone(t.ExtraArgs)
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
	return c.Command(t, r.Dir, spliceArgs(tpl, args), nil)
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
