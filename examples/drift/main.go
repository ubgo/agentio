// Command drift is the release-routine drift check: it reads the real
// Claude Code and Codex homes of this machine through HostFromOS (or, with
// -fixtures, the fixture homes checked into this repository),
// decodes every modelled file with the library's own readers, and lists what
// the library does not know yet: unknown keys (every agentio.Unknown field
// reached in the decoded values), unknown enum values (open string types
// whose IsKnown is false), unknown transcript record types, decode warnings,
// unknown format versions, and the installed version against Compat().
// Given a saved `claude --help` text it also lists flags missing from the
// feature table.
//
// Cost: Disk · Read plus one process-table read (live sessions). It never
// runs a vendor binary, never calls the network and never spends quota.
// The only secret-store read is Codex Identity on a home whose login is
// kept in the OS keyring (Process · Read through Host.Secrets). Output
// names files, keys and enum spellings only, never a value: map keys
// (paths, project names) and string fields are not printed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

// Limits keep one run quick on homes with thousands of transcripts: the
// newest transcriptSample conversations per home, the last transcriptTail
// bytes of each, and the newest historySample prompt-history entries.
const (
	transcriptSample = 200
	transcriptTail   = 256 << 10
	historySample    = 2000
)

// findingKind says what sort of drift a finding is. The spelling is the
// kind column of a saved report, so a baseline from an earlier run only
// keeps matching while these values stay the same.
type findingKind string

// Finding kinds, in print order.
const (
	kindUnknownKey    findingKind = "unknown-key"
	kindUnknownEnum   findingKind = "unknown-enum"
	kindUnknownRecord findingKind = "unknown-record"
	kindWarning       findingKind = "warning"
	kindFormatVersion findingKind = "format-version"
	kindReadError     findingKind = "read-error"
	kindNewRecordKey  findingKind = "new-record-key"
	kindMissingFlag   findingKind = "flag-not-in-table"
)

// Source and home labels used in more than one place. Every other source
// label names the one reader it is passed with.
const (
	sourceTranscript = "transcript"
	sourceHelp       = "--help"
	// noHome fills the home column of a finding that belongs to no home
	// (a flag in the saved --help text).
	noHome = "-"
	// longFlagPrefix marks a long flag inside a feature description.
	longFlagPrefix = "--"
)

// Path segments the walker appends for container elements; a map key is
// never printed (it can be a project path or a user-chosen name).
const (
	segSlice = "[]"
	segMap   = "{}"
)

// isKnownMethod is the method every open vendor enum type has.
const isKnownMethod = "IsKnown"

// helpFlagPattern matches a long flag in `--help` text.
const helpFlagPattern = `--[a-z][a-z0-9-]+`

// finding is one line of the report; count aggregates identical findings.
type finding struct {
	vendor               agentio.ID
	home, source, detail string
	kind                 findingKind
}

// report collects findings with counts. baseline holds the findings of a
// saved earlier run (baselineKey form); suppressed counts the ones skipped.
type report struct {
	counts     map[finding]int
	baseline   map[string]bool
	suppressed int
}

// add counts one occurrence of f.
func (r *report) add(f finding) {
	if r.counts == nil {
		r.counts = map[finding]int{}
	}
	r.counts[f]++
}

// print writes the findings sorted by vendor, home, kind, source, detail.
func (r *report) print(w io.Writer) {
	fs := make([]finding, 0, len(r.counts))
	for f := range r.counts {
		fs = append(fs, f)
	}
	sort.Slice(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		return strings.Join([]string{string(a.vendor), a.home, string(a.kind), a.source, a.detail}, "\x00") < strings.Join([]string{string(b.vendor), b.home, string(b.kind), b.source, b.detail}, "\x00")
	})
	for _, f := range fs {
		if r.baseline[f.baselineKey()] {
			r.suppressed++
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t(x%d)\n", f.vendor, f.home, f.kind, f.source, f.detail, r.counts[f])
	}
}

