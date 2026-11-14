# Codex install and updates

Where `codex` is installed, how (npm, bun, pnpm, Homebrew cask, standalone), which version, and how to update it, read from disk without running the binary. When you do need the binary's own answers (`--version`, `login status`, `features list`, the bundled model catalog), the `Runner` door runs it through `Host.Exec` and says so in its name.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` from [The Codex agent](agent.md).
- `Install` walks `Host.Env` `PATH` (never a shell). For a version it needs an npm-family layout: `bin/codex` resolving into `node_modules/@openai/codex`, where `package.json` holds the version.
- `Runner` calls need a `Host.Exec`; examples use a canned runner, real programs get one from `agentio.HostFromOS()`.

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/), which puts a fake npm install of codex 0.162.1 on `PATH` and answers Runner calls with recorded output.

## Step 1 — Read the install

`Install` returns the first `codex` on `PATH`, any shadowed ones, the resolved real path, the install method and the version. `codex.ParseVersion` accepts `codex-cli 0.162.1` or a bare version and keeps pre-release text. `codex.UpdateCommand(method)` is the vendor's own update argv for that method; the library never runs it.

<!-- snippet: examples/codexconfig/install.go#read-install -->
```go
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
```

## Step 2 — Read the update cache

`version.json` is written only by the TUI's startup check. `UpdateCache` returns it with `found=false` when absent.

<!-- snippet: examples/codexconfig/install.go#update-cache -->
```go
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
```

## Step 3 — Run the binary through the Runner

`a.Runner()` is the Process door. `BinaryVersion` runs `codex --version`, `LoginStatus` runs `codex login status`, `Features` runs `codex features list`, `BundledModels` runs `codex debug models --bundled`. `ParseFeatureList` is the parser `Features` uses, for output you captured yourself.

<!-- snippet: examples/codexconfig/install.go#runner -->
```go
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
```

## Try it

```sh
cd examples && go run ./codexconfig
```

The `== install` section:

```text
== install
path=~/npm/bin/codex method=npm version=0.162.1
package root=~/npm/lib/node_modules/@openai/codex
parsed 0.158.0-alpha.2.1 pre="alpha.2.1"
update: npm install -g @openai/codex
latest seen=0.156.1 checked=2026-09-24 dismissed=false
binary version: 0.162.1
login status: logged-in=true method=apikey
feature hooks stage="stable" enabled=true
feature unified_exec stage="under development" enabled=false
parsed rows: 1
bundled models: gpt-6-luna
```

## Pitfalls

- **Every Runner method may rewrite `auth.json`.** Any Codex subcommand can refresh the login and rewrite the file, so all Runner methods are labelled MayTouchLogin and nothing on a read path calls them. Prefer `Install` (disk) over `BinaryVersion` (process) for the version.
- **No version without `package.json`.** For Homebrew, standalone or an unknown layout, `Install.Version` is zero. Launch builders then warn `version-unknown` instead of gating flags (see [Codex launch](launch.md#step-4--handle-unavailable-features)).
- **`ErrNotInstalled` when no `PATH` entry has `codex`.** It is an error from `Install`; `TargetFor` treats it as "version unknown" instead.
- **`UpdateCommand` is a package function returning a bare `npm`/`bun`/`pnpm`/`brew` path**, not resolved through `Host.Env` `PATH`, and `false` for standalone installs (the installer script is not modelled). Claude's equivalent is a method on the agent.
- **`codex.ParseVersion` returns an error; `agentio.ParseVersion` returns a bool.** Same name, different result shape.
- **A stale update cache is normal.** `LatestVersion` can be older than the installed version (the cache is only refreshed by the TUI); the doctor reports this as info, not as "update available".
- **`Features` and `FeatureFlag` are the vendor's runtime feature switches** (`[features]` in `config.toml`). The version-gated launch flags are the separate `codex.Feature` enum ([Compat features](../compat/features.md)).

## Related

- [Install and identity (neutral)](../capabilities/install-identity.md), [The doors](../capabilities/doors.md)
- [Versions](../core/versions.md), [Compat](../compat/compat.md)
- [Codex doctor](doctor.md), [Codex launch](launch.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
