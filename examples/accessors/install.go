package main

import (
	"context"
	"fmt"
	"io"

	"github.com/ubgo/agentio"
)

// snippet:start install
// printInstalls reads what is on disk for each agent's binary: the PATH hit,
// the real path behind the symlink, the install method and the version from
// package metadata. Disk · Read; the binary is never run.
func printInstalls(w io.Writer, h agentio.Host, agents []agentio.Agent) {
	for _, a := range agents {
		in, ok := agentio.InstallerOf(a)
		if !ok {
			continue
		}
		inst, err := in.Install()
		if err != nil {
			// ErrNotInstalled is an answer, not a failure of the loop.
			fmt.Fprintf(w, "%s: %v\n", a.ID(), err)
			continue
		}
		fmt.Fprintf(w, "%s: %s -> %s method=%s version=%s\n",
			a.ID(), tidy(h, inst.Path), tidy(h, inst.RealPath), inst.Method, inst.Version)
	}
}

// snippet:end install

// snippet:start binaryversion
// printBinaryVersions runs `<binary> --version` through Host.Exec. This is
// a Process call that may touch the login, so it has its own accessor and a
// caller reaches it only on purpose.
func printBinaryVersions(ctx context.Context, w io.Writer, agents []agentio.Agent) {
	for _, a := range agents {
		bv, ok := agentio.BinaryVersionOf(a)
		if !ok {
			continue
		}
		v, err := bv.BinaryVersion(ctx)
		if err != nil {
			fmt.Fprintf(w, "%s: %v\n", a.ID(), err)
			continue
		}
		fmt.Fprintf(w, "%s: %s\n", a.ID(), v)
	}
}

// snippet:end binaryversion

// snippet:start identity
// printIdentities reads each home's login without the network. Email and
// Org are agentio.Personal and print redacted; Expired compares the stored
// expiry with the Host clock.
func printIdentities(w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		ir, ok := agentio.IdentityOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			id, err := ir.Identity(home)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "%s %s: logged-in=%t email=%s plan=%q billing=%s credential=%s expired=%t\n",
				a.ID(), home.Name(), id.LoggedIn, id.Email, id.PlanLabel, id.Billing, id.Credential, id.Expired(h.Now()))
		}
	}
	return nil
}

// snippet:end identity
