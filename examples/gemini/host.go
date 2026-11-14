package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/gemini"
)

// Fixture layout: the 0.68.0 home and its workspace, found by walking up
// from the working directory (go run from examples/, go test from
// examples/gemini).
const (
	fixtureDir  = "gemini/testdata/homes/0.68.0"
	fixtureUser = "userhome"
	fixtureProj = "project"
	projectName = "demo"
	gitDirName  = ".git"
	// Placeholders the fixture carries instead of machine paths.
	phProject = "@PROJECT@"
	phHash    = "@PROJECT_HASH@"
	// installedVersion is the version the fake npm install reports.
	installedVersion = "0.68.0"
	dirPerm          = 0o700
	filePerm         = 0o600
	execPerm         = 0o700
)

// errNoFixture means no parent dir holds the fixture.
var errNoFixture = errors.New("gemini example: fixture not found above the working dir")

// machine is the fake machine: a Host whose every input is a temp dir.
type machine struct {
	host    agentio.Host
	project string
	tmp     string
}

// setup copies the fixture home and project into tmp, replaces the
// placeholders (JSON-encoded inside JSON, so a Windows path stays valid),
// and installs a fake npm package of gemini on PATH. Disk · Write (tmp).
func setup(tmp string) (machine, error) {
	root, err := fixtureRoot()
	if err != nil {
		return machine{}, err
	}
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "work", projectName)
	if err := agentio.CopyTree(filepath.Join(root, fixtureUser), home); err != nil {
		return machine{}, err
	}
	if err := agentio.CopyTree(filepath.Join(root, fixtureProj), project); err != nil {
		return machine{}, err
	}
	if err := os.MkdirAll(filepath.Join(project, gitDirName), dirPerm); err != nil {
		return machine{}, err
	}
	if err := fillPlaceholders(filepath.Join(home, ".gemini"), project); err != nil {
		return machine{}, err
	}
	bin, err := fakeNPMInstall(filepath.Join(tmp, "npm"))
	if err != nil {
		return machine{}, err
	}
	h := agentio.Host{
		GOOS:     runtime.GOOS,
		UserHome: home,
		Env:      []string{"HOME=" + home, "PATH=" + bin},
		Now:      func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) },
	}
	return machine{host: h, project: project, tmp: tmp}, nil
}

// fixtureRoot walks up for the fixture. Disk · Read.
func fixtureRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, filepath.FromSlash(fixtureDir))
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p, nil
		}
		if filepath.Dir(dir) == dir {
			return "", errNoFixture
		}
		dir = filepath.Dir(dir)
	}
}

// fillPlaceholders rewrites the placeholders in every file under dir.
// Disk · Write (tmp).
func fillPlaceholders(dir, project string) error {
	quoted, err := json.Marshal(project)
	if err != nil {
		return err
	}
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out := bytes.ReplaceAll(b, []byte(`"`+phProject+`"`), quoted)
		out = bytes.ReplaceAll(out, []byte(phProject), []byte(project))
		out = bytes.ReplaceAll(out, []byte(phHash), []byte(gemini.ProjectHash(project)))
		return os.WriteFile(p, out, filePerm)
	})
}

// fakeNPMInstall writes a package.json and a bin script the way npm lays
// out @google/gemini-cli, and returns the dir to put on PATH. The script is
// never run. On Windows the bin is gemini.cmd (PATHEXT), as npm's shim is.
// Disk · Write (tmp).
func fakeNPMInstall(prefix string) (string, error) {
	pkg := filepath.Join(prefix, "lib", "node_modules", "@google", "gemini-cli")
	bin := filepath.Join(pkg, "bundle")
	if err := os.MkdirAll(bin, dirPerm); err != nil {
		return "", err
	}
	manifest := []byte(`{"name":"` + gemini.Package + `","version":"` + installedVersion + `"}`)
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), manifest, filePerm); err != nil {
		return "", err
	}
	name := gemini.Binary
	if runtime.GOOS == agentio.OSWindows {
		name += ".cmd"
	}
	return bin, os.WriteFile(filepath.Join(bin, name), []byte("#!/usr/bin/env node\n"), execPerm)
}

// tidy replaces machine paths so the output carries none: the project
// becomes <project>, the user home ~, the temp dir <tmp>. Pure.
func (m machine) tidy(s string) string {
	return strings.NewReplacer(m.project, "<project>", m.host.UserHome, "~", m.tmp, "<tmp>").Replace(s)
}
