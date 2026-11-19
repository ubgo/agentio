package agentio

import "bytes"

// TranscriptRef is what a transcript path says about itself: which agent,
// home and conversation it belongs to (TurnFileReader.TranscriptOwner).
type TranscriptRef struct {
	Agent          ID
	Home           Home
	ConversationID string
}

// TurnFileReader reads turns from a transcript path the caller already
// holds (a hook payload's transcript_path, a file watcher, a listing it
// stored), without finding the file again per call (an optional
// capability beside TurnReader; TurnFilesOf).
type TurnFileReader interface {
	// TurnsAt is Disk · Read: the same contract as TurnReader.Turns, over
	// the transcript at path. from is a cursor a previous call returned
	// (0 = the start); the returned cursor never passes an incomplete
	// trailing record or a turn whose usage has not been written yet.
	TurnsAt(path string, from Offset, fn func(Turn) error) (Offset, error)
	// TranscriptOwner maps a transcript path to its home and conversation
	// id from the path alone (plus the file head when the name does not
	// hold the id). ErrNotFound when path is not one of this agent's
	// transcripts. Disk · Read.
	TranscriptOwner(path string) (TranscriptRef, error)
}

// BadLine is a transcript line a turn reader skipped because it does not
// decode (a torn write, a disk error, a line another tool appended). The
// line carries no turn; the read goes on with the next line.
type BadLine struct {
	Path string
	// Offset is where the line starts.
	Offset Offset
	Err    error
}

// BadLineReporter is an optional extension of TurnFileReader: TurnsAt
// that also reports every line it skipped as undecodable (Claude Code,
// Codex). Plain TurnsAt skips the same lines silently; neither form stops
// at one.
type BadLineReporter interface {
	// TurnsAtReporting is TurnsAt plus bad, called once per skipped line
	// (nil = skip silently). Disk · Read.
	TurnsAtReporting(path string, from Offset, fn func(Turn) error, bad func(BadLine)) (Offset, error)
}

// TurnsAtReporting reads like r.TurnsAt and reports skipped lines to bad
// when r implements BadLineReporter; otherwise it is r.TurnsAt and bad is
// never called. Disk · Read.
func TurnsAtReporting(r TurnFileReader, path string, from Offset, fn func(Turn) error, bad func(BadLine)) (Offset, error) {
	if br, ok := r.(BadLineReporter); ok {
		return br.TurnsAtReporting(path, from, fn, bad)
	}
	return r.TurnsAt(path, from, fn)
}

// TrimLineEnd returns line without its terminator: one trailing "\n" and a
// "\r" before it ("\r\n"), or a lone trailing "\r". Turn.Raw holds lines
// trimmed this way for every agent. Pure.
func TrimLineEnd(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte{'\n'})
	return bytes.TrimSuffix(line, []byte{'\r'})
}
