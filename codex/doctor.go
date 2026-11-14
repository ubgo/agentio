package codex

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Codex-specific doctor check IDs (open set beside the neutral ones).
const (
	CheckProfileConflict agentio.CheckID = "profile-conflict"
	CheckStaleUpdate     agentio.CheckID = "update-cache"
	CheckConfigParse     agentio.CheckID = "config"
	// CheckKeyring reports a keyring login that could not be read (lookup
	// failure, unparsable value) or an ephemeral store; a readable keyring
	// login reports under the neutral CheckLogin.
	CheckKeyring agentio.CheckID = "keyring-login"
)

// Checks runs the doctor for h: binary, home, home env, API billing env
// (shared), installed version against the tested range, config.toml parse
// and drift, login presence and expiry, v2 profile files that collide with
// legacy profiles (Codex refuses that -p), and the stale update cache. Never calls a
// Runner method. Disk · Read, plus one keychain read (Process · Read via
// Host.Secrets, MayTouchLogin false) for keyring and auto homes, as
// Claude's doctor does.
func (a *Agent) Checks(ctx context.Context, h Home) []agentio.Check {
	out := a.core.Checks(ctx, h.Neutral())
	in, err := a.Install()
	if err == nil {
		out = append(out, agentio.VersionCheck(Compat(), in.Version))
	} else {
		out = append(out, agentio.VersionCheck(Compat(), agentio.Version{}))
	}
	cfg, warns, cerr := a.ConfigFileWarnings(h)
	switch {
	case cerr != nil:
		out = append(out, agentio.Check{ID: CheckConfigParse, Status: agentio.CheckFail, Message: cerr.Error()})
	case len(warns) > 0:
		out = append(out, agentio.Check{ID: CheckConfigParse, Status: agentio.CheckWarn, Message: fmt.Sprintf("%d config.toml key(s) changed type; kept raw", len(warns))})
	default:
		out = append(out, agentio.Check{ID: CheckConfigParse, Status: agentio.CheckOK, Message: a.Paths(h).Config})
	}
	if c, ok := a.providerKeyCheck(cfg); ok {
		out = append(out, c)
	}
	out = append(out, a.loginCheck(ctx, h))
	if c, ok := a.profileCheck(h, cfg); ok {
		out = append(out, c)
	}
	if uc, ok, err := a.UpdateCache(h); err == nil && ok && in.Version.Major+in.Version.Minor+in.Version.Patch > 0 {
		if uc.LatestVersion.Compare(in.Version) < 0 {
			out = append(out, agentio.Check{ID: CheckStaleUpdate, Status: agentio.CheckInfo,
				Message: "version.json latest " + uc.LatestVersion.String() + " is older than installed " + in.Version.String() + " (stale cache, not an update)"})
		}
	}
	return out
}

// loginCheck reads the login where cli_auth_credentials_store puts it.
// Like Claude's doctor (claude/doctor.go loginChecks → Credential) it
// reads the keychain for keyring and auto homes: Process · Read via
// Host.Secrets, MayTouchLogin false.
func (a *Agent) loginCheck(ctx context.Context, h Home) agentio.Check {
	id, err := a.IdentityContext(ctx, h)
	var kerr *KeyringError
	switch {
	case errors.Is(err, agentio.ErrUnsupported):
		return agentio.Check{ID: CheckKeyring, Status: agentio.CheckInfo, Message: err.Error()}
	case errors.As(err, &kerr):
		return agentio.Check{ID: CheckKeyring, Status: agentio.CheckWarn, Message: kerr.Error()}
	case err != nil:
		return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckFail, Message: err.Error()}
	case !id.LoggedIn:
		cmd := a.plainCommand(h, []string{subLogin})
		return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckWarn, Message: "not logged in" + storeWhere(id), Fix: &cmd}
	case !id.AccessExpiresAt.IsZero() && !a.now().IsZero() && !a.now().Before(id.AccessExpiresAt):
		return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckInfo, Message: "access token expired at " + id.AccessExpiresAt.Format(time.RFC3339) + "; codex refreshes it on its next run"}
	}
	return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckOK, Message: "logged in (" + string(id.Mode) + ", " + string(id.CredentialStore) + ")"}
}

// storeWhere says where a missing login was looked for (entry names only).
// Pure.
func storeWhere(id Identity) string {
	switch id.CredentialStore {
	case StoreKeyring:
		return ": no keyring entry " + id.Keyring.String()
	case StoreAuto:
		return ": no keyring entry " + id.Keyring.String() + " and no " + fileAuth
	}
	return ""
}

// InventoryEntry is one path of a home.
type InventoryEntry struct {
	Name    string // Paths field name, or the entry name for unknown entries
	Path    string
	Exists  bool
	IsDir   bool
	Size    int64 // bytes of a file; 0 for dirs (not walked)
	ModTime time.Time
}

// Inventory says which Paths exist and lists top-level entries Paths does
// not name (desktop state, new vendor files) by name only. Disk · Read.
type Inventory struct {
	Home    string
	Entries []InventoryEntry
	Unknown []string // top-level names not covered by Paths
}

