package agentio

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