// Report columns, tab-separated so a saved run can be read back as a
// baseline: vendor, home, kind, source, detail, count.
const (
	colVendor = iota
	colHome
	colKind
	colSource
	colDetail
	colCount
	numCols
)

// baselineKey is a finding without its home and count: the same unknown
// key in another home, or seen more often, is not new drift.
func (f finding) baselineKey() string {
	return strings.Join([]string{string(f.vendor), string(f.kind), f.source, f.detail}, "\t")
}

// loadBaseline reads a saved drift report; every finding line in it is
// suppressed from this run's output, so after a release only what changed
// since the last recorded run is listed.
func (r *report) loadBaseline(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	r.baseline = map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		cols := strings.Split(line, "\t")
		if len(cols) != numCols {
			continue
		}
		f := finding{vendor: agentio.ID(cols[colVendor]), home: cols[colHome], kind: findingKind(cols[colKind]), source: cols[colSource], detail: cols[colDetail]}
		r.baseline[f.baselineKey()] = true
	}
	return nil
}

// walker reports unknown keys, unknown enum values and warnings found
// anywhere in a decoded value. Why reflection: the drift check must follow
// every struct the library adds without a second hand-kept list.
type walker struct {
	r            *report
	vendor       agentio.ID
	home, source string
}

// add records one finding for the walker's vendor, home and source.
func (w walker) add(kind findingKind, detail string) {
	w.r.add(finding{vendor: w.vendor, home: w.home, source: w.source, kind: kind, detail: detail})
}

// walk descends v and records drift. path is the field path so far
// (".Settings.Hooks[]"); slice and map elements append segSlice/segMap, so
// no map key or string value is ever part of the output.
func (w walker) walk(v reflect.Value, path string) {
	if !v.IsValid() {
		return
	}
	switch v.Type() {
	case reflect.TypeFor[agentio.Unknown]():
		for _, k := range v.MapKeys() {
			w.add(kindUnknownKey, strings.TrimPrefix(path+"."+k.String(), "."))
		}
		return
	case reflect.TypeFor[agentio.Warnings]():
		for i := range v.Len() {
			wn := v.Index(i).Interface().(agentio.Warning)
			w.add(kindWarning, fmt.Sprintf("%s %s %s: %s", wn.Kind, wn.File, wn.Key, wn.Detail))
		}
		return
	case reflect.TypeFor[agentio.Raw]():
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			w.walk(v.Elem(), path)
		}
	case reflect.String:
		if m := v.MethodByName(isKnownMethod); m.IsValid() && v.Len() > 0 && m.Type().NumIn() == 0 && m.Type().NumOut() == 1 && m.Type().Out(0).Kind() == reflect.Bool {
			if !m.Call(nil)[0].Bool() {
				w.add(kindUnknownEnum, fmt.Sprintf("%s %s=%q", strings.TrimPrefix(path, "."), v.Type().Name(), v.String()))
			}
		}
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			if f := t.Field(i); f.IsExported() {
				seg := "." + f.Name
				if f.Anonymous {
					seg = ""
				}
				w.walk(v.Field(i), path+seg)
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return
		}
		for i := range v.Len() {
			w.walk(v.Index(i), path+segSlice)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			w.walk(v.MapIndex(k), path+segMap)
		}
	}
}

// read runs one reader and walks its value or reports its error.
func (w walker) read(source string, val reflect.Value, err error) {
	w.source = source
	switch {
	case errors.Is(err, agentio.ErrUnknownVersion):
		w.add(kindFormatVersion, err.Error())
	case errors.Is(err, agentio.ErrNotFound), errors.Is(err, agentio.ErrUnsupported):
	case err != nil:
		w.add(kindReadError, err.Error())
	}
	w.walk(val, "")
}

