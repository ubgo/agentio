# Codex install and updates

Where `codex` is installed, how (npm, bun, pnpm, Volta, [Homebrew cask] or formula(ds:block?id=codex-agent-install-7wqjtcqk), standalone), which version, and how to update it, read from disk without running the binary. When you do need the binary's own answers (`--version`, `login status`, `features list`, the bundled model catalog), the [`Runner`](ds:block?id=codex-runner-476qyke4) door runs it through `Host.Exec` and says so in its name.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` from [The Codex agent](agent.md).
- [`Install`](ds:block?id=codex-agent-install-7wqjtcqk) walks `Host.Env` `PATH` (never a shell). On Windows it tries each `PATHEXT` extension (`codex.cmd`). For a version it needs a layout that leads to the package: `bin/codex` resolving into `node_modules/@openai/codex` (npm, nvm), an npm or pnpm launcher shim naming it (read with `agentio.ShimTarget`), a bun or Volta bin dir, or a Homebrew formula (`Cellar/codex/<version>`); the package dir is where [`package.json`](ds:block?id=codex-packageversion-gts9xx4s) holds the version.
- `Runner` calls need a `Host.Exec`; examples use a canned runner, real programs get one from `agentio.HostFromOS()`.

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/), which puts a fake npm install of codex 0.162.1 on `PATH` and answers Runner calls with recorded output.

## Step 1 — Read the install

`Install` returns the first `codex` on `PATH`, any shadowed ones, the resolved real path, the install method and the version. [`codex.ParseVersion`](ds:block?id=codex-parseversion-zx9z9rav) accepts `codex-cli 0.162.1` or a bare version and keeps pre-release text; it returns `(agentio.Version, bool)` like `agentio.ParseVersion`. [`a.UpdateCommand()`](ds:block?id=codex-agent-updatecommand-mmv9gxga) is the vendor's own update command for the installed codex: it reads the install method with `Install` and resolves the package manager (`npm`, `bun`, `pnpm`, `brew`, `volta`; `npm.cmd` on Windows) on `Host.Env` `PATH`, and a Homebrew formula install upgrades without `--cask`; the library never runs it.

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

	v, ok := codex.ParseVersion("codex-cli 0.158.0-alpha.2.1")
	fmt.Fprintf(w, "parsed %s pre=%q ok=%t\n", v, v.Pre, ok)

	// UpdateCommand resolves npm (bun, pnpm, brew) on Host PATH for the
	// install method above. A Command to show or hand to a terminal; the
	// library never runs it.
	up, err := a.UpdateCommand()
	switch {
	case errors.Is(err, agentio.ErrNotInstalled):
		fmt.Fprintf(w, "update: package manager not on PATH\n")
	case errors.Is(err, agentio.ErrUnsupported):
		fmt.Fprintf(w, "update: none for this install method\n")
	case err != nil:
		return err
	default:
		fmt.Fprintf(w, "update: %s %s\n", m.rel(up.Path), strings.Join(up.Args, " "))
	}
	return nil
}
```

## Step 2 — Read the update cache

`version.json` is written only by the TUI's startup check. [`UpdateCache`](ds:block?id=codex-agent-updatecache-7c64td97) returns it with `found=false` when absent.

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
		uc.LatestVersion, uc.LastCheckedAt.UTC().Format("2006-01-02"), uc.DismissedVersion != nil)
	return nil
}
```

## Step 3 — Run the binary through the Runner

[`a.Runner()`](ds:block?id=codex-agent-runner-v8k7es62) is the Process door. [`BinaryVersion`](ds:block?id=codex-runner-binaryversion-fz7k9npd) runs `codex --version`, [`LoginStatus`](ds:block?id=codex-runner-loginstatus-xg3qprt5) runs `codex login status`, `Features` runs `codex features list`, [`BundledModels`](ds:block?id=codex-runner-bundledmodels-cpzy9yw7) runs `codex debug models --bundled`. [`ParseFeatureList`](ds:block?id=codex-parsefeaturelist-ptd3dvzf) is the parser `Features` uses, for output you captured yourself.

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
parsed 0.158.0-alpha.2.1 pre="alpha.2.1" ok=true
update: ~/npm/bin/npm install -g @openai/codex
latest seen=0.156.1 checked=2026-09-24 dismissed=false
binary version: 0.162.1
login status: logged-in=true method=apikey
feature hooks stage="stable" enabled=true
feature unified_exec stage="under development" enabled=false
parsed rows: 1
bundled models: gpt-6-luna
```

## Pitfalls

- **Every Runner method may rewrite [`auth.json`](ds:block?id=codex-runner-476qyke4).** Any Codex subcommand can refresh the login and rewrite the file, so all Runner methods are labelled MayTouchLogin and nothing on a read path calls them. Prefer `Install` (disk) over `BinaryVersion` (process) for the version.
- **No version without `package.json`.** For a Homebrew cask, standalone or an unknown layout (a Homebrew formula gives the version in its `Cellar` path), [`Install.Version`](ds:block?id=codex-agent-install-7wqjtcqk) is zero. Launch builders then warn [`version-unknown`](ds:block?id=codex-newbuilder-45cx4x37) instead of gating flags (see [Codex launch](launch.md#step-4--handle-unavailable-features)).
- **[`ErrNotInstalled`](ds:block?id=codex-agent-install-7wqjtcqk) when no `PATH` entry has `codex`.** It is an error from `Install`; [`TargetFor`](ds:block?id=codex-agent-targetfor-nzpuwqwd) treats it as "version unknown" instead.
- **[`UpdateCommand`](ds:block?id=codex-agent-updatecommand-mmv9gxga) needs the package manager on `PATH`.** It is a method with the same shape as Claude's (`(agentio.Command, error)`, Disk · Read): `agentio.ErrNotInstalled` when codex or the package manager is not on `Host.Env` `PATH`, `agentio.ErrUnsupported` for a standalone or unknown install (the installer script is not modelled). `Command.Path` is the resolved absolute path, never a bare `npm`.
- **A stale update cache is normal.** [`LatestVersion`](ds:block?id=codex-agent-checks-kj5pjpxm) can be older than the installed version (the cache is only refreshed by the TUI); the doctor reports this as info, not as "update available".
- **`Features` and [`FeatureFlag`](ds:block?id=codex-featureflag-daqfcd4n) are the vendor's runtime feature switches** (`[features]` in `config.toml`). The version-gated launch flags are the separate [`codex.Feature`](ds:block?id=codex-feature-u2cyacgu) enum ([Compat features](../compat/features.md)).

## Related

- [Install and identity (neutral)](../capabilities/install-identity.md), [The doors](../capabilities/doors.md)
- [Versions](../core/versions.md), [Compat](../compat/compat.md)
- [Codex doctor](doctor.md), [Codex launch](launch.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
