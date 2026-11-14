// Command gen writes gemini/features_gen.go from gemini/features.tsv. Run by
// `go generate` in package gemini (working directory = gemini/).
package main

import (
	"fmt"
	"os"

	"github.com/ubgo/agentio/gemini/internal/featuregen"
)

// File names relative to the gemini package directory.
const (
	inFile   = "features.tsv"
	outFile  = "features_gen.go"
	fileMode = 0o644
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	tsv, err := os.ReadFile(inFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", inFile, err)
	}
	src, err := featuregen.Generate(tsv)
	if err != nil {
		return err
	}
	return os.WriteFile(outFile, src, fileMode)
}
