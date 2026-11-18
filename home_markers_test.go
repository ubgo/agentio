package agentio_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// genericSpec is the toy agent with settings.json as a generic marker:
// other tools write a file of that name, so it marks a home only where the
// agent's own names select the folder.
func genericSpec() agentio.Spec {
	s := sampleSpec()
	s.HomeMarkers = []string{"config.json", "settings.json"}
	s.GenericHomeMarkers = []string{"settings.json"}
	return s
}

// TestIsHome_GenericMarkers: a generic marker alone makes a home of the
// default home, an extra-homes glob hit and the home variable's folder,
// never of a folder found elsewhere; a distinctive marker makes a home
// anywhere; HasHomeMarker counts every marker for a caller-named home.
func TestIsHome_GenericMarkers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	envDir := filepath.Join(root, "env-home")
	host := agenttest.NewHost(t, agenttest.WithEnv("TOY_HOME="+envDir))
	for _, rel := range []string{".toy", ".toy-work"} {
		agenttest.WriteFile(t, host.UserHome, rel+"/settings.json", "{}")
	}
	agenttest.WriteFile(t, envDir, "settings.json", "{}")
	agenttest.WriteFile(t, root, "found/settings.json", "{}")
	agenttest.WriteFile(t, root, "real/config.json", "{}")
	agenttest.WriteFile(t, root, "both/config.json", "{}")
	agenttest.WriteFile(t, root, "both/settings.json", "{}")
	agenttest.WriteFile(t, host.UserHome, "toy-nodot/settings.json", "{}")
	core, err := agentio.NewCore(genericSpec(), host)
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		dir          string
		isHome, have bool
	}{
		"default home, generic only":    {filepath.Join(host.UserHome, ".toy"), true, true},
		"extra-homes glob, generic":     {filepath.Join(host.UserHome, ".toy-work"), true, true},
		"home variable folder, generic": {envDir, true, true},
		"found elsewhere, generic only": {filepath.Join(root, "found"), false, true},
		"not the glob, generic only":    {filepath.Join(host.UserHome, "toy-nodot"), false, true},
		"found elsewhere, distinctive":  {filepath.Join(root, "real"), true, true},
		"found elsewhere, both":         {filepath.Join(root, "both"), true, true},
		"missing":                       {filepath.Join(root, "missing"), false, false},
	} {
		got, err := core.IsHome(tc.dir)
		if err != nil || got != tc.isHome {
			t.Errorf("%s: IsHome = %v, %v, want %v", name, got, err, tc.isHome)
		}
		have, err := core.HasHomeMarker(tc.dir)
		if err != nil || have != tc.have {
			t.Errorf("%s: HasHomeMarker = %v, %v, want %v", name, have, err, tc.have)
		}
	}
	homes, err := core.Homes()
	if err != nil || len(homes) != 3 {
		t.Errorf("Homes = %v, %v; want the default, the glob hit and the variable's folder", homes, err)
	}
	// The doctor checks a caller-named home: any marker will do.
	for _, c := range core.Checks(t.Context(), core.HomeAt(filepath.Join(root, "found"))) {
		if c.ID == agentio.CheckHome && c.Status != agentio.CheckOK {
			t.Errorf("doctor on a caller-named home with a generic marker: %+v", c)
		}
	}
}

// TestValidate_GenericMarkers: a generic marker must be one of
// HomeMarkers, and at least one marker must be distinctive, or no folder
// found elsewhere could ever be recognised.
func TestValidate_GenericMarkers(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		markers, generic []string
		ok               bool
	}{
		"subset":         {[]string{"a", "b"}, []string{"b"}, true},
		"none generic":   {[]string{"a"}, nil, true},
		"not a marker":   {[]string{"a"}, []string{"b"}, false},
		"all generic":    {[]string{"a", "b"}, []string{"a", "b"}, false},
		"without marker": {nil, []string{"a"}, false},
	} {
		s := sampleSpec()
		s.HomeMarkers, s.GenericHomeMarkers = tc.markers, tc.generic
		err := s.Validate()
		if (err == nil) != tc.ok || (err != nil && !errors.Is(err, agentio.ErrInvalidSpec)) {
			t.Errorf("%s: Validate = %v, want ok=%v", name, err, tc.ok)
		}
	}
}
