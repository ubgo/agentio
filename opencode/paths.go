package opencode

import (
	"path/filepath"
	"slices"
	"strings"
)

// DatabasePath is the SQLite database a release build of opencode uses for
// h, the file github.com/ubgo/agentio/opencode/store opens:
//   - OPENCODE_DB from the Host env when set: an absolute path as is, a
//     relative one under the data dir; ":memory:" keeps the database in
//     memory, so ok is false (nothing on disk to read);
//   - else <data dir>/opencode.db.
//
// A build from another channel (a dev or local build) writes
// opencode-<channel>.db instead unless OPENCODE_DISABLE_CHANNEL_DB is set;
// ChannelDatabasePath names that file. OPENCODE_DB is read from the Host
// env, which is the env of the program asking, not of a running opencode.
// cost: Pure.
func (a *Agent) DatabasePath(h Home) (path string, ok bool) {
	if v, set := a.host.Getenv(DBEnv); set && v != "" {
		switch {
		case v == memoryDB:
			return "", false
		case filepath.IsAbs(v):
			return v, true
		}
		return h.Path(v), true
	}
	return h.Path(dbFile), true
}

// ChannelDatabasePath is the database a build of channel uses in h when
// OPENCODE_DB is unset: opencode.db for the release channels (latest,
// beta, prod), else opencode-<channel>.db with every byte outside
// [A-Za-z0-9._-] replaced by '-'. cost: Pure.
func ChannelDatabasePath(h Home, channel string) string {
	if slices.Contains([]string{channelLatest, channelBeta, channelProd}, channel) {
		return h.Path(dbFile)
	}
	var b strings.Builder
	for _, c := range []byte(channel) {
		if isChannelByte(c) {
			b.WriteByte(c)
		} else {
			b.WriteByte('-')
		}
	}
	return h.Path(channelDBStem + b.String() + channelDBExt)
}

// isChannelByte reports a byte opencode keeps in a channel database name.
// cost: Pure.
func isChannelByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
}
