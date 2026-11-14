package agentio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Markers InstallFromPath reads in a slashed path. Each is a layout the
// package manager documents: Homebrew keeps formula versions under
// Cellar/<formula>/<version>/, Bun global installs under ~/.bun/, pnpm
// under a pnpm/ (global) or .pnpm/ (store) dir, Volta under ~/.volta or
// %LOCALAPPDATA%\Volta, and native installers (Claude Code) one dir per
// version under versions/<version>.
const (
	markerCellar   = "/Cellar/"
	markerBun      = "/.bun/"
	markerPNPM     = "/pnpm/"
	markerPNPMDot  = "/.pnpm/"
	markerVolta    = "/.volta/"
	markerVoltaWin = "/volta/"
	markerVersions = "/versions/"
	// packageJSONName is npm's manifest file.
	packageJSONName = "package.json"
	// maxPackageDepth bounds the walk up from the binary to its package
	// root: an npm bin script sits at most a few dirs below it
	// (bin/cli.js, dist/bundle/cli.js, bundle/gemini.js).
	maxPackageDepth = 6
)

// InstallFromPath reads an install from disk, never running the binary
// (it never runs the binary). hits are the PATH hits for the binary in order (Host.LookPathAll);
// packages are the npm package names the agent ships as ("@google/gemini-cli").
//
// It resolves the first hit through symlinks (and a Windows npm .cmd shim
// through ShimTarget), then takes the version from, in order:
//   - the package.json of a listed package found by walking up from the
//     resolved file (npm, pnpm, Bun, Volta, Homebrew formulae that install
//     through npm), with Method from the path (bun, pnpm, volta, brew, npm);
//   - a Homebrew Cellar/<formula>/<version>/ dir in the path;
//   - a native versions/<version> file or dir (the Claude Code installer).
//
// No hit is ErrNotInstalled; an install whose version is in none of these
// places has Method unknown and a zero Version (an answer, not an error).
// Disk · Read.
func InstallFromPath(hits, packages []string) (Install, error) {
	if len(hits) == 0 {
		return Install{}, ErrNotInstalled
	}
	in := Install{Path: hits[0], Method: InstallUnknown, Shadowed: slices.Clone(hits[1:])}
	real, err := filepath.EvalSymlinks(hits[0])
	if err != nil {
		real = hits[0]
	}
	in.RealPath = real
	cands := []string{real}
	if target, ok := ShimTarget(hits[0]); ok {
		cands = append(cands, target)
	}
	for _, c := range cands {
		if v, ok := packageVersionAbove(c, packages); ok {
			in.Version = v
			in.Method = methodFromPath(filepath.ToSlash(c))
			return in, nil
		}
	}
	slashed := filepath.ToSlash(real)
	if v, ok := versionAfter(slashed, markerCellar, 1); ok {
		in.Method, in.Version = InstallBrew, v
		return in, nil
	}
	if v, ok := versionAfter(slashed, markerVersions, 0); ok {
		in.Method, in.Version = InstallNative, v
	}
	return in, nil
}

// packageVersionAbove walks up from file for a package.json whose name is
// one of packages and returns its version. Disk · Read.
func packageVersionAbove(file string, packages []string) (Version, bool) {
	if len(packages) == 0 {
		return Version{}, false
	}
	dir := filepath.Dir(file)
	for range maxPackageDepth {
		if name, v, ok := readPackageJSON(filepath.Join(dir, packageJSONName)); ok && slices.Contains(packages, name) {
			return v, !v.IsZero()
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return Version{}, false
}

// packageManifest is the part of package.json InstallFromPath reads.
type packageManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// readPackageJSON reads a package.json's name and version. Disk · Read.
func readPackageJSON(p string) (string, Version, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", Version{}, false
	}
	var m packageManifest
	if err := json.Unmarshal(StripBOM(b), &m); err != nil {
		return "", Version{}, false
	}
	v, _ := ParseVersion(m.Version)
	return m.Name, v, true
}

// methodFromPath names the package manager that owns a slashed path. Pure.
func methodFromPath(slashed string) InstallMethod {
	lower := strings.ToLower(slashed)
	switch {
	case strings.Contains(slashed, markerCellar):
		return InstallBrew
	case strings.Contains(lower, markerBun):
		return InstallBun
	case strings.Contains(lower, markerPNPM), strings.Contains(lower, markerPNPMDot):
		return InstallPNPM
	case strings.Contains(lower, markerVolta), strings.Contains(lower, markerVoltaWin):
		return InstallVolta
	}
	return InstallNPM
}

// versionAfter parses the version in the path element skip elements after
// marker: Cellar/<formula>/<version> is skip 1, versions/<version> skip 0.
// Pure.
func versionAfter(slashed, marker string, skip int) (Version, bool) {
	i := strings.LastIndex(slashed, marker)
	if i < 0 {
		return Version{}, false
	}
	parts := strings.Split(slashed[i+len(marker):], "/")
	if len(parts) <= skip {
		return Version{}, false
	}
	el := parts[skip]
	if j := strings.IndexByte(el, '_'); j > 0 {
		el = el[:j] // a Homebrew revision: 0.68.0_1
	}
	v, ok := ParseVersion(el)
	if !ok || v.String() != strings.TrimPrefix(el, "v") {
		return Version{}, false
	}
	return v, true
}