// Fixture homes checked into the repository, relative to its root, and
// the root seen from `cd examples`. -fixtures runs the check on them, so
// the output is the same on every machine.
const (
	claudeFixture    = "claude/testdata/homes/2.1.325/userhome"
	codexFixture     = "codex/testdata/homes/0.162.1"
	repoFromExamples = ".."
	// fixtureNoBinaries is a PATH with no vendor binary, so Install reports
	// not-installed instead of finding the real claude or codex.
	fixtureNoBinaries = "PATH=/nonexistent-bin"
	fixtureUser       = "user"
)

// fixtureClock is the fixed time -fixtures reads at, so the run never
// depends on the day it is made.
func fixtureClock() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

func main() {
	helpPath := flag.String("claude-help", "", "a saved `claude --help` text: list long flags missing from claude/features.tsv")
	basePath := flag.String("baseline", "", "a saved earlier drift report: list only findings not in it")
	fixtures := flag.Bool("fixtures", false, "check the fixture homes in this repository instead of this machine's homes")
	flag.Parse()
	host, err := pickHost(*fixtures)
	if err == nil {
		err = run(context.Background(), host, options{helpPath: *helpPath, basePath: *basePath}, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "drift:", err)
		os.Exit(1)
	}
}

// snippet:start pickhost
// pickHost returns the real machine (the default: `task drift` checks the
// homes of whoever runs it) or a Host over the repository fixtures.
func pickHost(fixtures bool) (agentio.Host, error) {
	if !fixtures {
		return agentio.HostFromOS()
	}
	return fixtureHost(repoFromExamples)
}

// fixtureHost builds a Host over the fixture homes under repoRoot: the
// Claude fixture is a whole user home, the Codex fixture one home selected
// with CODEX_HOME. Processes is nil, so live sessions are read from their
// files without a process table; nothing else here reaches Secrets, HTTP
// or Exec.
func fixtureHost(repoRoot string) (agentio.Host, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return agentio.Host{}, err
	}
	home := filepath.Join(root, claudeFixture)
	return agentio.Host{
		UserHome: home,
		User:     fixtureUser,
		Env:      []string{"HOME=" + home, fixtureNoBinaries, codex.EnvHome + "=" + filepath.Join(root, codexFixture)},
		Now:      fixtureClock,
	}, nil
}

// snippet:end pickhost

// snippet:start run
// options are the optional inputs of one run.
type options struct {
	// helpPath is a saved `claude --help` text; "" skips the flag check.
	helpPath string
	// basePath is a saved earlier report; "" lists every finding.
	basePath string
}

// run checks every Claude and Codex home host can see and writes the
// report to out. host is a parameter so a test runs the check on fixture
// homes; main passes HostFromOS() unless -fixtures is given.
func run(ctx context.Context, host agentio.Host, opt options, out io.Writer) error {
	r := &report{}
	if opt.basePath != "" {
		if err := r.loadBaseline(opt.basePath); err != nil {
			return err
		}
	}
	if err := claudeDrift(ctx, claude.New(host), r, out); err != nil {
		return err
	}
	if err := codexDrift(ctx, codex.New(host), r, out); err != nil {
		return err
	}
	if opt.helpPath != "" {
		if err := helpDrift(opt.helpPath, r); err != nil {
			return err
		}
	}
	fmt.Fprintln(out)
	r.print(out)
	fmt.Fprintf(out, "%d distinct findings, %d already in the baseline\n", len(r.counts), r.suppressed)
	return nil
}

// snippet:end run

// versionLine prints the installed version against the tested range.
func versionLine(out io.Writer, vendor agentio.ID, inst agentio.Install, err error, c agentio.Compat) {
	if err != nil {
		fmt.Fprintf(out, "%s: install not found (%v)\n", vendor, err)
		return
	}
	fmt.Fprintf(out, "%s: installed %s, tested %s..%s: %s\n", vendor, inst.Version, c.Min, c.Max, c.Check(inst.Version))
}

