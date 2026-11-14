package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Install method values derived from the binary's real path. "native" and
// "npm-global" are verified on disk (this server: ~/.local/bin/claude →
// ~/.local/share/claude/versions/2.1.323; the owner's Mac: Homebrew npm
// package); "homebrew-cask" is from docs.
const (
	InstallNPMGlobal    agentio.InstallMethod = "npm-global"
	InstallHomebrewCask agentio.InstallMethod = "homebrew-cask"
)

// Real-path shapes that tell the install method.
const (
	nativeVersionsDir   = "/.local/share/claude/versions/"
	npmPackageDir       = "node_modules/@anthropic-ai/claude-code"
	caskroomDir         = "/Caskroom/"
	packageJSONFile     = "package.json"
	nativeBinDir        = "/.local/bin/"
	windowsExeExt       = ".exe"
	bunDirMarker        = "/.bun/"
	pnpmDirMarker       = "/pnpm/"
	binDirName          = "bin"
	bunHomeDir          = ".bun"
	voltaHomeDir        = ".volta"
	voltaHomeDirWindows = "volta"
)

// Install is how Claude Code is installed. Version comes from disk (the
// native versions dir name or the npm package.json), never from running
// the binary.
type Install struct {
	agentio.Install
	// RawMethod is ~/.claude.json installMethod verbatim ("native", "global").
	RawMethod          string
	Channel            UpdateChannel
	AutoUpdates        *bool
	ProtectedForNative bool
	LastUpdate         *UpdateResult
}

// Neutral returns the shared install facts. cost: Pure.
func (i Install) Neutral() agentio.Install { return i.Install }

// UpdateResult is .last-update-result.json.
type UpdateResult struct {
	At        time.Time
	Path      string
	Outcome   string
	Status    string
	From, To  agentio.Version
	ErrorCode *string
	Unknown   agentio.Unknown
}

type wireUpdateResult struct {
	Timestamp   time.Time `json:"timestamp"`
	Path        string    `json:"path"`
	Outcome     string    `json:"outcome"`
	Status      string    `json:"status"`
	VersionFrom string    `json:"version_from"`
	VersionTo   string    `json:"version_to"`
	ErrorCode   *string   `json:"error_code"`
}

// LastUpdate reads <home>/.last-update-result.json; nil when absent.
// cost: Disk · Read.
func (a *Agent) LastUpdate(h Home) (*UpdateResult, error) {
	p := filepath.Join(h.Dir(), lastUpdateFile)
	data, found, err := readFileBounded(p, maxResponseBytes)
	if err != nil || !found {
		return nil, err
	}
	var w wireUpdateResult
	unknown, _, err := decodeLenient(lastUpdateFile, data, reflect.TypeFor[wireUpdateResult](), func(b []byte) error { return json.Unmarshal(b, &w) })
	if err != nil {
		return nil, err
	}
	return &UpdateResult{At: w.Timestamp, Path: w.Path, Outcome: w.Outcome, Status: w.Status,
		From: agentio.MustVersion(w.VersionFrom), To: agentio.MustVersion(w.VersionTo), ErrorCode: w.ErrorCode, Unknown: unknown}, nil
}

// ParseVersion parses "2.1.323 (Claude Code)". It has the root
// agentio.ParseVersion shape: ok is false when s holds no version.
// cost: Pure.
func ParseVersion(s string) (agentio.Version, bool) { return agentio.ParseVersion(s) }

// detectInstall derives the install method and on-disk version from the
// PATH hit and its resolved path, on every OS:
//   - a path inside the native versions dir (the ~/.local/bin/claude
//     symlink on Linux and macOS);
//   - a path inside node_modules/@anthropic-ai/claude-code (npm, nvm; bun
//     and pnpm by their dirs), directly or through a cmd-shim script (npm
//     on Windows: %APPDATA%\npm\claude.cmd; pnpm everywhere);
//   - a bun or Volta bin dir;
//   - a copied (not symlinked) native binary in ~/.local/bin (the native
//     installer's Windows layout, %USERPROFILE%\.local\bin\claude.exe):
//     native, with the newest version in ~/.local/share/claude/versions.
//     That layout is inferred from the Linux/macOS one, not observed on
//     Windows (docs/platforms.md says so);
//   - a Homebrew cask.
//
// Disk · Read (shim, versions dir and package.json reads).
func detectInstall(hit, real string) (agentio.InstallMethod, agentio.Version) {
	if m, v := methodAndVersion(real); m != agentio.InstallUnknown {
		return m, v
	}
	if target, ok := agentio.ShimTarget(hit); ok {
		if m, v := methodAndVersion(target); m != agentio.InstallUnknown {
			if m == InstallNPMGlobal && strings.Contains(strings.ToLower(filepath.ToSlash(hit)), pnpmDirMarker) {
				m = agentio.InstallPNPM
			}
			return m, v
		}
	}
	binDir := filepath.Dir(hit)
	home := filepath.Dir(binDir)
	if strings.EqualFold(filepath.Base(binDir), binDirName) {
		switch strings.ToLower(filepath.Base(home)) {
		case bunHomeDir:
			return agentio.InstallBun, packageVersion(filepath.Join(home, "install", "global", "node_modules", "@anthropic-ai", "claude-code", packageJSONFile))
		case voltaHomeDir, voltaHomeDirWindows:
			return agentio.InstallVolta, voltaVersion(home)
		}
	}
	if hit == real && strings.Contains(filepath.ToSlash(hit), nativeBinDir) {
		return agentio.InstallNative, newestNativeVersion(filepath.Join(home, "share", "claude", "versions"))
	}
	return agentio.InstallUnknown, agentio.Version{}
}

