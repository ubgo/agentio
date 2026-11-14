// Command values shows the small value types every agentio result is made
// of: error sentinels and rate limits (errors.go), presence-tracked optional
// fields, unknown vendor keys and drift warnings (values.go), versions
// (versions.go) and enums (enums.go). Everything here is Pure: no file, no
// network, no process.
//
//	cd examples && go run ./values
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "values:", err)
		os.Exit(1)
	}
}

// run prints every section in the order the guides read them.
func run(w io.Writer) error {
	p := printer{w: w}
	for _, step := range []func(printer) error{
		matchSentinels, rateLimits,
		optionalFields, unknownKeys, collectWarnings,
		parseVersions, compareVersions,
		parseEnums, listEnums,
	} {
		if err := step(p); err != nil {
			return err
		}
	}
	return nil
}

// printer writes one line per call.
type printer struct{ w io.Writer }

// line prints one line.
// A write error on the output (a closed pipe) is dropped: the example has
// nothing better to do with it than stop printing.
func (p printer) line(s string) { _, _ = fmt.Fprintln(p.w, s) }
