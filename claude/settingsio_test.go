package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// roundTripBody has modelled keys, unknown top-level keys and unknown keys
// nested inside modelled objects.
const roundTripBody = `{
  "model": "sonnet",
  "effortLevel": "high",
  "apiKeyHelper": "~/bin/helper.sh",
  "env": {"CLAUDE_CODE_API_KEY_HELPER_TTL_MS": "10000"},
  "permissions": {"allow": ["Bash(ls:*)"], "futureRuleKind": ["x"]},
  "sandbox": {"enabled": true, "network": {"allowUnixSockets": ["/tmp/s"]}},
  "statusLine": {"type": "command", "command": "status.sh", "padding": 1, "newKnob": true},
  "brandNewKey": {"nested": [1, 2, 3]},
  "anotherNewKey": "v"
}`

// TestSettingsRoundTrip: ParseSettings then json.Marshal keeps every key
// and value, modelled or not, on the fixtures and a body full of
// unmodelled keys.
func TestSettingsRoundTrip(t *testing.T) {
	t.Parallel()
	bodies := map[string][]byte{"synthetic": []byte(roundTripBody)}
	files, _ := filepath.Glob(filepath.Join("testdata", "homes", "*", "userhome", ".claude*", "settings.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		bodies[f] = b
	}
	for name, body := range bodies {
		s, err := ParseSettings(name, body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var want, got map[string]json.RawMessage
		if err := json.Unmarshal(body, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		for k, v := range want {
			if !jsonEqual(t, v, got[k]) {
				t.Errorf("%s: key %s = %s, want %s", name, k, got[k], v)
			}
		}
		for k := range got {
			if _, ok := want[k]; !ok {
				t.Errorf("%s: invented key %s", name, k)
			}
		}
	}
}

// jsonEqual compares two JSON values semantically (key order and
// whitespace ignored, at every depth).
func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	return canonJSON(a) == canonJSON(b)
}

// canonJSON renders a JSON value with sorted keys and no whitespace.
func canonJSON(r json.RawMessage) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(r, &m) == nil && m != nil {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		var sb strings.Builder
		sb.WriteString("{")
		for _, k := range keys {
			sb.WriteString(k + ":" + canonJSON(m[k]) + ",")
		}
		return sb.String() + "}"
	}
	var arr []json.RawMessage
	if json.Unmarshal(r, &arr) == nil && arr != nil {
		var sb strings.Builder
		sb.WriteString("[")
		for _, e := range arr {
			sb.WriteString(canonJSON(e) + ",")
		}
		return sb.String() + "]"
	}
	var c bytes.Buffer
	if err := json.Compact(&c, r); err != nil {
		return string(r)
	}
	return c.String()
}

// TestValidateSettings: unknown keys are listed (dotted when nested) and
// kept; a modelled key with the wrong type is a type error.
func TestValidateSettings(t *testing.T) {
	t.Parallel()
	c, err := ValidateSettings([]byte(roundTripBody))
	if err != nil || !c.OK() {
		t.Fatalf("%+v %v", c, err)
	}
	want := []string{"anotherNewKey", "brandNewKey", "permissions.futureRuleKind", "sandbox.network", "statusLine.newKnob"}
	if !slices.Equal(c.UnknownKeys, want) {
		t.Errorf("unknown = %v, want %v", c.UnknownKeys, want)
	}
	bad, err := ValidateSettings([]byte(`{"cleanupPeriodDays": "thirty", "model": "opus"}`))
	if err != nil || bad.OK() || bad.TypeErrors[0].Key != "cleanupPeriodDays" || len(bad.UnknownKeys) != 0 {
		t.Errorf("type error = %+v %v", bad, err)
	}
	if _, err := ValidateSettings([]byte(`[1]`)); err == nil {
		t.Error("an array is not a settings object")
	}
}

// TestSettingsSchema: every modelled key is listed, enums carry values,
// and the schema stays open to unknown keys.
func TestSettingsSchema(t *testing.T) {
	t.Parallel()
	keys := SettingsKeys()
	for _, k := range []string{"apiKeyHelper", "effortLevel", "env", "permissions", "model"} {
		if !slices.Contains(keys, k) {
			t.Errorf("keys lack %s", k)
		}
	}
	s := SettingsSchema()
	if s.AdditionalProperties != nil || !slices.Equal(s.Properties["effortLevel"].Enum, []string{"low", "medium", "high", "xhigh", "max"}) {
		t.Errorf("schema = %+v", s.Properties["effortLevel"])
	}
	if len(keys) != len(reflect.VisibleFields(reflect.TypeFor[Settings]()))-2 { // Unknown and Warnings
		t.Errorf("keys = %d, fields = %d", len(keys), len(reflect.VisibleFields(reflect.TypeFor[Settings]())))
	}
}

// TestLaunchSettings: a 0600 file named by the run id, never reused; inline
// returns the JSON; non-objects and bad run ids are refused.
func TestLaunchSettings(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "runs")
	body := []byte(`{"apiKeyHelper":"~/bin/h.sh","x":1}`)
	p, err := LaunchSettings(body, SettingsViaFile, dir, "20261114-120000-ab12")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "settings-20261114-120000-ab12.json" {
		t.Errorf("path = %s", p)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != launchSettingsFileMode {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
	if got, _ := os.ReadFile(p); string(got) != string(body) {
		t.Errorf("content = %s", got)
	}
	if _, err := LaunchSettings(body, SettingsViaFile, dir, "20261114-120000-ab12"); err == nil {
		t.Error("an existing run file must be refused")
	}
	if s, err := LaunchSettings(body, SettingsViaInline, "", ""); err != nil || s != string(body) {
		t.Errorf("inline = %q %v", s, err)
	}
	if _, err := LaunchSettings([]byte(`"x"`), SettingsViaInline, "", ""); err == nil {
		t.Error("a non-object must be refused")
	}
	if _, err := LaunchSettings(body, SettingsViaFile, dir, "../escape"); !errors.Is(err, ErrBadRunID) {
		t.Errorf("bad run id = %v", err)
	}
}
