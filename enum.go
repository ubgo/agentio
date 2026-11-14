package agentio

import "fmt"

// enum8 is the shape of every library-owned closed set: a uint8 whose
// String() comes from a names list indexed by value.
type enum8 interface{ ~uint8 }

// unknownEnumFormat renders a value outside its names list, so a corrupted
// or future value prints as something greppable instead of panicking.
const unknownEnumFormat = "%s(%d)"

// enumString returns names[e], or "<kind>(<n>)" when e is out of range.
func enumString[E enum8](e E, kind string, names []string) string {
	if int(e) < len(names) {
		return names[e]
	}
	return fmt.Sprintf(unknownEnumFormat, kind, uint8(e))
}

// enumParse finds s in names. Why a linear scan: every closed set here has
// fewer than 30 values and is parsed only from config or JSON.
func enumParse[E enum8](s string, names []string) (E, bool) {
	for i, n := range names {
		if n == s {
			return E(i), true
		}
	}
	return 0, false
}

// enumUnmarshal is the shared UnmarshalText body: it rejects unknown names
// because these are library-owned sets (vendor-derived sets are open
// strings and never come through here).
func enumUnmarshal[E enum8](dst *E, text []byte, kind string, names []string) error {
	v, ok := enumParse[E](string(text), names)
	if !ok {
		return fmt.Errorf("agentio: unknown %s %q", kind, text)
	}
	*dst = v
	return nil
}

// enumValues lists every value of a closed set in declaration order, the
// canonical iteration list for help text and validation.
func enumValues[E enum8](names []string) []E {
	out := make([]E, len(names))
	for i := range names {
		out[i] = E(i)
	}
	return out
}