// newestNativeVersion is the highest version named by an entry of the
// native versions dir (~/.local/share/claude/versions; "2.1.325" or
// "2.1.325.exe"); zero when none parses. Disk · Read.
func newestNativeVersion(dir string) agentio.Version {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return agentio.Version{}
	}
	var best agentio.Version
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), windowsExeExt)
		if v, ok := agentio.ParseVersion(name); ok && v.Compare(best) > 0 {
			best = v
		}
	}
	return best
}

// voltaVersion reads the claude-code package.json from Volta's image dir
// (lib/node_modules on unix, node_modules on Windows). Disk · Read.
func voltaVersion(voltaHome string) agentio.Version {
	base := filepath.Join(voltaHome, "tools", "image", "packages", "@anthropic-ai", "claude-code")
	for _, sub := range []string{filepath.Join("lib", "node_modules"), "node_modules"} {
		if v := packageVersion(filepath.Join(base, sub, "@anthropic-ai", "claude-code", packageJSONFile)); !v.IsZero() {
			return v
		}
	}
	return agentio.Version{}
}

// methodAndVersion derives the install method and on-disk version from a
// resolved binary or script path alone. Pure apart from the package.json
// read.
func methodAndVersion(real string) (agentio.InstallMethod, agentio.Version) {
	slash := filepath.ToSlash(real)
	switch {
	case strings.Contains(slash, nativeVersionsDir):
		v, _ := agentio.ParseVersion(filepath.Base(real))
		return agentio.InstallNative, v
	case strings.Contains(slash, npmPackageDir):
		i := strings.Index(slash, npmPackageDir)
		pkg := filepath.FromSlash(slash[:i+len(npmPackageDir)])
		m := InstallNPMGlobal
		switch {
		case strings.Contains(slash, bunDirMarker):
			m = agentio.InstallBun
		case strings.Contains(strings.ToLower(slash), pnpmDirMarker):
			m = agentio.InstallPNPM
		}
		return m, packageVersion(filepath.Join(pkg, packageJSONFile))
	case strings.Contains(slash, caskroomDir):
		return InstallHomebrewCask, agentio.Version{}
	}
	return agentio.InstallUnknown, agentio.Version{}
}

// packageVersion reads "version" from an npm package.json; zero when absent.
func packageVersion(path string) agentio.Version {
	data, found, err := readFileBounded(path, maxResponseBytes)
	if err != nil || !found {
		return agentio.Version{}
	}
	var w struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &w) != nil {
		return agentio.Version{}
	}
	return agentio.MustVersion(w.Version)
}

// Install reports the PATH hit, its real path, method, on-disk version,
// other PATH entries named claude, and the default home's update facts.
// ErrNotInstalled when claude is not on PATH. cost: Disk · Read (PATH,
// symlinks, ~/.claude.json, package.json); never runs claude.
func (a *Agent) Install() (Install, error) {
	path, err := a.LookPath()
	if err != nil {
		return Install{}, err
	}
	inst := Install{Install: agentio.Install{Path: path, RealPath: path}}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		inst.RealPath = real
	}
	inst.Method, inst.Version = detectInstall(path, inst.RealPath)
	for _, p := range a.host.LookPathAll(Binary) {
		if p != path {
			inst.Shadowed = append(inst.Shadowed, p)
		}
	}
	def := a.DefaultHome()
	if g, err := a.GlobalConfig(def); err == nil {
		inst.RawMethod, inst.AutoUpdates = g.InstallMethod, g.AutoUpdates
		inst.ProtectedForNative = g.AutoUpdatesProtectedForNative != nil && *g.AutoUpdatesProtectedForNative
	}
	if l, err := a.UserSettings(def); err == nil {
		inst.Channel = l.Settings.AutoUpdatesChannel
	}
	if lu, err := a.LastUpdate(def); err == nil {
		inst.LastUpdate = lu
	}
	return inst, nil
}

// fileExists reports a present path (Lstat).
func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
