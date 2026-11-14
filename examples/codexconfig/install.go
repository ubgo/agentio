package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/codex"
)

// showInstall covers docs/codex/install.md.
func showInstall(w io.Writer, m machine) error {
	if err := readInstall(w, m); err != nil {
		return err
	}
	if err := readUpdateCache(w, m.host); err != nil {
		return err
	}
	return runBinary(w, m.host)
}

// snippet:start read-install
// readInstall finds codex on Host.Env PATH, resolves the npm symlink and
// reads the version from package.json. It never runs the binary.
func readInstall(w io.Writer, m machine) error {
	a := codex.New(m.host)
	in, err := a.Install()
	if err != nil {
		return err // agentio.ErrNotInstalled when no PATH entry has codex
	}
	fmt.Fprintf(w, "path=%s method=%s version=%s\n", m.rel(in.Path), in.Method, in.Version)
	fmt.Fprintf(w, "package root=%s\n", m.rel(in.PackageRoot))

	v, err := codex.ParseVersion("codex-cli 0.158.0-alpha.2.1")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "parsed %s pre=%q\n", v, v.Pre)

	if up, ok := codex.UpdateCommand(in.Method); ok {
		// A Command to show or hand to a terminal; the library never runs it.
		fmt.Fprintf(w, "update: %s\n", up)
	}
	return nil
}

// snippet:end read-install

// snippet:start update-cache
// readUpdateCache reads version.json, the TUI's last update check.
func readUpdateCache(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	uc, found, err := a.UpdateCache(a.DefaultHome())
	if err != nil || !found {
		return err
	}
	fmt.Fprintf(w, "latest seen=%s checked=%s dismissed=%v\n",
		uc.LatestVersion, uc.LastCheckedAt.Format("2006-01-02"), uc.DismissedVersion != nil)
	return nil
}

// snippet:end update-cache

// snippet:start runner
// runBinary goes through the Runner door: every call runs codex through
// Host.Exec (here a canned runner) and may refresh auth.json, so nothing
// on the read path calls these implicitly.
func runBinary(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	ctx := context.Background()
	r := a.Runner()

	v, err := r.BinaryVersion(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "binary version: %s\n", v)

	st, err := r.LoginStatus(ctx, a.DefaultHome())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "login status: logged-in=%v method=%s\n", st.LoggedIn, st.Method)

	flags, err := r.Features(ctx, a.DefaultHome())
	if err != nil {
		return err
	}
	for _, f := range flags {
		fmt.Fprintf(w, "feature %s stage=%q enabled=%v\n", f.Name, f.Stage, f.Enabled)
	}

	// ParseFeatureList is the same parser, for output you captured yourself.
	parsed := codex.ParseFeatureList([]byte("js_repl experimental false\n"))
	fmt.Fprintf(w, "parsed rows: %d\n", len(parsed))

	cat, err := r.BundledModels(ctx)
	if err != nil {
		return err
	}
	slugs := make([]string, 0, len(cat.Models))
	for _, mi := range cat.Models {
		slugs = append(slugs, mi.Slug)
	}
	fmt.Fprintf(w, "bundled models: %s\n", strings.Join(slugs, ", "))
	return nil
}

// snippet:end runner
