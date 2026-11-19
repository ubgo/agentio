package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// SessionStatus is sessions/<pid>.json `status` (open set).
type SessionStatus string

// Statuses seen: idle and busy (docs + disk), shell (this server: a shell
// command the user ran with `!` is executing), waiting (recorded at 2.1.327
// while a dialog was open; the file then also carries waitingFor).
const (
	StatusIdle    SessionStatus = "idle"
	StatusBusy    SessionStatus = "busy"
	StatusShell   SessionStatus = "shell"
	StatusWaiting SessionStatus = "waiting"
)

// IsKnown reports a recorded status. cost: Pure.
func (s SessionStatus) IsKnown() bool {
	return s == StatusIdle || s == StatusBusy || s == StatusShell || s == StatusWaiting
}

// Neutral maps onto the shared status. shell counts as Busy: the session is
// running a command and is not waiting for input. waiting is
// SessionWaiting: the session is blocked on the user (a dialog or
// permission prompt), not idle at its prompt and not working. cost: Pure.
func (s SessionStatus) Neutral() agentio.SessionStatus {
	switch s {
	case StatusIdle:
		return agentio.SessionIdle
	case StatusWaiting:
		return agentio.SessionWaiting
	case StatusBusy, StatusShell:
		return agentio.SessionBusy
	}
	return agentio.SessionStatusUnknown
}

// ProcStart is the start stamp Claude Code records next to a pid to detect
// pid reuse: a date string on darwin, /proc/<pid>/stat starttime ticks on
// linux.
type ProcStart string

// LiveSession is one sessions/<pid>.json registry entry.
type LiveSession struct {
	PID        int
	ProcStart  ProcStart
	SessionID  string
	Cwd        string
	Name       string
	NameSource string
	Version    string
	Kind       agentio.SessionKind
	Entrypoint string
	Status     SessionStatus
	// WaitingFor is the vendor's free-text reason while Status is waiting
	// (seen: "dialog open"); "" when absent. Kept as text, not an enum: one
	// value was recorded and the vendor does not document the set.
	WaitingFor      string
	StartedAt       time.Time
	UpdatedAt       time.Time
	StatusUpdatedAt time.Time
	JobID           string
	BridgeSessionID string
	MessagingSocket string
	PeerProtocol    int
	PeerFeatures    []string
	Tmux            string
	PIDDomain       string
	// Alive: the pid is in the process table and its start time is within
	// liveTolerance of StartedAt (pid-reuse guard).
	Alive   bool
	Path    string
	Unknown agentio.Unknown
}

type wireLiveSession struct {
	PID             json.Number         `json:"pid"`
	SessionID       string              `json:"sessionId"`
	Cwd             string              `json:"cwd"`
	StartedAt       millis              `json:"startedAt"`
	ProcStart       ProcStart           `json:"procStart"`
	Version         string              `json:"version"`
	PeerProtocol    int                 `json:"peerProtocol"`
	PeerFeatures    []string            `json:"peerFeatures"`
	Kind            agentio.SessionKind `json:"kind"`
	Entrypoint      string              `json:"entrypoint"`
	PIDDomain       string              `json:"pidDomain"`
	Tmux            string              `json:"tmux"`
	MessagingSocket string              `json:"messagingSocketPath"`
	Name            string              `json:"name"`
	NameSource      string              `json:"nameSource"`
	Status          SessionStatus       `json:"status"`
	WaitingFor      string              `json:"waitingFor"`
	UpdatedAt       millis              `json:"updatedAt"`
	StatusUpdatedAt millis              `json:"statusUpdatedAt"`
	BridgeSessionID string              `json:"bridgeSessionId"`
	JobID           string              `json:"jobId"`
}

