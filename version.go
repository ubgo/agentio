package agentio

import (
	"strconv"
	"strings"
)

// Version is a vendor CLI version ("2.1.323", "0.162.1", "1.0.0-beta.2").
// The zero Version means unknown; every consumer (Compat.Check,
// FeatureSpan.In, launch builders) treats unknown as "build and warn",
// never as a refusal.
type Version struct {
	Major, Minor, Patch int
	// Pre is the pre-release suffix without the dash ("beta.2"); "" for a release.
	Pre string
	// Raw is the text it was parsed from (e.g. "2.1.323 (Claude Code)").
	Raw string
}

// Version parsing bytes.
const (
	versionDot = '.'
	versionPre = '-'
	versionVee = 'v'
)

// ParseVersion finds the first dotted number in s ("codex-cli 0.162.1",
// "2.1.323 (Claude Code)", "v1.2") and parses it with an optional
// "-pre" suffix. Why scan instead of a strict parse: vendor `--version`
// output wraps the number in product text that changes between releases.
// ok is false when s holds no number. Pure.
func ParseVersion(s string) (Version, bool) {
	raw := strings.TrimSpace(s)
	for i := 0; i < len(raw); i++ {
		if !isDigit(raw[i]) {
			continue
		}
		// A number glued to a letter ("x86") is not a version start,
		// except a leading 'v' ("v1.2").
		if i > 0 && isAlnum(raw[i-1]) && raw[i-1] != versionVee {
			continue
		}
		v, ok := parseVersionAt(raw[i:])
		if ok {
			v.Raw = raw
			return v, true
		}
	}
	return Version{}, false
}

// parseVersionAt parses "M[.m[.p]][-pre]" at the start of s.
func parseVersionAt(s string) (Version, bool) {
	var parts [3]int
	n, i := 0, 0
	for n < len(parts) {
		j := i
		for j < len(s) && isDigit(s[j]) {
			j++
		}
		if j == i {
			break
		}
		v, err := strconv.Atoi(s[i:j])
		if err != nil {
			return Version{}, false
		}
		parts[n] = v
		n++
		i = j
		if i < len(s) && s[i] == versionDot && i+1 < len(s) && isDigit(s[i+1]) && n < len(parts) {
			i++
			continue
		}
		break
	}
	if n == 0 {
		return Version{}, false
	}
	v := Version{Major: parts[0], Minor: parts[1], Patch: parts[2]}
	if i < len(s) && s[i] == versionPre {
		j := i + 1
		for j < len(s) && (isAlnum(s[j]) || s[j] == versionDot) {
			j++
		}
		v.Pre = strings.TrimRight(s[i+1:j], string(versionDot))
	}
	return v, true
}

// MustVersion parses a version constant known at compile time (TestedMin);
// an unparsable constant yields the zero (unknown) Version, which a
// vendor's test catches. Pure.
func MustVersion(s string) Version {
	v, _ := ParseVersion(s)
	return v
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isAlnum(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// IsZero reports an unknown version. Pure.
func (v Version) IsZero() bool {
	return v.Major == 0 && v.Minor == 0 && v.Patch == 0 && v.Pre == ""
}

// String renders "M.m.p[-pre]", or "" for the zero Version. Pure.
func (v Version) String() string {
	if v.IsZero() {
		return ""
	}
	s := strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if v.Pre != "" {
		s += string(versionPre) + v.Pre
	}
	return s
}

// MarshalText encodes String(). Pure.
func (v Version) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalText parses with ParseVersion; empty text is the zero Version. Pure.
func (v *Version) UnmarshalText(b []byte) error {
	p, _ := ParseVersion(string(b))
	*v = p
	return nil
}

// Compare orders versions semver-style: -1, 0, +1. A release sorts after
// its pre-releases; pre-release identifiers compare numerically when both
// are numbers, else lexically. Raw is ignored. Pure.
func (v Version) Compare(o Version) int {
	for _, d := range [...]int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		if d != 0 {
			return sign(d)
		}
	}
	return comparePre(v.Pre, o.Pre)
}

// comparePre compares two pre-release strings per semver §11.
func comparePre(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	as, bs := strings.Split(a, string(versionDot)), strings.Split(b, string(versionDot))
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aErr := strconv.Atoi(as[i])
		bn, bErr := strconv.Atoi(bs[i])
		switch {
		case aErr == nil && bErr == nil:
			if an != bn {
				return sign(an - bn)
			}
		case aErr == nil:
			return -1
		case bErr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return sign(len(as) - len(bs))
}

func sign(d int) int {
	switch {
	case d < 0:
		return -1
	case d > 0:
		return 1
	}
	return 0
}

// Compat is the vendor version range a vendor module was tested against
// (SPEC §5.8). Zero bounds mean "no range" (config-only agents).
type Compat struct{ Min, Max Version }

// Check places v against the range. Unknown v or an unset range gives
// CompatUnknown; outside the range is a warning for callers, never a
// refusal (invariant 14). Pure.
func (c Compat) Check(v Version) CompatStatus {
	if v.IsZero() || (c.Min.IsZero() && c.Max.IsZero()) {
		return CompatUnknown
	}
	if !c.Min.IsZero() && v.Compare(c.Min) < 0 {
		return CompatOlder
	}
	if !c.Max.IsZero() && v.Compare(c.Max) > 0 {
		return CompatNewer
	}
	return CompatInRange
}

// Warning returns the drift warning for v, if any: WarnVersionUnknown for an
// unknown version against a real range, WarnUntestedVersion outside it.
// Launch builders append it to Command.Warnings. Pure.
func (c Compat) Warning(v Version) (Warning, bool) {
	if c.Min.IsZero() && c.Max.IsZero() {
		return Warning{}, false
	}
	switch c.Check(v) {
	case CompatUnknown:
		return Warning{Kind: WarnVersionUnknown, Detail: "installed version unknown; tested " + c.Min.String() + ".." + c.Max.String()}, true
	case CompatNewer, CompatOlder:
		return Warning{Kind: WarnUntestedVersion, Detail: "installed " + v.String() + " is outside tested " + c.Min.String() + ".." + c.Max.String()}, true
	}
	return Warning{}, false
}

// FeatureSpan is when a vendor flag or file exists: available from Since
// (inclusive) until Until (exclusive). A zero bound is unknown, never
// invented (invariant 15).
type FeatureSpan struct{ Since, Until Version }

// In reports whether the feature may exist in v: true unless v is provably
// before Since or at/after Until; an unknown v is always true (build and
// warn, SPEC §5.8). Pure.
func (s FeatureSpan) In(v Version) bool {
	if v.IsZero() {
		return true
	}
	if !s.Since.IsZero() && v.Compare(s.Since) < 0 {
		return false
	}
	if !s.Until.IsZero() && v.Compare(s.Until) >= 0 {
		return false
	}
	return true
}

// Known reports whether either bound is set. Pure.
func (s FeatureSpan) Known() bool { return !s.Since.IsZero() || !s.Until.IsZero() }
