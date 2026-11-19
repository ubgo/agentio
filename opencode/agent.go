// Package opencode is agentio's typed API for the opencode CLI (npm
// opencode-ai, the native installer and Homebrew): install and version,
// the data dir (the home, under the XDG data dir), the legacy JSON session
// storage with its turns, process classification, the launch command, and
// the doctor.
//
// opencode keeps every session since 1.2.0 in a SQLite database
// (opencode.db in the data dir). This package is stdlib only, so it reads
// the legacy storage/ tree that 1.2.0 migrated (sessions written before
// that release); the database is read by the separate module
// github.com/ubgo/agentio/opencode/store, which carries the SQLite driver.
//
// Every shape is recorded from opencode 1.18.30 and the legacy files of
// 0.14.7 and 1.1.14 on a real machine (values replaced in the fixtures).
// The package never logs in, never reads auth.json, and writes nothing
// opencode owns.
package opencode

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
)

// ID is opencode's agent id.
const ID agentio.ID = "opencode"

// Agent is opencode on one Host. Like claude.Agent it is not an
// agentio.Agent itself: Neutral() returns the L1/L2 view, so vendor
// methods keep their rich types.
type Agent struct {
	host agentio.Host
	// core is the shared L1 implementation built from Spec(): PATH lookup,
	// Command assembly, process matching, the base stop plan.
	core *agentio.Core
}

// New returns the opencode agent for h. cost: Pure; no I/O. Spec() is a
// constant that TestSpecValidates proves valid, so NewCore cannot fail
// here; a nil core would only follow an edit that breaks that test.
func New(h agentio.Host) *Agent {
	core, err := agentio.NewCore(Spec(), h)
	if err != nil {
		core = nil
	}
	return &Agent{host: h, core: core}
}

// Host returns the host the agent reads through. cost: Pure.
func (a *Agent) Host() agentio.Host { return a.host }

// ID returns opencode.ID. cost: Pure.
func (a *Agent) ID() agentio.ID { return ID }

// Name returns "opencode". cost: Pure.
func (a *Agent) Name() string { return displayName }

// homeMarkers are entries opencode writes into its data dir: the database
// (every start since 1.2.0) and the legacy storage/ tree. Neither name is
// written by another agent into its home, so no marker is generic.
// auth.json is left out: Codex and pi homes hold one too, and a data dir
// that has run opencode at all holds the database.
func homeMarkers() []string { return []string{dbFile, storageDir} }

// HomeMarkers lists the marker entries of a data dir. cost: Pure.
func HomeMarkers() []string { return homeMarkers() }

// autonomyArgs maps the levels opencode can express. Only full has a flag
// (--auto: "auto-approve permissions that are not explicitly denied"); the
// other levels are permission rules in opencode.json with no flag, so they
// are refused rather than mapped to a guess. AutonomyDefault adds nothing,
// so the configured permissions apply.
func autonomyArgs() map[agentio.Autonomy][]string {
	return map[agentio.Autonomy][]string{
		agentio.AutonomyFull: {flagAuto},
	}
}

// Spec returns the data opencode's core is built from. HomeEnv is empty:
// opencode has no home variable of its own. Its data dir follows
// XDG_DATA_HOME, a base dir every XDG tool shares, so the default home is
// resolved from the Host env (DefaultHome) and a launch on another data
// dir sets XDG_DATA_HOME itself (Launch). Headless runs need the `run`
// subcommand in front, which a template cannot place, so Launch builds
// them. cost: Pure.
func Spec() agentio.Spec {
	return agentio.Spec{
		ID: ID, Name: displayName, Binary: Binary,
		DefaultHome:  filepath.Join(defaultDataBase, appDir),
		HomeMarkers:  homeMarkers(),
		NewArgs:      []string{placeholderArgs},
		ResumeArgs:   []string{flagSession, placeholderID, placeholderArgs},
		ModelArgs:    []string{flagModel, placeholderModel},
		PromptArgs:   []string{flagPrompt, placeholderPrompt},
		AutonomyArgs: autonomyArgs(),
		VersionArgs:  []string{flagVersion},
		Packages:     []string{Package},
		Process: agentio.ProcessMatch{
			Basenames:       []string{Binary, Binary + exeSuffix},
			ScriptBasenames: []string{Binary},
		},
	}
}

// exeSuffix is the Windows executable extension.
const exeSuffix = ".exe"

// Spec returns opencode.Spec(). cost: Pure.
func (a *Agent) Spec() agentio.Spec { return Spec() }

// Home is one opencode data dir (<XDG data dir>/opencode). It wraps the
// neutral handle so every vendor method is compile-checked against
// opencode.
type Home struct{ h agentio.Home }

// Neutral returns the shared handle. cost: Pure.
func (h Home) Neutral() agentio.Home { return h.h }

// Dir is the data dir. cost: Pure.
func (h Home) Dir() string { return h.h.Dir() }

// IsDefault reports the data dir the Host env selects. cost: Pure.
func (h Home) IsDefault() bool { return h.h.IsDefault() }

// Name is "personal" for the default home, else the name of the XDG data
// base the dir sits in (/srv/alt/opencode is "alt"). cost: Pure.
func (h Home) Name() string { return h.h.Name() }

