package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// InstallMethod values beyond the neutral set (install-context/src/lib.rs).
const (
	InstallVitePlus agentio.InstallMethod = "vite-plus"
)

// Install-path facts (install-context/src/lib.rs:121-131, 304-316; a real
// Mac: /opt/homebrew/bin/codex → ../lib/node_modules/@openai/codex/bin/codex.js).
const (
	nodeModulesMarker   = "node_modules/@openai/codex"
	fileCaskroom        = "/Caskroom/"
	dirStandalone       = "packages/standalone"
	filePackageJSON     = "package.json"
	bunMarker           = "/.bun/"
	pnpmMarker          = "/pnpm/"
	dirCellarCodex      = "/Cellar/codex/"
	dirBunHome          = ".bun"
	dirVoltaHome        = ".volta"
	dirVoltaHomeWindows = "volta"
	dirBin              = "bin"
)

// Install is the on-disk install of the codex binary.
type Install struct {
	agentio.Install
	PackageRoot string // node_modules/@openai/codex for npm-family installs
}

// ParseVersion accepts "codex-cli 0.162.1" and bare "0.162.1";
// "0.158.0-alpha.2.1" → Pre "alpha.2.1". It has the root
// agentio.ParseVersion shape: ok is false when s holds no version. Pure.
func ParseVersion(s string) (agentio.Version, bool) { return agentio.ParseVersion(s) }

// Install finds codex on Host.Env PATH (a walk under Host.GOOS's rules:
// PATHEXT on Windows finds codex.cmd; never a shell), resolves symlinks and
// launcher shims, infers the method, and reads the npm package version from
// package.json. Layouts recognised, on every OS:
//   - a symlink into node_modules/@openai/codex (npm, nvm and Homebrew-node
//     prefixes; bun when the path holds /.bun/, pnpm when it holds /pnpm/);
//   - a cmd-shim script (npm on Windows: %APPDATA%\npm\codex.cmd; pnpm on
//     every OS) naming node_modules/@openai/codex (agentio.ShimTarget);
//   - a bun bin dir (~/.bun/bin/codex[.exe]) whose package is in
//     ~/.bun/install/global/node_modules/@openai/codex;
//   - a Volta bin dir (~/.volta/bin, %LOCALAPPDATA%\Volta\bin);
//   - a Homebrew cask (/Caskroom/) or formula (/Cellar/codex/<version>/);
//   - the standalone installer (packages/standalone).
//
// Never runs codex. ErrNotInstalled when no PATH entry has it. Disk · Read.
func (a *Agent) Install() (Install, error) {
	hits := a.host.LookPathAll(Binary)
	if len(hits) == 0 {
		return Install{}, fmt.Errorf("%w: %s", agentio.ErrNotInstalled, Binary)
	}
	in := Install{Install: agentio.Install{Path: hits[0], Method: agentio.InstallUnknown, Shadowed: hits[1:]}}
	real, err := filepath.EvalSymlinks(hits[0])
	if err != nil {
		real = hits[0]
	}
	in.RealPath = real
	detectInstall(&in, hits[0], real)
	if in.PackageRoot != "" && in.Version.IsZero() {
		in.Version = packageVersion(filepath.Join(in.PackageRoot, filePackageJSON))
	}
	return in, nil
}

