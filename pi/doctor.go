package pi

import (
	"context"
	"fmt"
	"strings"

	"github.com/ubgo/agentio"
)

// pi-specific doctor check IDs (open set beside the neutral ones).
const (
	// CheckSettingsParse reports settings.json: absent (fine), parsed, or
	// keys that changed type.
	CheckSettingsParse agentio.CheckID = "settings"
	// CheckCacheWarming notes cacheWarming: "idle" (pi's default) keeps
	// the prompt cache warm between runs and spends tokens doing so.
	CheckCacheWarming agentio.CheckID = "cache-warming"
)

// Checks runs the doctor for h: binary, home, home env (shared), the
// installed version against the tested range (from package metadata, the
// binary is never run), settings.json parse, login presence (auth.json
// entries or provider key variables, names only) and the cacheWarming
// spend note. Disk · Read.
func (a *Agent) Checks(ctx context.Context, h Home) []agentio.Check {
	out := a.core.Checks(ctx, h.Neutral())
	in, err := a.Install()
	if err != nil {
		in = agentio.Install{}
	}
	out = append(out, agentio.VersionCheck(Compat(), in.Version))
	s, found, warns, err := a.ReadSettings(h)
	switch {
	case err != nil:
		out = append(out, agentio.Check{ID: CheckSettingsParse, Status: agentio.CheckFail, Message: err.Error()})
	case !found:
		out = append(out, agentio.Check{ID: CheckSettingsParse, Status: agentio.CheckOK, Message: "no settings.json (pi defaults apply)"})
	case len(warns) > 0:
		out = append(out, agentio.Check{ID: CheckSettingsParse, Status: agentio.CheckWarn, Message: fmt.Sprintf("%d settings.json key(s) changed type; kept raw", len(warns))})
	default:
		out = append(out, agentio.Check{ID: CheckSettingsParse, Status: agentio.CheckOK, Message: a.Paths(h).Settings})
	}
	out = append(out, a.loginCheck(h))
	if s.CacheWarming.Or(CacheWarmingIdle) == CacheWarmingIdle {
		out = append(out, agentio.Check{ID: CheckCacheWarming, Status: agentio.CheckInfo, Message: "cacheWarming is idle (the default): pi keeps the prompt cache warm for up to 30 minutes after a run and spends tokens doing so"})
	}
	return out
}

// loginCheck reports which credentials pi can use, by provider and
// variable name only. Disk · Read.
func (a *Agent) loginCheck(h Home) agentio.Check {
	logins, err := a.Logins(h)
	if err != nil {
		return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckFail, Message: err.Error()}
	}
	var parts []string
	for _, l := range logins {
		parts = append(parts, l.Provider+" ("+string(l.Type)+")")
	}
	if env := a.EnvLogins(); len(env) > 0 {
		parts = append(parts, "env "+strings.Join(env, ", "))
	}
	if len(parts) == 0 {
		return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckWarn, Message: "no login: auth.json is empty and no provider key variable is set (run pi and /login, or export a provider key)"}
	}
	return agentio.Check{ID: agentio.CheckLogin, Status: agentio.CheckOK, Message: strings.Join(parts, "; ")}
}

// Runner is the door to calls that run the pi binary (Host.Exec).
type Runner struct{ r agentio.Runner }

// Runner returns the Process door. Pure.
func (a *Agent) Runner() Runner { return Runner{r: a.core.Runner()} }

// BinaryVersion runs `pi --version` on the default home. Process · Read.
// Never `pi auth check` (it refreshes OAuth tokens unless --no-refresh).
func (r Runner) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	return r.r.Version(ctx)
}