// Inventory stats every known path of h and names the rest. Disk · Read.
func (a *Agent) Inventory(h Home) (Inventory, error) {
	p := a.Paths(h)
	inv := Inventory{Home: p.Home}
	named := []struct{ name, path string }{
		{"Config", p.Config}, {"AgentsMD", p.AgentsMD}, {"Rules", p.Rules}, {"Hooks", p.Hooks},
		{"Auth", p.Auth}, {"Credentials", p.Credentials}, {"InstallationID", p.InstallationID},
		{"Sessions", p.Sessions}, {"ArchivedSessions", p.ArchivedSessions}, {"SessionIndex", p.SessionIndex},
		{"History", p.History}, {"TranscriptionHistory", p.TranscriptionHistory}, {"ShellSnapshots", p.ShellSnapshots},
		{"Attachments", p.Attachments}, {"StateDB", p.StateDB}, {"ThreadHistoryDB", p.ThreadHistoryDB},
		{"GoalsDB", p.GoalsDB}, {"QueueDB", p.QueueDB}, {"MemoriesDB", p.MemoriesDB}, {"LogsDB", p.LogsDB},
		{"ModelsCache", p.ModelsCache}, {"VersionCache", p.VersionCache}, {"Skills", p.Skills},
		{"VendorSkills", p.VendorSkills}, {"Plugins", p.Plugins}, {"Memories", p.Memories}, {"Cache", p.Cache},
		{"Log", p.Log}, {"Tmp", p.Tmp}, {"DesktopState", p.DesktopState}, {"DesktopDB", p.DesktopDB},
		{"IPCSocket", p.IPCSocket}, {"ChatProcesses", p.ChatProcesses}, {"ComputerUse", p.ComputerUse},
		{"Browser", p.Browser}, {"Visualizations", p.Visualizations}, {"ThreadWriterLocks", p.ThreadWriterLocks},
	}
	for _, m := range p.Markers {
		named = append(named, struct{ name, path string }{"Marker", m})
	}
	known := map[string]bool{}
	for _, n := range named {
		e := InventoryEntry{Name: n.name, Path: n.path}
		if st, err := os.Lstat(n.path); err == nil {
			e.Exists, e.IsDir, e.ModTime = true, st.IsDir(), st.ModTime()
			if !st.IsDir() {
				e.Size = st.Size()
			}
		}
		inv.Entries = append(inv.Entries, e)
		known[topLevel(p.Home, n.path)] = true
	}
	entries, err := os.ReadDir(p.Home)
	if errors.Is(err, fs.ErrNotExist) {
		return inv, nil
	}
	if err != nil {
		return inv, fmt.Errorf("codex: read %s: %w", p.Home, err)
	}
	profiles, _ := a.ProfileFiles(h)
	for _, e := range entries {
		name := e.Name()
		if known[name] || isProfileFile(name, profiles) || isSidecar(name, known) {
			continue
		}
		inv.Unknown = append(inv.Unknown, name)
	}
	sort.Strings(inv.Unknown)
	return inv, nil
}

// SQLite sidecars and the desktop state backup are known by their base.
func sidecarSuffixes() []string { return []string{"-wal", "-shm", ".bak"} }

func isSidecar(name string, known map[string]bool) bool {
	for _, s := range sidecarSuffixes() {
		if base, ok := strings.CutSuffix(name, s); ok && base != "" && known[base] {
			return true
		}
	}
	return false
}

// isProfileFile reports a v2 profile file <name>.config.toml of profiles.
func isProfileFile(name string, profiles []string) bool {
	base, ok := strings.CutSuffix(name, fileProfileSuffix)
	return ok && slices.Contains(profiles, base)
}

// topLevel returns the first path element of p under home ("" outside it).
func topLevel(home, p string) string {
	rel, err := filepath.Rel(home, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return first
}

// profileCheck warns when config.toml has legacy profiles: `codex -p NAME`
// (profile v2, since 0.131.0, rust-v0.131.0 release notes #22647) refuses a
// NAME that is also a legacy `[profiles.NAME]` table or the root
// `profile = "NAME"` selector (config/src/loader/mod.rs:303-320 at
// rust-v0.162.1). A v2 file with a legacy name is the live conflict.
func (a *Agent) profileCheck(h Home, cfg Config) (agentio.Check, bool) {
	legacy := cfg.LegacyProfileNames()
	if len(legacy) == 0 {
		return agentio.Check{}, false
	}
	v2, _ := a.ProfileFiles(h)
	var clash []string
	for _, n := range legacy {
		if slices.Contains(v2, n) {
			clash = append(clash, n)
		}
	}
	msg := "legacy profiles in config.toml: " + strings.Join(legacy, ", ") +
		"; `codex -p NAME` (profile v2, since 0.131.0) refuses while a legacy [profiles.NAME] table or `profile = \"NAME\"` exists — move each into <home>/NAME.config.toml and remove the legacy table/selector"
	if len(clash) > 0 {
		msg += "; v2 file(s) already collide: " + strings.Join(clash, ", ")
	}
	return agentio.Check{ID: CheckProfileConflict, Status: agentio.CheckWarn, Message: msg}, true
}

// providerKeyCheck warns (api-billing-env) when a variable that is set in
// Host.Env is the env_key of a configured model provider: that provider
// then authenticates with the key and bills per token. This is how
// OPENAI_API_KEY affects Codex at all (rust-v0.162.1 auth never reads it).
// Names only, never the value. Pure.
func (a *Agent) providerKeyCheck(cfg Config) (agentio.Check, bool) {
	var hits []string
	for _, name := range slices.Sorted(maps.Keys(cfg.ModelProviders)) {
		key, ok := cfg.ModelProviders[name].EnvKey.Get()
		if !ok || key == "" {
			continue
		}
		if v, set := a.host.Getenv(key); set && v != "" {
			hits = append(hits, key+" (model provider "+name+")")
		}
	}
	if len(hits) == 0 {
		return agentio.Check{}, false
	}
	return agentio.Check{ID: agentio.CheckAPIBillingEnv, Status: agentio.CheckWarn, Message: strings.Join(hits, ", ") + " set: that provider bills per token"}, true
}
