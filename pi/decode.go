package pi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Read bounds: pi's JSON files are small, sessions can be large.
const (
	// maxJSONFileBytes caps a settings, mcp, trust or auth read so a
	// corrupted multi-GB file never lands in memory.
	maxJSONFileBytes = 8 << 20
	// headLimit is the head window for a session header: the first line is
	// the header (PKG/docs/session-format.md:64-75), a few hundred bytes.
	headLimit = 64 << 10
	// tailBytes is the tail window read for a session's title and last
	// prompt: never the whole transcript.
	tailBytes = 256 << 10
	// scanStartBytes is the scanner's initial buffer.
	scanStartBytes = 64 << 10
	// maxTypeChangeRetries bounds how many keys decodeLenient moves to
	// Unknown for a type change before giving up.
	maxTypeChangeRetries = 32
)

// millis is Unix milliseconds, the epoch form pi writes in message
// timestamps and auth.json expires. It never leaves the decoder.
type millis int64

// Time converts to UTC; 0 (unset) stays the zero time.
func (m millis) Time() time.Time {
	if m <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(m)).UTC()
}

// readFileBounded reads a whole small file, refusing anything above limit.
// found is false (nil error) when the file does not exist. Disk · Read.
func readFileBounded(path string, limit int64) (data []byte, found bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	data, err = io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, true, err
	}
	if int64(len(data)) > limit {
		return nil, true, fmt.Errorf("pi: %s is larger than %d bytes", path, limit)
	}
	return agentio.StripBOM(data), true, nil
}

// readTail returns the last n bytes of path, dropping a first partial line
// when the read starts mid-file. Disk · Read.
func readTail(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := max(info.Size()-n, 0)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if start > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	return buf, nil
}

// firstLine returns the first complete line within headLimit bytes of path.
// Disk · Read.
func firstLine(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(io.LimitReader(f, headLimit))
	sc.Buffer(make([]byte, 0, scanStartBytes), headLimit)
	if sc.Scan() {
		return agentio.StripBOM(bytes.Clone(sc.Bytes())), nil
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, io.ErrUnexpectedEOF
}

// splitLines splits a buffer into non-empty lines.
func splitLines(b []byte) [][]byte {
	var out [][]byte
	for line := range bytes.SplitSeq(agentio.StripBOM(b), []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) > 0 {
			out = append(out, line)
		}
	}
	return out
}

// jsonKeys lists the JSON keys a wire struct models, from its tags.
func jsonKeys(t reflect.Type) map[string]bool {
	keys := map[string]bool{}
	for _, k := range agentio.JSONKeys(t) {
		keys[k] = true
	}
	return keys
}

// decodeLenient decodes one JSON object through decode (which unmarshals
// into a wire struct of type known) and returns the keys the struct does not
// model. A key whose value changed type is moved to Unknown with a
// WarnTypeChanged warning and the rest still decodes (invariant 17). file
// names the source for warnings; it never holds a value.
func decodeLenient(file string, data []byte, known reflect.Type, decode func([]byte) error) (agentio.Unknown, agentio.Warnings, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("pi: decode %s: %w", file, err)
	}
	keys := jsonKeys(known)
	var unknown agentio.Unknown
	var warns agentio.Warnings
	for k, v := range raw {
		if !keys[k] {
			if unknown == nil {
				unknown = agentio.Unknown{}
			}
			unknown[k] = v
		}
	}
	body := data
	for range maxTypeChangeRetries {
		err := decode(body)
		var te *json.UnmarshalTypeError
		if err == nil {
			return unknown, warns, nil
		}
		if !errors.As(err, &te) {
			return unknown, warns, fmt.Errorf("pi: decode %s: %w", file, err)
		}
		top, _, _ := strings.Cut(te.Field, ".")
		if top == "" {
			// A type error from a field's own UnmarshalJSON (agentio.Opt)
			// carries no field path: find the key by decoding each alone.
			top = failingKey(raw, decode)
		}
		v, ok := raw[top]
		if !ok {
			return unknown, warns, fmt.Errorf("pi: decode %s: %w", file, err)
		}
		if unknown == nil {
			unknown = agentio.Unknown{}
		}
		unknown[top] = v
		delete(raw, top)
		warns = append(warns, agentio.Warning{Kind: agentio.WarnTypeChanged, File: file, Key: top, Detail: "expected " + te.Type.String()})
		if body, err = json.Marshal(raw); err != nil {
			return unknown, warns, fmt.Errorf("pi: decode %s: %w", file, err)
		}
	}
	return unknown, warns, fmt.Errorf("pi: decode %s: too many type changes", file)
}

// failingKey returns the first key (sorted) whose value alone fails
// decode; "" when none does. Pure apart from decode.
func failingKey(raw map[string]json.RawMessage, decode func([]byte) error) string {
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		b, err := json.Marshal(map[string]json.RawMessage{k: raw[k]})
		if err == nil && decode(b) != nil {
			return k
		}
	}
	return ""
}

// isNotExist reports a missing file or dir.
func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

// joinList renders a list flag value (read,grep,find,ls).
func joinList(vs []string) string { return strings.Join(vs, listSep) }