// claudeDrift reads every modelled file of every Claude Code home and walks
// the decoded values. Disk · Read only: Remote settings are read from their
// on-disk cache, never fetched.
func claudeDrift(ctx context.Context, c *claude.Agent, r *report, out io.Writer) error {
	inst, ierr := c.Install()
	versionLine(out, claude.ID, inst.Install, ierr, claude.Compat())
	homes, err := c.Homes()
	if err != nil {
		return err
	}
	for _, h := range homes {
		w := walker{r: r, vendor: claude.ID, home: h.Name()}
		fmt.Fprintf(out, "claude home %s\n", h.Name())
		v, err := c.GlobalConfig(h)
		w.read("global-config", reflect.ValueOf(v), err)
		us, err := c.UserSettings(h)
		w.read("settings", reflect.ValueOf(us), err)
		rs, _, err := c.RemoteSettings(h)
		w.read("remote-settings", reflect.ValueOf(rs), err)
		ms, err := c.ManagedSettings(h)
		w.read("managed-settings", reflect.ValueOf(ms), err)
		pl, err := c.PolicyLimits(h)
		w.read("policy-limits", reflect.ValueOf(pl), err)
		st, err := c.Stats(h)
		w.read("stats-cache", reflect.ValueOf(st), err)
		lu, err := c.LastUpdate(h)
		w.read("last-update", reflect.ValueOf(lu), err)
		plugins, err := c.Plugins(h, nil)
		w.read("plugins", reflect.ValueOf(plugins), err)
		mk, err := c.Marketplaces(h)
		w.read("marketplaces", reflect.ValueOf(mk), err)
		bl, err := c.PluginBlocklist(h)
		w.read("plugin-blocklist", reflect.ValueOf(bl), err)
		hk, err := c.Hooks(h, nil)
		w.read("hooks", reflect.ValueOf(hk), err)
		mcp, err := c.MCPServers(h, nil)
		w.read("mcp", reflect.ValueOf(mcp), err)
		sd, err := c.SubagentDefs(h, nil)
		w.read("subagents", reflect.ValueOf(sd), err)
		osty, err := c.OutputStyles(h, nil)
		w.read("output-styles", reflect.ValueOf(osty), err)
		sk, err := c.Skills(h, nil)
		w.read("skills", reflect.ValueOf(sk), err)
		sc, err := c.SlashCommands(h, nil)
		w.read("commands", reflect.ValueOf(sc), err)
		jobs, err := c.Jobs(h)
		w.read("jobs", reflect.ValueOf(jobs), err)
		dm, err := c.Daemon(h)
		w.read("daemon", reflect.ValueOf(dm), err)
		tm, err := c.Teams(h)
		w.read("teams", reflect.ValueOf(tm), err)
		tl, err := c.TaskLists(h)
		w.read("tasks", reflect.ValueOf(tl), err)
		ls, err := c.LiveSessions(h)
		w.read("sessions", reflect.ValueOf(ls), err)
		ide, err := c.IDELocks(h)
		w.read("ide", reflect.ValueOf(ide), err)
		hist, err := c.PromptHistory(h, claude.HistoryQuery{Limit: historySample})
		w.read("history", reflect.ValueOf(hist), err)
		transcriptDrift(ctx, c, h, w, out)
	}
	return nil
}

