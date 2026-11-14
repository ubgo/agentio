package state

import (
	"net/url"
	"strings"
	"testing"
)

// TestFileURIPath covers the path part of the SQLite URI on every OS,
// including the Windows drive form that failed on the Windows runner
// ("invalid uri authority: C:%5CUsers...").
func TestFileURIPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/home/u/.codex/state_5.sqlite", "/home/u/.codex/state_5.sqlite"},
		{"C:/Users/RUNNER~1/AppData/Local/Temp/x/state_5.sqlite", "/C:/Users/RUNNER~1/AppData/Local/Temp/x/state_5.sqlite"},
		{"d:/codex/state_5.sqlite", "/d:/codex/state_5.sqlite"},
		{"//server/share/state_5.sqlite", "//server/share/state_5.sqlite"},
		{"state_5.sqlite", "state_5.sqlite"},
		{"", ""},
	}
	for _, c := range cases {
		if got := fileURIPath(c.in); got != c.want {
			t.Errorf("fileURIPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDSNWindowsPath checks the whole URI for a drive path: an empty
// authority, forward slashes, and the read-only query. The input is
// already slashed (what filepath.ToSlash makes of C:\Users\... on
// Windows), so the test means the same on every OS.
func TestDSNWindowsPath(t *testing.T) {
	got := dsn("C:/Users/RUNNER~1/AppData/Local/Temp/x/state_5.sqlite")
	const prefix = "file:///C:/Users/RUNNER~1/AppData/Local/Temp/x/state_5.sqlite?"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("dsn = %q, want prefix %q", got, prefix)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse %q: %v", got, err)
	}
	if u.Host != "" {
		t.Errorf("authority = %q, want empty", u.Host)
	}
	q := u.Query()
	if q.Get("mode") != "ro" || len(q["_pragma"]) != 2 {
		t.Errorf("query = %v, want mode=ro and two pragmas", q)
	}
}

// TestDSNPosixPath keeps the POSIX form unchanged: file:///abs/path.
func TestDSNPosixPath(t *testing.T) {
	got := dsn("/home/u/.codex/state_5.sqlite")
	if !strings.HasPrefix(got, "file:///home/u/.codex/state_5.sqlite?") {
		t.Fatalf("dsn = %q", got)
	}
}
