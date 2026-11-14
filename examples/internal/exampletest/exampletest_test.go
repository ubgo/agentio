package exampletest

import "testing"

// TestNormalizeHomePaths: home-relative paths get forward slashes; quoted
// escapes and other backslashes stay.
func TestNormalizeHomePaths(t *testing.T) {
	in := `path=~\.claude\settings.json title="a\nb" quote=\" key=C:\x`
	want := `path=~/.claude/settings.json title="a\nb" quote=\" key=C:\x`
	if got := normalizeHomePaths(in); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