// transcriptDrift reads the tails of the newest transcripts: unknown record
// types, and unknown keys / enum values inside assistant and user records.
func transcriptDrift(ctx context.Context, c *claude.Agent, h claude.Home, w walker, out io.Writer) {
	convs, err := c.Conversations(ctx, h, agentio.ConversationQuery{Limit: transcriptSample})
	w.read("conversations", reflect.ValueOf(convs), err)
	versions := map[string]int{}
	// firstSeen[type][key] is the oldest Claude Code version that wrote the
	// key on a record of that type. Record has no Unknown (its typed views
	// pick fields per type), so new top-level keys are found by version:
	// a key first written after TestedMin is drift the fixtures predate.
	firstSeen := map[claude.RecordType]map[string]agentio.Version{}
	for _, conv := range convs {
		recs, err := c.TranscriptTail(h, conv.ID, transcriptTail)
		if err != nil {
			w.read(sourceTranscript, reflect.Value{}, err)
			continue
		}
		for _, rec := range recs {
			if rec.Version != "" {
				versions[rec.Version]++
				noteKeys(firstSeen, rec)
			}
			if !rec.Type.IsKnown() {
				w.source = sourceTranscript
				w.add(kindUnknownRecord, string(rec.Type))
				continue
			}
			if a, err := rec.AsAssistant(); err == nil {
				w.read("transcript assistant", reflect.ValueOf(a), nil)
			}
			if u, err := rec.AsUser(); err == nil {
				w.read("transcript user", reflect.ValueOf(u), nil)
			}
		}
	}
	testedMin := agentio.MustVersion(claude.TestedMin)
	w.source = sourceTranscript
	for typ, keys := range firstSeen {
		for k, v := range keys {
			if v.Compare(testedMin) > 0 {
				w.add(kindNewRecordKey, fmt.Sprintf("%s.%s since %s", typ, k, v))
			}
		}
	}
	vs := make([]string, 0, len(versions))
	for v := range versions {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	fmt.Fprintf(out, "  transcripts read %d, record versions %s\n", len(convs), strings.Join(vs, " "))
}

// noteKeys records the top-level key names of rec against its version.
func noteKeys(firstSeen map[claude.RecordType]map[string]agentio.Version, rec claude.Record) {
	v, ok := agentio.ParseVersion(rec.Version)
	if !ok {
		return
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(rec.Raw, &top) != nil {
		return
	}
	m := firstSeen[rec.Type]
	if m == nil {
		m = map[string]agentio.Version{}
		firstSeen[rec.Type] = m
	}
	for k := range top {
		if old, seen := m[k]; !seen || v.Compare(old) < 0 {
			m[k] = v
		}
	}
}

// codexDrift reads config.toml, the login identity and the conversation
// index of every Codex home. Disk · Read, plus a keyring read through
// Host.Secrets for a home whose login lives in the OS keyring (Identity
// does that lookup). No token is ever printed: the walker never prints
// string values.
func codexDrift(ctx context.Context, x *codex.Agent, r *report, out io.Writer) error {
	inst, ierr := x.Install()
	versionLine(out, codex.ID, inst.Install, ierr, codex.Compat())
	homes, err := x.Homes()
	if err != nil {
		return err
	}
	if len(homes) == 0 {
		fmt.Fprintln(out, "codex: no home on this machine")
	}
	for _, h := range homes {
		w := walker{r: r, vendor: codex.ID, home: h.Name()}
		fmt.Fprintf(out, "codex home %s\n", h.Name())
		cfg, warns, err := x.ConfigFileWarnings(h)
		w.read("config.toml", reflect.ValueOf(cfg), err)
		w.walk(reflect.ValueOf(warns), "")
		id, err := x.Identity(h)
		w.read("auth", reflect.ValueOf(id), err)
		convs, err := x.Conversations(ctx, h, codex.ConversationQuery{Limit: transcriptSample})
		w.read("conversations", reflect.ValueOf(convs), err)
	}
	return nil
}

// helpDrift lists long flags in a saved `claude --help` text that no
// features.tsv row names (the table only holds version-gated flags, so a
// hit is a candidate row, not an error).
func helpDrift(path string, r *report) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, f := range claude.Features() {
		for _, tok := range strings.Fields(f.String()) {
			if strings.HasPrefix(tok, longFlagPrefix) {
				known[tok] = true
			}
		}
	}
	seen := map[string]bool{}
	for _, fl := range regexp.MustCompile(helpFlagPattern).FindAllString(string(b), -1) {
		if !known[fl] && !seen[fl] {
			seen[fl] = true
			r.add(finding{vendor: claude.ID, home: noHome, source: sourceHelp, kind: kindMissingFlag, detail: fl})
		}
	}
	return nil
}