// parseLiveSession decodes one registry file body.
func parseLiveSession(path string, data []byte) (LiveSession, error) {
	var w wireLiveSession
	unknown, _, err := decodeLenient(sessionsDir, data, reflect.TypeFor[wireLiveSession](), func(b []byte) error { w = wireLiveSession{}; return json.Unmarshal(b, &w) })
	if err != nil {
		return LiveSession{}, err
	}
	pid, err := strconv.Atoi(w.PID.String())
	if err != nil {
		return LiveSession{}, fmt.Errorf("claude: %s: pid %q", path, w.PID)
	}
	return LiveSession{
		PID: pid, ProcStart: w.ProcStart, SessionID: w.SessionID, Cwd: w.Cwd, Name: w.Name, NameSource: w.NameSource,
		Version: w.Version, Kind: w.Kind, Entrypoint: w.Entrypoint, Status: w.Status, WaitingFor: w.WaitingFor, StartedAt: w.StartedAt.Time(),
		UpdatedAt: w.UpdatedAt.Time(), StatusUpdatedAt: w.StatusUpdatedAt.Time(), JobID: w.JobID, BridgeSessionID: w.BridgeSessionID,
		MessagingSocket: w.MessagingSocket, PeerProtocol: w.PeerProtocol, PeerFeatures: w.PeerFeatures, Tmux: w.Tmux,
		PIDDomain: w.PIDDomain, Path: path, Unknown: unknown,
	}, nil
}

// processTable reads Host.Processes once; nil when the host has none or
// the read fails (Alive then stays false).
func (a *Agent) processTable(ctx context.Context) map[int]agentio.Process {
	if a.host.Processes == nil {
		return nil
	}
	list, err := a.host.Processes.Processes(ctx)
	if err != nil {
		return nil
	}
	m := make(map[int]agentio.Process, len(list))
	for _, p := range list {
		m[p.PID] = p
	}
	return m
}

// alive applies the pid-reuse guard. A row with no start time is trusted
// on pid presence alone.
func alive(table map[int]agentio.Process, pid int, startedAt time.Time) bool {
	p, ok := table[pid]
	if !ok {
		return false
	}
	if p.Start.IsZero() || startedAt.IsZero() {
		return true
	}
	d := p.Start.Sub(startedAt)
	return d <= liveTolerance && d >= -liveTolerance
}

// LiveSessions reads every sessions/<pid>.json of h (never the *.key
// files) and marks each Alive against the process table. Unreadable
// entries are skipped: the registry is advisory. cost: Disk · Read + one
// process-table read (Host.Processes).
func (a *Agent) LiveSessions(h Home) ([]LiveSession, error) {
	dir := filepath.Join(h.Dir(), sessionsDir)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	table := a.processTable(context.Background())
	var out []LiveSession
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || filepath.Ext(n) != jsonExt || strings.Count(n, ".") != 1 {
			continue
		}
		p := filepath.Join(dir, n)
		data, found, err := readFileBounded(p, maxResponseBytes)
		if err != nil || !found {
			continue
		}
		s, err := parseLiveSession(p, data)
		if err != nil {
			continue
		}
		s.Alive = alive(table, s.PID, s.StartedAt)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out, nil
}

// LiveSessionsIn returns the live sessions whose cwd is dir. cost: Disk ·
// Read + process table.
func (a *Agent) LiveSessionsIn(h Home, dir string) ([]LiveSession, error) {
	all, err := a.LiveSessions(h)
	if err != nil {
		return nil, err
	}
	want := filepath.Clean(dir)
	var out []LiveSession
	for _, s := range all {
		if filepath.Clean(s.Cwd) == want {
			out = append(out, s)
		}
	}
	return out, nil
}

// IsLive reports an alive session running conversationID. cost: Disk ·
// Read + process table.
func (a *Agent) IsLive(h Home, conversationID string) (bool, error) {
	all, err := a.LiveSessions(h)
	if err != nil {
		return false, err
	}
	for _, s := range all {
		if s.SessionID == conversationID && s.Alive {
			return true, nil
		}
	}
	return false, nil
}

// Neutral projects onto the shared live session. cost: Pure.
func (l LiveSession) Neutral(h Home) agentio.LiveSession {
	return agentio.LiveSession{
		PID: l.PID, ConversationID: l.SessionID, Dir: l.Cwd, Home: h.Neutral(), Kind: l.Kind,
		Status: l.Status.Neutral(), StatusSince: l.StatusUpdatedAt, StartedAt: l.StartedAt, Alive: l.Alive,
	}
}

// LiveProcess is one process-table row recognised as Claude Code.
type LiveProcess struct {
	PID, PPID int
	Kind      agentio.ProcessKind
	Session   bool
	Exe       string
	Argv      []string
	Start     time.Time
	// Cwd is best effort; "" when the host cannot read it.
	Cwd string
}

