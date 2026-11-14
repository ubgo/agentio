package claude

import (
	"encoding/json"
	"fmt"
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
	nativeVersionsDir = "/.local/share/claude/versions/"
	npmPackageDir     = "node_modules/@anthropic-ai/claude-code"
	caskroomDir       = "/Caskroom/"
	packageJSONFile   = "package.json"
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

// ParseVersion parses "2.1.323 (Claude Code)". cost: Pure.
func ParseVersion(s string) (agentio.Version, error) {
	v, ok := agentio.ParseVersion(s)
	if !ok {
		return agentio.Version{}, fmt.Errorf("claude: no version in %q", s)
	}
	return v, nil
}

// methodAndVersion derives the install method and on-disk version from the
// resolved binary path.
func methodAndVersion(real string) (agentio.InstallMethod, agentio.Version) {
	slash := filepath.ToSlash(real)
	switch {
	case strings.Contains(slash, nativeVersionsDir):
		v, _ := agentio.ParseVersion(filepath.Base(real))
		return agentio.InstallNative, v
	case strings.Contains(slash, npmPackageDir):
		i := strings.Index(slash, npmPackageDir)
		pkg := filepath.FromSlash(slash[:i+len(npmPackageDir)])
		return InstallNPMGlobal, packageVersion(filepath.Join(pkg, packageJSONFile))
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
	inst.Method, inst.Version = methodAndVersion(inst.RealPath)
	pathList, _ := a.host.Getenv(pathEnv)
	for _, p := range agentio.LookPathAll(pathList, Binary) {
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
