package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/codex"
)

// showAgent covers docs/codex/agent.md.
func showAgent(w io.Writer, m machine) error {
	if err := createAgent(w, m.host); err != nil {
		return err
	}
	if err := pickHome(w, m); err != nil {
		return err
	}
	neutralView(w, m.host)
	return nil
}

// snippet:start create-agent
// createAgent builds the Codex agent and reads its static facts. New reads
// nothing; InstallationID is one small file read.
func createAgent(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	spec := a.Spec()
	fmt.Fprintf(w, "id=%s name=%s binary=%s home-env=%s default-home=%s\n",
		a.ID(), a.Name(), spec.Binary, spec.HomeEnv, spec.DefaultHome)

	h := a.DefaultHome()
	env := a.HomeEnv([]string{"PATH=/usr/bin", codex.EnvHome + "=/somewhere"}, h)
	_, set := agentio.EnvLookup(env, codex.EnvHome)
	fmt.Fprintf(w, "default home keeps CODEX_HOME in the child env: %v\n", set)

	id, err := a.InstallationID(h)
	if err != nil {
		return err
	}
	// InstallationID is Personal: printing it shows a redacted form.
	fmt.Fprintf(w, "installation id: %v\n", id)
	return nil
}

// snippet:end create-agent

// snippet:start pick-home
// pickHome shows every way to get a codex.Home: the default, the homes
// on disk, the one CODEX_HOME names, a caller-named directory, and the
// conversion from a neutral agentio.Home.
func pickHome(w io.Writer, m machine) error {
	a := codex.New(m.host)
	def := a.DefaultHome()
	fmt.Fprintf(w, "default: %s (default=%v)\n", m.rel(def.Dir()), def.IsDefault())

	homes, err := a.Homes()
	if err != nil {
		return err
	}
	for _, h := range homes {
		fmt.Fprintf(w, "found home %q at %s\n", h.Name(), m.rel(h.Dir()))
	}

	_, fromEnv := a.HomeFromEnv()
	fmt.Fprintf(w, "CODEX_HOME set: %v\n", fromEnv)

	ok, err := a.IsHome(def.Dir())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "is a home: %v\n", ok)

	if _, err := a.HomeAt(m.project); err != nil {
		fmt.Fprintf(w, "project dir is not a home: %v\n", errors.Is(err, agentio.ErrNotFound))
	}

	back, err := a.HomeOf(def.Neutral())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "neutral round trip: %v\n", back.Dir() == def.Dir())
	return nil
}

// snippet:end pick-home

// snippet:start neutral
// neutralView is the agentio.Agent view of Codex, for code that loops
// over agents: capabilities are reached through typed accessors.
func neutralView(w io.Writer, host agentio.Host) {
	n := codex.New(host).Neutral()
	fmt.Fprintf(w, "neutral id=%s capabilities=%d\n", n.ID(), len(agentio.Supported(n)))
	if reason, declared := agentio.Unsupported(n, agentio.CapConversationMover); declared {
		fmt.Fprintf(w, "conversation mover unsupported: %s\n", reason)
	}
	if d, ok := agentio.DoctorOf(n); ok {
		fmt.Fprintf(w, "doctor checks via the neutral view: %d\n", len(d.Checks(context.Background(), n.DefaultHome())))
	}
}

// snippet:end neutral