// detectInstall fills Method and PackageRoot (and Version for a Homebrew
// formula) from the PATH hit and its resolved path. Disk · Read (shim and
// package dir checks).
func detectInstall(in *Install, hit, real string) {
	if root, ok := npmPackageRoot(real); ok {
		in.PackageRoot, in.Method = root, npmFamily(filepath.ToSlash(real))
		return
	}
	if target, ok := agentio.ShimTarget(hit); ok {
		if root, ok := npmPackageRoot(target); ok {
			in.PackageRoot, in.Method = root, npmFamily(filepath.ToSlash(target)+" "+filepath.ToSlash(hit))
			return
		}
	}
	binDir := filepath.Dir(hit)
	switch base := strings.ToLower(filepath.Base(filepath.Dir(binDir))); {
	case base == dirBunHome && strings.EqualFold(filepath.Base(binDir), dirBin):
		in.Method = agentio.InstallBun
		in.PackageRoot = filepath.Join(filepath.Dir(binDir), "install", "global", "node_modules", "@openai", "codex")
		return
	case (base == dirVoltaHome || base == dirVoltaHomeWindows) && strings.EqualFold(filepath.Base(binDir), dirBin):
		in.Method = agentio.InstallVolta
		in.PackageRoot = voltaPackageRoot(filepath.Dir(binDir))
		return
	}
	slashed := filepath.ToSlash(real)
	switch {
	case strings.Contains(slashed, fileCaskroom):
		in.Method = agentio.InstallBrew
	case strings.Contains(slashed, dirCellarCodex):
		in.Method = agentio.InstallBrew
		rest := slashed[strings.Index(slashed, dirCellarCodex)+len(dirCellarCodex):]
		v, _, _ := strings.Cut(rest, "/")
		in.Version, _ = agentio.ParseVersion(v)
	case strings.Contains(slashed, dirStandalone):
		in.Method = agentio.InstallStandalone
	}
}

// npmPackageRoot returns the node_modules/@openai/codex dir that p is
// inside, with OS separators. Pure.
func npmPackageRoot(p string) (string, bool) {
	slashed := filepath.ToSlash(p)
	i := strings.Index(slashed, nodeModulesMarker)
	if i < 0 {
		return "", false
	}
	return filepath.FromSlash(slashed[:i+len(nodeModulesMarker)]), true
}

// npmFamily names which npm-style manager owns a path (bun and pnpm keep
// their globals under their own dirs; everything else, nvm included, is
// npm). Pure.
func npmFamily(slashed string) agentio.InstallMethod {
	switch {
	case strings.Contains(slashed, bunMarker):
		return agentio.InstallBun
	case strings.Contains(strings.ToLower(slashed), pnpmMarker):
		return agentio.InstallPNPM
	}
	return agentio.InstallNPM
}

// voltaPackageRoot is where Volta keeps a global package: under
// tools/image/packages/<pkg>, in lib/node_modules on unix and node_modules
// on Windows (Volta docs, "Volta directory layout"). The first that holds a
// package.json wins; "" when neither does. Disk · Read.
func voltaPackageRoot(voltaHome string) string {
	base := filepath.Join(voltaHome, "tools", "image", "packages", "@openai", "codex")
	for _, sub := range [][]string{{"lib", "node_modules"}, {"node_modules"}} {
		p := filepath.Join(append(append([]string{base}, sub...), "@openai", "codex")...)
		if _, err := os.Stat(filepath.Join(p, filePackageJSON)); err == nil {
			return p
		}
	}
	return ""
}

func packageVersion(path string) agentio.Version {
	b, err := readFile(path)
	if err != nil {
		return agentio.Version{}
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return agentio.Version{}
	}
	v, _ := agentio.ParseVersion(pkg.Version)
	return v
}

// Update commands per method (tui/src/update_action.rs).
const (
	npmPkg      = "@openai/codex"
	toolNPM     = "npm"
	toolBun     = "bun"
	toolPNPM    = "pnpm"
	toolBrew    = "brew"
	toolVolta   = "volta"
	argInstall  = "install"
	argAdd      = "add"
	argGlobal   = "-g"
	argUpgrade  = "upgrade"
	argCask     = "--cask"
	whatUpdate  = "update"
	reasonNoCmd = "no update command for install method "
)

