package claude

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
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// maxTypeChangeRetries bounds how many top-level keys decodeLenient moves
// to Unknown for a type change before it gives up; a file where every key
// changed type is not this format any more.
const maxTypeChangeRetries = 32

// jsonTagSep separates the key from options in a json struct tag.
const jsonTagSep = ","

// jsonNull is the literal JSON null.
const jsonNull = "null"

// millis is Unix milliseconds, the epoch form Claude Code writes in
// ~/.claude.json, sessions/*.json, daemon files and history.jsonl. It never
// leaves the decoder: exported fields are time.Time.
type millis int64

// Time converts to UTC; 0 (unset) stays the zero time.
func (m millis) Time() time.Time {
	if m <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(m)).UTC()
}

// timePtr converts to a pointer; nil when unset.
func (m millis) timePtr() *time.Time {
	if m <= 0 {
		return nil
	}
	t := m.Time()
	return &t
}

// UnmarshalJSON accepts a number or null; a float (seen in some caches) is
// truncated.
func (m *millis) UnmarshalJSON(b []byte) error {
	if string(b) == jsonNull {
		*m = 0
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*m = millis(f)
	return nil
}

// jsonKeys lists the JSON keys a wire struct models, from its tags. Why
// reflection: the key list must follow the struct, never a second hand-kept
// list that drifts.
func jsonKeys(t reflect.Type) map[string]bool {
	keys := map[string]bool{}
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, jsonTagSep)
		if name != "" {
			keys[name] = true
		}
	}
	return keys
}

// decodeLenient decodes one JSON object through decode (which unmarshals
// into a wire struct of type known) and returns the keys the struct does not
// model. A key whose value changed type is moved to Unknown with a
// WarnTypeChanged warning and the rest of the object still decodes
// (invariant 17). file names the source for warnings; it never holds a value.
func decodeLenient(file string, data []byte, known reflect.Type, decode func([]byte) error) (agentio.Unknown, agentio.Warnings, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("claude: decode %s: %w", file, err)
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
		if !errors.As(err, &te) || te.Field == "" {
			return unknown, warns, fmt.Errorf("claude: decode %s: %w", file, err)
		}
		top, _, _ := strings.Cut(te.Field, ".")
		v, ok := raw[top]
		if !ok {
			return unknown, warns, fmt.Errorf("claude: decode %s: %w", file, err)
		}
		if unknown == nil {
			unknown = agentio.Unknown{}
		}
		unknown[top] = v
		delete(raw, top)
		warns = append(warns, agentio.Warning{Kind: agentio.WarnTypeChanged, File: file, Key: top, Detail: "expected " + te.Type.String()})
		if body, err = json.Marshal(raw); err != nil {
			return unknown, warns, fmt.Errorf("claude: decode %s: %w", file, err)
		}
	}
	return unknown, warns, fmt.Errorf("claude: decode %s: too many type changes", file)
}

// readFileBounded reads a whole small file, refusing anything above limit so
// a corrupted multi-GB file never lands in memory. found is false (nil
// error) when the file does not exist.
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
		return nil, true, fmt.Errorf("claude: %s is larger than %d bytes", path, limit)
	}
	return data, true, nil
}

// readTail returns the last n bytes of path, dropping a first partial line
// when the read starts mid-file. A file shorter than n is returned whole.
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

// headLines calls fn for each complete line within the first limit bytes
// of path; fn returns false to stop.
func headLines(path string, limit int, fn func([]byte) bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(io.LimitReader(f, int64(limit)))
	sc.Buffer(make([]byte, 0, scanStartBytes), limit)
	for sc.Scan() {
		if !fn(sc.Bytes()) {
			return nil
		}
	}
	// A line longer than the window ends the head scan; that is not an error.
	if err := sc.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return err
	}
	return nil
}

// splitLines splits a buffer into non-empty lines.
func splitLines(b []byte) [][]byte {
	var out [][]byte
	for line := range bytes.SplitSeq(b, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) > 0 {
			out = append(out, line)
		}
	}
	return out
}

// isNotExist reports a missing file or dir.
func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

// boolPtr returns a pointer to b, for presence-tracked bools.
func boolPtr(b bool) *bool { return &b }

// rawOrNil keeps a raw JSON value unless it is absent or null.
func rawOrNil(b json.RawMessage) agentio.Raw {
	if len(b) == 0 || string(b) == jsonNull {
		return nil
	}
	return agentio.Raw(b)
}

// readAllBounded reads a response body up to maxResponseBytes.
func readAllBounded(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxResponseBytes))
}