// Target is a plain launch target for this home. cost: Pure.
func (h Home) Target() agentio.Target { return h.h.Target() }

// Path joins elem under the data dir. cost: Pure.
func (h Home) Path(elem ...string) string { return h.h.Path(elem...) }

// String is "opencode:<dir>". cost: Pure.
func (h Home) String() string { return h.h.String() }

// MarshalJSON encodes the neutral handle. cost: Pure.
func (h Home) MarshalJSON() ([]byte, error) { return h.h.MarshalJSON() }

// dataBase is the XDG data base dir: $XDG_DATA_HOME when it is set to an
// absolute path, else ~/.local/share. opencode itself uses any non-empty
// value as is; the XDG spec says a relative value is invalid and must be
// ignored, and a relative value would resolve against opencode's working
// dir, which differs per launch, so it is ignored here. cost: Pure.
func (a *Agent) dataBase() string {
	if v, ok := a.host.Getenv(XDGDataHomeEnv); ok && filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	return filepath.Join(a.host.UserHome, defaultDataBase)
}

// configBase is the XDG config base dir, by the same rule. cost: Pure.
func (a *Agent) configBase() string {
	if v, ok := a.host.Getenv(XDGConfigHomeEnv); ok && filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	return filepath.Join(a.host.UserHome, defaultConfigBase)
}

// DataDir is the data dir the Host env selects: $XDG_DATA_HOME/opencode,
// else ~/.local/share/opencode. cost: Pure.
func (a *Agent) DataDir() string { return filepath.Join(a.dataBase(), appDir) }

// ConfigDir is opencode's global config dir (opencode.json, agents,
// commands, plugins): $XDG_CONFIG_HOME/opencode, else ~/.config/opencode.
// Config is not modelled yet; the path is for callers. cost: Pure.
func (a *Agent) ConfigDir() string { return filepath.Join(a.configBase(), appDir) }

// DefaultHome is the data dir the Host env selects (DataDir), marked
// default: a launch on it leaves XDG_DATA_HOME as inherited. cost: Pure.
func (a *Agent) DefaultHome() Home {
	return Home{h: agentio.NewHome(ID, a.DataDir(), true, agentio.DefaultHomeName)}
}

// HomeFromEnv is the default home when XDG_DATA_HOME selects it (set to an
// absolute path); false when the variable is absent, so the default is the
// fixed ~/.local/share/opencode. cost: Pure.
func (a *Agent) HomeFromEnv() (Home, bool) {
	if v, ok := a.host.Getenv(XDGDataHomeEnv); !ok || !filepath.IsAbs(v) {
		return Home{}, false
	}
	return a.DefaultHome(), true
}

// HomeAt returns the home at dir, the data dir itself (the folder holding
// storage/ and opencode.db); relative dirs resolve against UserHome. The
// default data dir is the default home. cost: Pure.
func (a *Agent) HomeAt(dir string) Home {
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(a.host.UserHome, dir)
	}
	dir = filepath.Clean(dir)
	if dir == a.DataDir() {
		return a.DefaultHome()
	}
	name := strings.TrimPrefix(filepath.Base(filepath.Dir(dir)), ".")
	return Home{h: agentio.NewHome(ID, dir, false, name)}
}

// HomeOf converts a neutral home; ErrForeignHome for another agent's.
// cost: Pure.
func (a *Agent) HomeOf(n agentio.Home) (Home, error) {
	if err := agentio.CheckOwner(ID, n); err != nil {
		return Home{}, err
	}
	return Home{h: n}, nil
}

// Homes lists the default data dir when it holds a marker. opencode has
// one data dir per XDG data base and no naming convention for others;
// other homes come from the caller (HomeAt). cost: Disk · Read.
func (a *Agent) Homes() ([]Home, error) {
	def := a.DefaultHome()
	ok, err := a.IsHome(def.Dir())
	if err != nil {
		return nil, fmt.Errorf("opencode: homes: %w", err)
	}
	if !ok {
		return nil, nil
	}
	return []Home{def}, nil
}

// IsHome reports whether dir holds opencode.db or storage/. cost: Disk · Read.
func (a *Agent) IsHome(dir string) (bool, error) { return a.core.HasHomeMarker(dir) }

// LookPath finds opencode by walking PATH from Host.Env, never a shell.
// cost: Disk · Read.
func (a *Agent) LookPath() (string, error) { return a.core.LookPath() }

// Install finds opencode on PATH and reads its version from the npm
// package.json (opencode-ai) or the Homebrew Cellar path, never running
// it. The native installer (~/.opencode/bin/opencode) leaves no version
// on disk: Version is zero then, and Runner().BinaryVersion asks the
// binary. cost: Disk · Read.
func (a *Agent) Install() (agentio.Install, error) { return a.core.Install() }

// Runner is the Process door: commands that run the opencode binary.
type Runner struct {
	agentio.Runner
}

// Runner returns the Process door. Holding it is the opt-in to running the
// binary. cost: Pure.
func (a *Agent) Runner() Runner { return Runner{Runner: a.core.Runner()} }

// BinaryVersion runs `opencode --version`. cost: Process · Read.
func (r Runner) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	return r.Version(ctx)
}