// UpdateCommand is the vendor's own update command for the codex on Host
// PATH: the install method comes from Install, and the package manager
// (npm, bun, pnpm, brew, volta) is resolved on Host PATH under Host.GOOS's
// rules (npm.cmd on Windows) like every binary the library names, never
// left as a bare name. A Homebrew formula install upgrades without --cask. ErrNotInstalled when codex or
// the package manager is not on PATH; ErrUnsupported for a standalone or
// unknown install (standalone re-runs the installer script, not modelled).
// Never run by the library. cost: Disk · Read (PATH walk, symlink and
// package.json read).
func (a *Agent) UpdateCommand() (agentio.Command, error) {
	in, err := a.Install()
	if err != nil {
		return agentio.Command{}, err
	}
	var tool string
	var args []string
	switch in.Method {
	case agentio.InstallNPM:
		tool, args = toolNPM, []string{argInstall, argGlobal, npmPkg}
	case agentio.InstallBun:
		tool, args = toolBun, []string{argInstall, argGlobal, npmPkg}
	case agentio.InstallPNPM:
		tool, args = toolPNPM, []string{argAdd, argGlobal, npmPkg}
	case agentio.InstallBrew:
		tool, args = toolBrew, []string{argUpgrade, argCask, Binary}
		if strings.Contains(filepath.ToSlash(in.RealPath), dirCellarCodex) {
			// a Homebrew formula (Cellar), not the cask: no --cask
			tool, args = toolBrew, []string{argUpgrade, Binary}
		}
	case agentio.InstallVolta:
		tool, args = toolVolta, []string{argInstall, npmPkg}
	default:
		return agentio.Command{}, agentio.UnsupportedError(ID, whatUpdate, reasonNoCmd+string(in.Method))
	}
	path, err := a.host.LookPath(tool)
	if err != nil {
		return agentio.Command{}, fmt.Errorf("codex: update with %s: %w", tool, err)
	}
	return a.host.NewCommand(path, args, ""), nil
}

// UpdateCache is <home>/version.json, written only by the TUI's startup
// check: LatestVersion can be older than the installed one (stale, not
// "an update is available").
type UpdateCache struct {
	LatestVersion    agentio.Version
	LastCheckedAt    time.Time
	DismissedVersion *agentio.Version
	Unknown          agentio.Unknown
}

type updateCacheWire struct {
	LatestVersion    string  `json:"latest_version"`
	LastCheckedAt    string  `json:"last_checked_at"`
	DismissedVersion *string `json:"dismissed_version"`
}

// UpdateCache reads version.json; found=false when absent. Disk · Read.
func (a *Agent) UpdateCache(h Home) (UpdateCache, bool, error) {
	b, err := readFile(a.Paths(h).VersionCache)
	if errors.Is(err, fs.ErrNotExist) {
		return UpdateCache{}, false, nil
	}
	if err != nil {
		return UpdateCache{}, false, fmt.Errorf("codex: read %s: %w", fileVersionCache, err)
	}
	var w updateCacheWire
	if err := json.Unmarshal(b, &w); err != nil {
		return UpdateCache{}, false, fmt.Errorf("codex: decode %s: %w", fileVersionCache, err)
	}
	lv, _ := agentio.ParseVersion(w.LatestVersion)
	uc := UpdateCache{LatestVersion: lv, LastCheckedAt: parseRFC3339(w.LastCheckedAt)}
	if w.DismissedVersion != nil {
		if dv, ok := agentio.ParseVersion(*w.DismissedVersion); ok {
			uc.DismissedVersion = &dv
		}
	}
	uc.Unknown, _ = agentio.ExtractUnknown(b, []string{"latest_version", "last_checked_at", "dismissed_version"})
	return uc, true, nil
}

// Runner is the Process door: every method runs the codex binary through
// Host.Exec and can refresh and rewrite auth.json (seen 2026-11-13), so all
// are MayTouchLogin and none is called implicitly (SPEC §4).
type Runner struct {
	a    *Agent
	core agentio.Runner
}

// Runner returns the Process door. Pure.
func (a *Agent) Runner() *Runner { return &Runner{a: a, core: a.core.Runner()} }

// BinaryVersion runs `codex --version`. Process · Read; not yet proven
// login-safe (CODEX_API.md §21).
func (r *Runner) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	return r.core.Version(ctx)
}

