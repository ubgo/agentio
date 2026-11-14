package agentcli

import "fmt"

// ID names an agent ("claude", "codex", or a config-defined one). Charset
// [a-z0-9-], first byte a letter, at most maxIDLen bytes. Built-ins are
// typed constants in the vendor packages; config strings go through ParseID.
type ID string

// maxIDLen bounds an ID: it ends up in file names and flags.
const maxIDLen = 64

// ParseID validates s as an ID. Pure.
func ParseID(s string) (ID, error) {
	if s == "" || len(s) > maxIDLen {
		return "", fmt.Errorf("%w: %q: length must be 1..%d", ErrInvalidID, s, maxIDLen)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case (c >= '0' && c <= '9') || c == '-':
			if i == 0 {
				return "", fmt.Errorf("%w: %q: must start with a letter", ErrInvalidID, s)
			}
		default:
			return "", fmt.Errorf("%w: %q: only [a-z0-9-] allowed", ErrInvalidID, s)
		}
	}
	return ID(s), nil
}

// String returns the ID. Pure.
func (id ID) String() string { return string(id) }
