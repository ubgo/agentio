package agentio

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestParseID(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"claude", "codex", "my-agent-2", "a"} {
		if _, err := ParseID(ok); err != nil {
			t.Errorf("ParseID(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Claude", "1abc", "-x", "a_b", "a b", "ä", string(make([]byte, maxIDLen+1))} {
		if _, err := ParseID(bad); !errors.Is(err, ErrInvalidID) {
			t.Errorf("ParseID(%q) = %v, want ErrInvalidID", bad, err)
		}
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"2.1.323 (Claude Code)", "2.1.323", true},
		{"codex-cli 0.162.1", "0.162.1", true},
		{"v1.2", "1.2.0", true},
		{"1.0.0-beta.2", "1.0.0-beta.2", true},
		{"x86 tool 3.4.5", "3.4.5", true},
		{"no version", "", false},
		{"", "", false},
	} {
		v, ok := ParseVersion(tc.in)
		if ok != tc.ok || v.String() != tc.want {
			t.Errorf("ParseVersion(%q) = %q,%v want %q,%v", tc.in, v, ok, tc.want, tc.ok)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	t.Parallel()
	order := []string{"0.9.9", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0", "1.0.1", "1.10.0", "2.1.323"}
	for i := range order {
		for j := range order {
			a, b := MustVersion(order[i]), MustVersion(order[j])
			want := sign(i - j)
			if got := a.Compare(b); got != want {
				t.Errorf("%s vs %s = %d, want %d", order[i], order[j], got, want)
			}
		}
	}
}

func TestCompatAndFeatureSpan(t *testing.T) {
	t.Parallel()
	c := Compat{Min: MustVersion("2.1.300"), Max: MustVersion("2.1.323")}
	for in, want := range map[string]CompatStatus{"": CompatUnknown, "2.1.299": CompatOlder, "2.1.300": CompatInRange, "2.1.323": CompatInRange, "2.1.324": CompatNewer} {
		if got := c.Check(MustVersion(in)); got != want {
			t.Errorf("Check(%q) = %s, want %s", in, got, want)
		}
	}
	if (Compat{}).Check(MustVersion("1.0.0")) != CompatUnknown {
		t.Error("no range must be unknown")
	}
	if w, ok := c.Warning(MustVersion("3.0.0")); !ok || w.Kind != WarnUntestedVersion {
		t.Errorf("Warning(newer) = %v,%v", w, ok)
	}
	if w, ok := c.Warning(Version{}); !ok || w.Kind != WarnVersionUnknown {
		t.Errorf("Warning(unknown) = %v,%v", w, ok)
	}
	if _, ok := (Compat{}).Warning(Version{}); ok {
		t.Error("config-only agents have no range and no warning")
	}
	s := FeatureSpan{Since: MustVersion("2.1.49"), Until: MustVersion("2.2.0")}
	for in, want := range map[string]bool{"": true, "2.1.48": false, "2.1.49": true, "2.1.999": true, "2.2.0": false} {
		if got := s.In(MustVersion(in)); got != want {
			t.Errorf("In(%q) = %v, want %v", in, got, want)
		}
	}
	if (FeatureSpan{}).Known() || !s.Known() || !(FeatureSpan{}).In(MustVersion("9.9.9")) {
		t.Error("Known / zero span")
	}
}

func TestWindowClassAndExpiry(t *testing.T) {
	t.Parallel()
	for d, want := range map[time.Duration]WindowClass{0: WindowUnknown, 3 * time.Hour: WindowShort, 5 * time.Hour: WindowShort, day: WindowDay, week: WindowWeek, 30 * day: WindowMonth} {
		if got := (Window{Length: d}).Class(); got != want {
			t.Errorf("Class(%s) = %s, want %s", d, got, want)
		}
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Hour)
	if (Identity{}).Expired(now) || !(Identity{ExpiresAt: &past}).Expired(now) || (Identity{ExpiresAt: &future}).Expired(now) || !(Identity{ExpiresAt: &now}).Expired(now) {
		t.Error("Identity.Expired")
	}
	if (Usage{Windows: []Window{{Percent: 10}, {Percent: 80}}}).MaxPercent() != 80 {
		t.Error("MaxPercent")
	}
}

func TestOptJSON(t *testing.T) {
	t.Parallel()
	type doc struct {
		A Opt[int64]  `json:"a"`
		B Opt[string] `json:"b,omitzero"`
		C Opt[bool]   `json:"c"`
	}
	var d doc
	if err := json.Unmarshal([]byte(`{"a":0,"c":null}`), &d); err != nil {
		t.Fatal(err)
	}
	if !d.A.Set || d.A.V != 0 || d.B.Set || d.C.Set {
		t.Errorf("presence: %+v", d)
	}
	out, err := json.Marshal(doc{A: Some[int64](5)})
	if err != nil || string(out) != `{"a":5,"c":null}` {
		t.Errorf("marshal = %s, %v", out, err)
	}
	if err := json.Unmarshal([]byte(`{"a":"x"}`), &d); err == nil {
		t.Error("type change must surface as an error for the reader to degrade")
	}
}

func TestExtractUnknownFor(t *testing.T) {
	t.Parallel()
	type inner struct {
		X int `json:"x"`
	}
	type rec struct {
		inner
		Name    string `json:"name,omitempty"`
		Skip    string `json:"-"`
		Plain   int
		Unknown Unknown
	}
	u, err := ExtractUnknownFor([]byte(`{"x":1,"name":"n","Plain":2,"newKey":{"a":1},"other":true}`), reflect.TypeFor[rec]())
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Keys(); !slices.Equal(got, []string{"newKey", "other"}) {
		t.Errorf("unknown keys = %v", got)
	}
}

func TestEnumText(t *testing.T) {
	t.Parallel()
	for _, c := range CapabilityValues() {
		b, _ := c.MarshalText()
		var back Capability
		if err := back.UnmarshalText(b); err != nil || back != c {
			t.Errorf("round trip %s: %v", c, err)
		}
	}
	if Capability(250).String() != "capability(250)" {
		t.Errorf("out of range = %s", Capability(250))
	}
	var b Billing
	if err := b.UnmarshalText([]byte("nope")); err == nil {
		t.Error("closed set must reject unknown names")
	}
	if !InstallNPM.IsKnown() || InstallMethod("cargo").IsKnown() {
		t.Error("open set IsKnown")
	}
}