// run executes `codex <args>` on h and returns stdout; a non-zero exit is
// an error carrying the first line of stderr (no secrets are on argv).
func (r *Runner) run(ctx context.Context, h Home, args ...string) ([]byte, error) {
	cmd := r.a.plainCommand(h, args)
	res, err := r.core.Run(ctx, cmd, nil)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		first, _, _ := bytes.Cut(bytes.TrimSpace(res.Stderr), []byte{newline})
		return res.Stdout, fmt.Errorf("codex %s: exit %d: %s", strings.Join(args, " "), res.ExitCode, snippet(first))
	}
	return res.Stdout, nil
}

// LoginStatus is `codex login status`.
type LoginStatus struct {
	LoggedIn bool
	Method   AuthMode
	Text     string
}

// login status output fragments (0.162.1).
const (
	subStatus        = "status"
	statusChatGPT    = "ChatGPT"
	statusAPIKey     = "API key"
	statusNotLogged  = "Not logged in"
	featuresListCols = 3
)

// LoginStatus runs `codex login status`. Process · Read, MayTouchLogin.
func (r *Runner) LoginStatus(ctx context.Context, h Home) (LoginStatus, error) {
	cmd := r.a.plainCommand(h, []string{subLogin, subStatus})
	res, err := r.core.Run(ctx, cmd, nil)
	if err != nil {
		return LoginStatus{}, err
	}
	text := strings.TrimSpace(string(res.Stdout) + string(res.Stderr))
	st := LoginStatus{LoggedIn: res.ExitCode == 0 && !strings.Contains(text, statusNotLogged), Text: text}
	switch {
	case strings.Contains(text, statusChatGPT):
		st.Method = AuthChatGPT
	case strings.Contains(text, statusAPIKey):
		st.Method = AuthAPIKey
	}
	return st, nil
}

// FeatureStage is a `codex features list` stage.
type FeatureStage string

// FeatureStage values.
const (
	StageStable           FeatureStage = "stable"
	StageExperimental     FeatureStage = "experimental"
	StageUnderDevelopment FeatureStage = "under development"
	StageDeprecated       FeatureStage = "deprecated"
	StageRemoved          FeatureStage = "removed"
)

// IsKnown reports a known stage. Pure.
func (s FeatureStage) IsKnown() bool {
	switch s {
	case StageStable, StageExperimental, StageUnderDevelopment, StageDeprecated, StageRemoved:
		return true
	}
	return false
}

// FeatureFlag is one `codex features list` row (named FeatureFlag because
// Feature is the version-gated launch feature enum, SPEC §5.8).
type FeatureFlag struct {
	Name    string
	Stage   FeatureStage
	Enabled bool
}

// Features runs `codex features list` and parses `<name> <stage> <bool>`
// rows (stage may contain a space). Process · Read, MayTouchLogin.
func (r *Runner) Features(ctx context.Context, h Home) ([]FeatureFlag, error) {
	out, err := r.run(ctx, h, subFeatures, subList)
	if err != nil {
		return nil, err
	}
	return ParseFeatureList(out), nil
}

// ParseFeatureList parses `codex features list` output. Pure.
func ParseFeatureList(out []byte) []FeatureFlag {
	var flags []FeatureFlag
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < featuresListCols {
			continue
		}
		last := f[len(f)-1]
		if last != "true" && last != "false" {
			continue
		}
		flags = append(flags, FeatureFlag{Name: f[0], Stage: FeatureStage(strings.Join(f[1:len(f)-1], " ")), Enabled: last == "true"})
	}
	return flags
}

// debug models words.
const (
	subDebug    = "debug"
	subModels   = "models"
	flagBundled = "--bundled"
)

// BundledModels runs `codex debug models --bundled` and decodes its JSON
// catalog. Process · Read, MayTouchLogin.
func (r *Runner) BundledModels(ctx context.Context) (ModelCatalog, error) {
	out, err := r.run(ctx, r.a.DefaultHome(), subDebug, subModels, flagBundled)
	if err != nil {
		return ModelCatalog{}, err
	}
	cat, _, err := ParseModelCatalog(out)
	return cat, err
}