// argMCPServe is the `claude mcp serve` subcommand: Claude Code acting as
// an MCP server for another client (documented), not a conversation.
const argMCPServe = "serve"

// ClassifyProcess says whether p is Claude Code, from Spec().Process
// (agentop's production rules). No `claude mcp …` row is a session:
// `claude mcp serve` is an MCP server, every other `mcp` subcommand
// (list, add, remove, get, …) a short config command. Every other match
// is a CLI conversation. cost: Pure.
func ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	if !Spec().Process.Match(p) {
		return agentio.ProcessInfo{}, false
	}
	if i := slices.Index(p.Argv, subMCP); i >= 0 {
		if i+1 < len(p.Argv) && p.Argv[i+1] == argMCPServe {
			return agentio.ProcessInfo{Agent: ID, Kind: agentio.ProcessMCPServer}, true
		}
		return agentio.ProcessInfo{Agent: ID, Kind: agentio.ProcessCommand}, true
	}
	return agentio.ProcessInfo{Agent: ID, Kind: agentio.ProcessCLI, Session: true}, true
}

// ClassifyProcess is the package function on the agent. cost: Pure.
func (a *Agent) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	return ClassifyProcess(p)
}

// Processes lists Claude Code processes from Host.Processes, with a
// best-effort cwd each; never runs claude. cost: Process · Read (process
// table + cwd lookups), MayTouchLogin false.
func (a *Agent) Processes(ctx context.Context) ([]LiveProcess, error) {
	if a.host.Processes == nil {
		return nil, agentio.UnsupportedError(ID, "processes", "the host has no process lister")
	}
	list, err := a.host.Processes.Processes(ctx)
	if err != nil {
		return nil, fmt.Errorf("claude: processes: %w", err)
	}
	var out []LiveProcess
	for _, p := range list {
		info, ok := ClassifyProcess(p)
		if !ok {
			continue
		}
		lp := LiveProcess{PID: p.PID, PPID: p.PPID, Kind: info.Kind, Session: info.Session, Exe: p.Exe, Argv: p.Argv, Start: p.Start}
		if cwd, err := a.host.Processes.Cwd(ctx, p.PID); err == nil {
			lp.Cwd = cwd
		}
		out = append(out, lp)
	}
	return out, nil
}

// JobChild is one child link of a background job.
type JobChild struct {
	ID   string `json:"id"`
	Href string `json:"href"`
	Kind string `json:"kind"`
}

// Job is jobs/<id>/state.json.
type Job struct {
	ID                                                                        string
	State, Tempo, Detail, Needs, Intent, Template, Name, Backend, BgIsolation string
	Tokens                                                                    int64
	SessionID, ResumeSessionID, Cwd, CLIVersion                               string
	RespawnFlags, ProviderEnv                                                 []string
	Children                                                                  []JobChild
	CreatedAt, UpdatedAt                                                      time.Time
	Unknown                                                                   agentio.Unknown
}

