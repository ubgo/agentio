package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/khanakia/agentcli"
)

// Claude check ids (CLAUDE_API §27); the shared ones come from the root.
const (
	CheckBinaryShadowed    agentcli.CheckID = "binary-shadowed"
	CheckShellFunction     agentcli.CheckID = "shell-function"
	CheckVersionMin        agentcli.CheckID = "version-min"
	CheckHomeEnvDefault    agentcli.CheckID = "home-env-default"
	CheckLoginExpired      agentcli.CheckID = "login-expired"
	CheckSettingsParse     agentcli.CheckID = "settings-parse"
	CheckGlobalConfigParse agentcli.CheckID = "global-config-parse"
	CheckGlobalConfigSize  agentcli.CheckID = "global-config-size"
	CheckMCPProjectPending agentcli.CheckID = "mcp-project-pending"
	CheckPluginsMissing    agentcli.CheckID = "plugins-missing"
	CheckStaleLiveSessions agentcli.CheckID = "stale-live-sessions"
	CheckRetention         agentcli.CheckID = "retention"
	CheckHistorySize       agentcli.CheckID = "history-size"
	CheckAuthStatus        agentcli.CheckID = "auth-status"
)

// Doctor thresholds.
const (
	// globalConfigWarnBytes is the size at which Claude Code starts backing
	// ~/.claude.json up aggressively (8 MiB per docs).
	globalConfigWarnBytes = 8 << 20
	// defaultCleanupDays is Claude's retention when cleanupPeriodDays is unset.
	defaultCleanupDays = 30
	// shellFunctionMark is how a zsh/bash snapshot defines a claude function.
	shellFunctionMark = "claude () {"
	// snapshotScanBytes bounds the shell snapshot read.
	snapshotScanBytes = 16 << 20
	mib               = 1 << 20
)

// DoctorOptions holds the opt-in doors; nil Runner = no check runs claude.
type DoctorOptions struct{ Runner *Runner }

// Doctor computes health checks for h (and p when set) from disk reads:
// the L1 checks, version-tested, shadowed binaries, shell functions,
// minimum version, home env trap, login presence and expiry, settings and
// .claude.json parse and size, pending project MCP servers, missing
// plugins, stale live sessions, retention and history size. With
// opts.Runner it also runs `auth status` (Process, may touch login).
// Messages carry names and counts only, never values. cost: Disk · Read
// (Process only through opts.Runner).
func (a *Agent) Doctor(ctx context.Context, h Home, p *Project, opts DoctorOptions) []agentcli.Check {
	out := a.core.Checks(ctx, h.Neutral())
	add := func(id agentcli.CheckID, st agentcli.CheckStatus, msg string) {
		out = append(out, agentcli.Check{ID: id, Status: st, Message: msg})
	}
	inst, ierr := a.Install()
	out = append(out, agentcli.VersionCheck(Compat(), inst.Version))
	if ierr == nil && len(inst.Shadowed) > 0 {
		add(CheckBinaryShadowed, agentcli.CheckWarn, fmt.Sprintf("%d other claude on PATH: %s", len(inst.Shadowed), strings.Join(inst.Shadowed, ", ")))
	}
	if a.shellFunctionDefined(h) {
		add(CheckShellFunction, agentcli.CheckInfo, "claude is also a shell function in a shell snapshot; the library uses the PATH binary")
	}
	eff, _ := a.Settings(h, p, nil)
	for _, l := range eff.Layers {
		switch {
		case l.Err != nil:
			add(CheckSettingsParse, agentcli.CheckFail, l.Source.Path+" does not parse")
		case len(l.Settings.Warnings) > 0:
			add(CheckSettingsParse, agentcli.CheckWarn, fmt.Sprintf("%s: %d keys changed type", l.Source.Path, len(l.Settings.Warnings)))
		}
		for ev := range l.Settings.Hooks {
			if !ev.IsKnown() {
				add(CheckSettingsParse, agentcli.CheckWarn, l.Source.Path+": unknown hook event "+string(ev))
			}
		}
	}
	if minV := eff.Merged.MinimumVersion; minV != "" && !inst.Version.IsZero() && inst.Version.Compare(agentcli.MustVersion(minV)) < 0 {
		add(CheckVersionMin, agentcli.CheckFail, "installed "+inst.Version.String()+" is below minimumVersion "+minV)
	}
	if v, ok := a.host.Getenv(ConfigDirEnv); ok && filepath.Clean(v) == a.DefaultHome().Dir() {
		add(CheckHomeEnvDefault, agentcli.CheckWarn, ConfigDirEnv+" names the default home; Claude Code then uses a hashed keychain name and looks logged out")
	}
	a.loginChecks(h, add)
	a.globalConfigChecks(h, add)
	if p != nil {
		if ss, err := a.MCPServers(h, p); err == nil {
			var pending []string
			for _, s := range ss {
				if s.Scope == MCPScopeProject && s.Approval == ApprovalPending {
					pending = append(pending, s.Name)
				}
			}
			if len(pending) > 0 {
				add(CheckMCPProjectPending, agentcli.CheckInfo, ".mcp.json servers not approved yet: "+strings.Join(pending, ", "))
			}
		}
	}
	a.pluginChecks(h, p, eff.Merged, add)
	if ls, err := a.LiveSessions(h); err == nil && a.host.Processes != nil {
		stale := 0
		for _, l := range ls {
			if !l.Alive {
				stale++
			}
		}
		if stale > 0 {
			add(CheckStaleLiveSessions, agentcli.CheckInfo, fmt.Sprintf("%d sessions/*.json entries whose process is gone", stale))
		}
	}
	a.retentionCheck(h, eff.Merged, add)
	a.historySizeCheck(h, add)
	if opts.Runner != nil {
		if st, err := opts.Runner.AuthStatus(ctx, h); err != nil {
			add(CheckAuthStatus, agentcli.CheckWarn, "auth status failed: "+err.Error())
		} else if !st.LoggedIn {
			add(CheckAuthStatus, agentcli.CheckWarn, "auth status: not logged in")
		} else {
			add(CheckAuthStatus, agentcli.CheckOK, "auth status: logged in ("+string(st.AuthMethod)+")")
		}
	}
	return out
}

