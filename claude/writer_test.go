package claude

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestMoveHistoryRefusesLiveSession: Writer.MoveHistory refuses while Claude
// runs in the source folder, with the same blocker PlanMove reports, as a
// typed *LiveSessionError matching agentio.ErrMoveBlocked; nothing moves.
func TestMoveHistoryRefusesLiveSession(t *testing.T) {
	t.Parallel()
	procs := []agentio.Process{{PID: 4242, Start: fixtureStartedAt().Add(-3 * time.Second), Argv: []string{"claude"}}}
	a, h := sessionsAgent(t, filepath.Join("testdata", "homes", fixtureVersions(t)[0], "userhome"), procs)
	src := a.HistoryDir(h, "/work/proj4")
	agenttest.WriteFile(t, src, "6d50c0ec-647e-18aa-84c7-04170288be89.jsonl", "{}\n")
	res, err := a.Writer().MoveHistory(h, "/work/proj4", "/work/proj4-new")
	var le *LiveSessionError
	if res.Moved || !errors.As(err, &le) || !errors.Is(err, agentio.ErrMoveBlocked) || len(le.PIDs) != 1 || le.PIDs[0] != 4242 {
		t.Fatalf("MoveHistory with a live session: %+v err=%v", res, err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source history moved anyway: %v", err)
	}
	plan, err := a.PlanMove(agentio.MoveRequest{From: h.Neutral(), To: h.Neutral(), Dir: "/work/proj4"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers) == 0 || plan.Blockers[len(plan.Blockers)-1] != le.Blockers()[0] {
		t.Errorf("PlanMove blockers %q, MoveHistory blockers %q: want the same text", plan.Blockers, le.Blockers())
	}
}

// TestApplyHomeLinksChecksOwner: ApplyHomeLinks refuses links planned for
// another agent's homes with ErrForeignHome, as ApplyMove does, and links
// nothing.
func TestApplyHomeLinksChecksOwner(t *testing.T) {
	t.Parallel()
	a, dir := fixtureAgent(t, fixtureVersions(t)[0])
	work := a.HomeAt(filepath.Join(dir, ".claude-work-new"))
	links, err := a.Neutral().(agentio.HomeLinker).PlanHomeLinks(a.DefaultHome().Neutral(), work.Neutral())
	if err != nil || len(links) == 0 {
		t.Fatalf("plan %v %v", links, err)
	}
	foreign := append([]agentio.HomeLink(nil), links...)
	foreign[0].Agent = "codex"
	if err := a.Writer().ApplyHomeLinks(foreign); !errors.Is(err, agentio.ErrForeignHome) {
		t.Fatalf("Writer.ApplyHomeLinks foreign: %v", err)
	}
	if err := a.Neutral().(agentio.HomeLinker).ApplyHomeLinks(foreign); !errors.Is(err, agentio.ErrForeignHome) {
		t.Fatalf("neutral ApplyHomeLinks foreign: %v", err)
	}
	if _, err := os.Lstat(links[0].To); !os.IsNotExist(err) {
		t.Fatalf("a foreign plan created %s", links[0].To)
	}
	if err := a.Writer().ApplyHomeLinks(links); err != nil {
		t.Fatalf("own plan: %v", err)
	}
}

// TestNeutralMoveHistoryReportsMoved: the neutral HistoryMover keeps the
// "nothing moved" case: a folder without history is Moved false with an
// empty Backup and nil error, a real move is Moved true with its backup.
func TestNeutralMoveHistoryReportsMoved(t *testing.T) {
	t.Parallel()
	a, _ := fixtureAgent(t, fixtureVersions(t)[0])
	hm, ok := agentio.HistoryMoverOf(a.Neutral())
	if !ok {
		t.Fatal("no HistoryMover")
	}
	def := a.DefaultHome().Neutral()
	none, err := hm.MoveHistory(def, "/work/no-history", "/work/elsewhere")
	if err != nil || none.Moved || len(none.Backup.Entries) != 0 {
		t.Fatalf("no history: %+v %v", none, err)
	}
	got, err := hm.MoveHistory(def, "/work/proj12", "/work/proj12-renamed")
	if err != nil || !got.Moved || len(got.Backup.Entries) == 0 {
		t.Fatalf("real move: %+v %v", got, err)
	}
}