type wireJob struct {
	State           string     `json:"state"`
	Tempo           string     `json:"tempo"`
	Detail          string     `json:"detail"`
	Needs           string     `json:"needs"`
	Tokens          int64      `json:"tokens"`
	Template        string     `json:"template"`
	RespawnFlags    []string   `json:"respawnFlags"`
	Intent          string     `json:"intent"`
	Name            string     `json:"name"`
	SessionID       string     `json:"sessionId"`
	ResumeSessionID string     `json:"resumeSessionId"`
	CLIVersion      string     `json:"cliVersion"`
	Cwd             string     `json:"cwd"`
	BgIsolation     string     `json:"bgIsolation"`
	ProviderEnv     []string   `json:"providerEnv"`
	Backend         string     `json:"backend"`
	Children        []JobChild `json:"children"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// Jobs reads every jobs/<id>/state.json of h. cost: Disk · Read.
func (a *Agent) Jobs(h Home) ([]Job, error) {
	dir := filepath.Join(h.Dir(), jobsDir)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	var out []Job
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, found, err := readFileBounded(filepath.Join(dir, e.Name(), jobStateFile), maxResponseBytes)
		if err != nil || !found {
			continue
		}
		var w wireJob
		unknown, _, err := decodeLenient(jobStateFile, data, reflect.TypeFor[wireJob](), func(b []byte) error { w = wireJob{}; return json.Unmarshal(b, &w) })
		if err != nil {
			continue
		}
		out = append(out, Job{ID: e.Name(), State: w.State, Tempo: w.Tempo, Detail: w.Detail, Needs: w.Needs, Intent: w.Intent,
			Template: w.Template, Name: w.Name, Backend: w.Backend, BgIsolation: w.BgIsolation, Tokens: w.Tokens,
			SessionID: w.SessionID, ResumeSessionID: w.ResumeSessionID, Cwd: w.Cwd, CLIVersion: w.CLIVersion,
			RespawnFlags: w.RespawnFlags, ProviderEnv: w.ProviderEnv, Children: w.Children,
			CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt, Unknown: unknown})
	}
	return out, nil
}

// JobEvent is one jobs/<id>/timeline.jsonl line.
type JobEvent struct {
	At     time.Time `json:"at"`
	State  string    `json:"state"`
	Detail string    `json:"detail"`
	Text   string    `json:"text"`
}

// JobTimeline reads the last 256 KiB of jobs/<id>/timeline.jsonl.
// cost: Disk · Read (bounded tail).
func (a *Agent) JobTimeline(h Home, id string) ([]JobEvent, error) {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return nil, fmt.Errorf("%w: job %q", agentio.ErrNotFound, id)
	}
	buf, err := readTail(filepath.Join(h.Dir(), jobsDir, id, jobTimelineFile), tailScanBytes)
	if isNotExist(err) {
		return nil, fmt.Errorf("%w: job %s", agentio.ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	var out []JobEvent
	for _, line := range splitLines(buf) {
		var ev JobEvent
		if json.Unmarshal(line, &ev) == nil {
			out = append(out, ev)
		}
	}
	return out, nil
}

// DaemonLock is daemon.lock.
type DaemonLock struct {
	PID            int             `json:"pid"`
	Version        string          `json:"version"`
	JSONPath       string          `json:"jsonPath"`
	LogPath        string          `json:"logPath"`
	StartedAt      millis          `json:"startedAt"`
	Origin         string          `json:"origin"`
	SpawnedBy      json.RawMessage `json:"spawnedBy"`
	ProcStart      ProcStart       `json:"procStart"`
	ProcessWrapper string          `json:"processWrapper"`
}

// DaemonStatus is daemon.status.json; workers stay raw (no sample).
type DaemonStatus struct {
	SupervisorPID       int                        `json:"supervisorPid"`
	SupervisorProcStart ProcStart                  `json:"supervisorProcStart"`
	WrittenAt           millis                     `json:"writtenAt"`
	Workers             map[string]json.RawMessage `json:"workers"`
}

// DaemonRoster is daemon/roster.json; workers stay raw (no sample).
type DaemonRoster struct {
	Proto         int                        `json:"proto"`
	SupervisorPID int                        `json:"supervisorPid"`
	UpdatedAt     millis                     `json:"updatedAt"`
	Workers       map[string]json.RawMessage `json:"workers"`
}

// DaemonAuth is daemon-auth-status.json.
type DaemonAuth struct {
	Since  millis `json:"since"`
	Status string `json:"status"`
}

// Daemon is the background-session supervisor's files.
type Daemon struct {
	Lock    *DaemonLock
	Status  *DaemonStatus
	Roster  *DaemonRoster
	Auth    *DaemonAuth
	Running bool
}

// readJSONInto reads a bounded JSON file through decode; false when absent
// or undecodable.
func readJSONInto(path string, decode func([]byte) error) bool {
	data, found, err := readFileBounded(path, maxResponseBytes)
	return err == nil && found && decode(data) == nil
}

// Daemon reads the daemon files of h; Running = the lock's pid is alive
// (start time checked against the lock's startedAt). cost: Disk · Read +
// process table.
func (a *Agent) Daemon(h Home) (Daemon, error) {
	var d Daemon
	var lock DaemonLock
	if readJSONInto(filepath.Join(h.Dir(), daemonLockFile), func(b []byte) error { return json.Unmarshal(b, &lock) }) {
		d.Lock = &lock
		d.Running = alive(a.processTable(context.Background()), lock.PID, lock.StartedAt.Time())
	}
	var st DaemonStatus
	if readJSONInto(filepath.Join(h.Dir(), daemonStatusFile), func(b []byte) error { return json.Unmarshal(b, &st) }) {
		d.Status = &st
	}
	var ro DaemonRoster
	if readJSONInto(filepath.Join(h.Dir(), daemonDir, daemonRosterFile), func(b []byte) error { return json.Unmarshal(b, &ro) }) {
		d.Roster = &ro
	}
	var au DaemonAuth
	if readJSONInto(filepath.Join(h.Dir(), daemonAuthStatusFile), func(b []byte) error { return json.Unmarshal(b, &au) }) {
		d.Auth = &au
	}
	return d, nil
}

// TeamMember is one teams/<team>/config.json member.
type TeamMember struct {
	AgentID       string          `json:"agentId"`
	AgentType     string          `json:"agentType"`
	BackendType   string          `json:"backendType"`
	Cwd           string          `json:"cwd"`
	JoinedAt      millis          `json:"joinedAt"`
	Name          string          `json:"name"`
	Subscriptions json.RawMessage `json:"subscriptions"`
	TmuxPaneID    string          `json:"tmuxPaneId"`
}

// TeamLead is the lead process of a team.
type TeamLead struct {
	PID       int    `json:"pid"`
	PIDSpace  string `json:"pidSpace"`
	ClaimedAt millis `json:"claimedAt"`
}

// Team is teams/<team>/config.json.
type Team struct {
	Name          string
	CreatedAt     time.Time
	LeadAgentID   string
	LeadSessionID string
	LeadProcess   *TeamLead
	Members       []TeamMember
	Unknown       agentio.Unknown
}

type wireTeam struct {
	Name          string       `json:"name"`
	CreatedAt     millis       `json:"createdAt"`
	LeadAgentID   string       `json:"leadAgentId"`
	LeadSessionID string       `json:"leadSessionId"`
	LeadProcess   *TeamLead    `json:"leadProcess"`
	Members       []TeamMember `json:"members"`
}

// Teams reads every teams/<team>/config.json of h. cost: Disk · Read.
func (a *Agent) Teams(h Home) ([]Team, error) {
	dir := filepath.Join(h.Dir(), teamsDir)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	var out []Team
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, found, err := readFileBounded(filepath.Join(dir, e.Name(), teamConfigFile), maxResponseBytes)
		if err != nil || !found {
			continue
		}
		var w wireTeam
		unknown, _, err := decodeLenient(teamConfigFile, data, reflect.TypeFor[wireTeam](), func(b []byte) error { w = wireTeam{}; return json.Unmarshal(b, &w) })
		if err != nil {
			continue
		}
		out = append(out, Team{Name: w.Name, CreatedAt: w.CreatedAt.Time(), LeadAgentID: w.LeadAgentID, LeadSessionID: w.LeadSessionID,
			LeadProcess: w.LeadProcess, Members: w.Members, Unknown: unknown})
	}
	return out, nil
}

// TaskItem is one file of a task list; its shape has no sample, so the
// body stays raw.
type TaskItem struct {
	Name string
	Raw  agentio.Raw
}

// TaskList is one tasks/<list>/ dir.
type TaskList struct {
	Name  string
	Items []TaskItem
}

// TaskLists reads tasks/<list>/ dirs: item names and raw bodies (bounded).
// cost: Disk · Read.
func (a *Agent) TaskLists(h Home) ([]TaskList, error) {
	dir := filepath.Join(h.Dir(), tasksDir)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	var out []TaskList
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		tl := TaskList{Name: e.Name()}
		items, _ := os.ReadDir(filepath.Join(dir, e.Name()))
		for _, it := range items {
			if it.IsDir() {
				continue
			}
			ti := TaskItem{Name: it.Name()}
			if data, found, err := readFileBounded(filepath.Join(dir, e.Name(), it.Name()), maxResponseBytes); err == nil && found {
				ti.Raw = agentio.Raw(data)
			}
			tl.Items = append(tl.Items, ti)
		}
		out = append(out, tl)
	}
	return out, nil
}
