package opencode

import (
	"context"
	"os"
	"strconv"

	"github.com/ubgo/agentio"
)

// opencode-specific doctor check IDs (open set beside the neutral ones).
const (
	// CheckDatabase reports where the sessions are: opencode.db (read by
	// opencode/store, not by this package) and the legacy storage/ tree.
	CheckDatabase agentio.CheckID = "database"
	// CheckDataHome notes an XDG_DATA_HOME that moves the data dir.
	CheckDataHome agentio.CheckID = "data-home"
	// CheckAutoUpdate notes that opencode updates itself unless
	// OPENCODE_DISABLE_AUTOUPDATE is set, so the installed version can
	// move under a caller.
	CheckAutoUpdate agentio.CheckID = "auto-update"
)

// Checks runs the doctor for h: binary, home (shared), installed version
// against the tested range, where the sessions are stored, the XDG data
// dir, and auto-update. Never runs the binary and never opens the
// database. cost: Disk · Read.
func (a *Agent) Checks(ctx context.Context, h Home) []agentio.Check {
	out := a.core.Checks(ctx, h.Neutral())
	in, err := a.Install()
	if err != nil {
		in = agentio.Install{}
	}
	out = append(out, agentio.VersionCheck(Compat(), in.Version))
	out = append(out, a.databaseCheck(ctx, h))
	if v, ok := a.host.Getenv(XDGDataHomeEnv); ok && v != "" {
		out = append(out, agentio.Check{ID: CheckDataHome, Status: agentio.CheckInfo, Message: XDGDataHomeEnv + " is set: the data dir is " + a.DataDir()})
	}
	if _, ok := a.host.Getenv(DisableAutoUpdateEnv); !ok {
		out = append(out, agentio.Check{ID: CheckAutoUpdate, Status: agentio.CheckInfo, Message: DisableAutoUpdateEnv + " is unset: opencode updates itself"})
	}
	return out
}

// databaseCheck says which store holds h's sessions. cost: Disk · Read.
func (a *Agent) databaseCheck(ctx context.Context, h Home) agentio.Check {
	db, onDisk := a.DatabasePath(h)
	dbExists := false
	if onDisk {
		if _, err := os.Stat(db); err == nil {
			dbExists = true
		}
	}
	legacy, err := a.Sessions(ctx, h, agentio.ConversationQuery{})
	if err != nil {
		return agentio.Check{ID: CheckDatabase, Status: agentio.CheckFail, Message: err.Error()}
	}
	switch {
	case dbExists:
		return agentio.Check{ID: CheckDatabase, Status: agentio.CheckInfo, Message: db + " holds the sessions since opencode 1.2.0 (read them with opencode/store); this package lists " + strconv.Itoa(len(legacy)) + " legacy sessions from storage/"}
	case !onDisk:
		return agentio.Check{ID: CheckDatabase, Status: agentio.CheckWarn, Message: DBEnv + " keeps the database in memory: sessions are not saved"}
	case len(legacy) > 0:
		return agentio.Check{ID: CheckDatabase, Status: agentio.CheckOK, Message: strconv.Itoa(len(legacy)) + " legacy sessions in storage/, no database yet"}
	}
	return agentio.Check{ID: CheckDatabase, Status: agentio.CheckInfo, Message: "no sessions yet"}
}