func (a *Agent) loginChecks(h Home, add func(agentcli.CheckID, agentcli.CheckStatus, string)) {
	c, err := a.Credential(h)
	if err != nil {
		add(agentcli.CheckLogin, agentcli.CheckWarn, "no login found in the credentials file or keychain")
		return
	}
	add(agentcli.CheckLogin, agentcli.CheckOK, "login in "+string(c.Source))
	if c.Expired(a.now()) {
		add(CheckLoginExpired, agentcli.CheckWarn, "login expired at "+c.ExpiresAt.Format(time.RFC3339)+"; run claude in this home to refresh it")
	}
}

func (a *Agent) globalConfigChecks(h Home, add func(agentcli.CheckID, agentcli.CheckStatus, string)) {
	p := a.globalConfigPath(h)
	info, err := os.Stat(p)
	if err != nil {
		return
	}
	if info.Size() > globalConfigWarnBytes {
		add(CheckGlobalConfigSize, agentcli.CheckWarn, fmt.Sprintf("%s is %d MiB", p, info.Size()/mib))
	}
	g, err := a.GlobalConfig(h)
	switch {
	case err != nil:
		add(CheckGlobalConfigParse, agentcli.CheckFail, p+" does not parse")
	case len(g.Warnings) > 0:
		add(CheckGlobalConfigParse, agentcli.CheckWarn, fmt.Sprintf("%s: %d keys changed type", p, len(g.Warnings)))
	}
}

func (a *Agent) pluginChecks(h Home, p *Project, s Settings, add func(agentcli.CheckID, agentcli.CheckStatus, string)) {
	installed, err := a.Plugins(h, p)
	if err != nil {
		return
	}
	have := map[PluginID]bool{}
	for _, pl := range installed {
		have[pl.ID] = true
	}
	var missing []string
	for id, on := range s.EnabledPlugins {
		if on && !have[id] {
			missing = append(missing, string(id))
		}
	}
	if len(missing) > 0 {
		add(CheckPluginsMissing, agentcli.CheckWarn, "enabled but not installed: "+strings.Join(missing, ", "))
	}
}

func (a *Agent) retentionCheck(h Home, s Settings, add func(agentcli.CheckID, agentcli.CheckStatus, string)) {
	days := defaultCleanupDays
	if s.CleanupPeriodDays != nil {
		days = *s.CleanupPeriodDays
	}
	msg := fmt.Sprintf("transcripts kept %d days", days)
	if last, err := a.LastCleanup(h); err == nil && last != nil && !a.now().IsZero() {
		msg += fmt.Sprintf("; last cleanup %d h ago", int(a.now().Sub(*last).Hours()))
		if a.now().Sub(*last) > time.Duration(days)*hoursPerDay*time.Hour {
			add(CheckRetention, agentcli.CheckWarn, msg)
			return
		}
	}
	add(CheckRetention, agentcli.CheckInfo, msg)
}

func (a *Agent) historySizeCheck(h Home, add func(agentcli.CheckID, agentcli.CheckStatus, string)) {
	info, err := os.Stat(filepath.Join(h.Dir(), historyFile))
	if err != nil {
		return
	}
	entries, _ := os.ReadDir(filepath.Join(h.Dir(), projectsDir))
	add(CheckHistorySize, agentcli.CheckInfo, fmt.Sprintf("history.jsonl %d MiB; %d project history dirs", info.Size()/mib, len(entries)))
}

// shellFunctionDefined scans the newest shell snapshot for a claude
// function (the owner's machine defines one; a shell lookup finds it first).
func (a *Agent) shellFunctionDefined(h Home) bool {
	snaps, err := a.ShellSnapshots(h)
	if err != nil || len(snaps) == 0 {
		return false
	}
	newest := snaps[0]
	for _, s := range snaps[1:] {
		if s.Created.After(newest.Created) {
			newest = s
		}
	}
	data, found, err := readFileBounded(newest.Path, snapshotScanBytes)
	return err == nil && found && strings.Contains(string(data), shellFunctionMark)
}
