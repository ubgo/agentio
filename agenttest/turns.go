package agenttest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ubgo/agentio"
)

// goldenPerm is the mode of a rewritten golden file.
const goldenPerm = 0o600

// GoldenTurn is the comparable form of an agentio.Turn in a golden file:
// every field but Raw (the vendor line, which the fixture already holds).
type GoldenTurn struct {
	ID             string               `json:"id,omitempty"`
	Interim        bool                 `json:"interim,omitempty"`
	Role           agentio.Role         `json:"role"`
	Time           time.Time            `json:"time"`
	ConversationID string               `json:"conversation_id,omitempty"`
	Dir            string               `json:"dir,omitempty"`
	Model          string               `json:"model,omitempty"`
	StopReason     string               `json:"stop_reason,omitempty"`
	AgentVersion   string               `json:"agent_version,omitempty"`
	Text           string               `json:"text"`
	Tokens         *agentio.TokenCounts `json:"tokens,omitempty"`
}

// Golden projects turns onto their golden form. Pure.
func Golden(turns []agentio.Turn) []GoldenTurn {
	out := make([]GoldenTurn, 0, len(turns))
	for _, t := range turns {
		out = append(out, GoldenTurn{
			ID: t.ID, Interim: t.Interim, Role: t.Role, Time: t.Time, ConversationID: t.ConversationID,
			Dir: t.Dir, Model: t.Model, StopReason: t.StopReason, AgentVersion: t.AgentVersion, Text: t.Text, Tokens: t.Tokens,
		})
	}
	return out
}

// CheckTurns compares turns with the JSON golden file at golden; with
// update set it rewrites the file instead (the `go test -update` pattern).
// Disk · Read (Disk · Write with update).
func CheckTurns(t testing.TB, golden string, turns []agentio.Turn, update bool) {
	t.Helper()
	got, err := json.MarshalIndent(Golden(turns), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if update {
		if err := os.WriteFile(golden, got, goldenPerm); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run the test with -update to write it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("turns differ from %s (run with -update after checking the change)\ngot:\n%s", filepath.Base(golden), got)
	}
}

// ReadTurns reads every turn of path from cursor 0 and returns them with
// the final cursor. Disk · Read.
func ReadTurns(t testing.TB, r agentio.TurnFileReader, path string) ([]agentio.Turn, agentio.Offset) {
	t.Helper()
	var out []agentio.Turn
	off, err := r.TurnsAt(path, 0, func(tn agentio.Turn) error {
		out = append(out, tn)
		return nil
	})
	if err != nil {
		t.Fatalf("TurnsAt %s: %v", path, err)
	}
	return out, off
}

// GrowingTurns replays a live writer: it copies src into a temp file a
// piece at a time (each line in two halves, so a read also lands inside a
// half-written line), calls TurnsAt after every piece with the cursor the
// previous call returned, and returns every turn emitted in order. For a
// reader that keeps the cursor contract the result equals ReadTurns of
// the finished file: no turn twice, none lost, tokens kept. name is the
// temp file's base name (some readers key on it). Disk · Read + Disk ·
// Write (temp dir only).
func GrowingTurns(t testing.TB, r agentio.TurnFileReader, src, name string) []agentio.Turn {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), name)
	var (
		out  []agentio.Turn
		from agentio.Offset
		cuts []int
	)
	start := 0
	for i, b := range data {
		if b == '\n' {
			cuts = append(cuts, start+(i+1-start)/2, i+1)
			start = i + 1
		}
	}
	if start < len(data) {
		cuts = append(cuts, len(data))
	}
	for _, cut := range cuts {
		if err := os.WriteFile(dst, data[:cut], goldenPerm); err != nil {
			t.Fatal(err)
		}
		from, err = r.TurnsAt(dst, from, func(tn agentio.Turn) error {
			out = append(out, tn)
			return nil
		})
		if err != nil {
			t.Fatalf("TurnsAt at %d bytes: %v", cut, err)
		}
	}
	return out
}
