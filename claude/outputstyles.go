package claude

import (
	"path/filepath"
	"strings"
)

// Output style frontmatter keys (docs).
const (
	fmKeepCodingInstructions = "keep-coding-instructions"
	fmForceForPlugin         = "force-for-plugin"
)

// OutputStyle is one output-styles/*.md file.
type OutputStyle struct {
	Name, Description, File string
	Scope                   SkillScope
	KeepCodingInstructions  *bool
	ForceForPlugin          *bool
	// Builtin is true for styles Claude Code ships; none are listed from
	// disk (their names on this version were not recorded).
	Builtin bool
	Err     error
}

// OutputStyles lists output-styles/*.md of the user, project and plugin
// dirs. cost: Disk · Read (frontmatter only).
func (a *Agent) OutputStyles(h Home, p *Project) ([]OutputStyle, error) {
	var out []OutputStyle
	for _, r := range a.roots(h, p, outputStylesDir) {
		for _, f := range markdownIn(r.dir) {
			d, _, err := readFront(f, frontmatterReadBytes)
			s := OutputStyle{Name: d.String(fmName), Description: d.String(fmDescription), File: f, Scope: r.scope,
				KeepCodingInstructions: docBool(d, fmKeepCodingInstructions), ForceForPlugin: docBool(d, fmForceForPlugin), Err: err}
			if s.Name == "" {
				s.Name = strings.TrimSuffix(filepath.Base(f), markdownExt)
			}
			out = append(out, s)
		}
	}
	return out, nil
}
