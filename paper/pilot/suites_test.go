package pilot

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sadayamuthu/agentchaos/internal/experiment"
)

func load(t *testing.T, name string) *experiment.File {
	t.Helper()
	f, err := experiment.Load(filepath.Join("suites", name))
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return f
}

func TestSuitesLoad(t *testing.T) {
	for name, want := range map[string]int{"sonnet.yaml": 8, "haiku.yaml": 8, "smoke.yaml": 2} {
		f := load(t, name)
		if len(f.Experiments) != want {
			t.Errorf("%s: %d experiments, want %d", name, len(f.Experiments), want)
		}
	}
}

func TestControlsMirrorTheirFaultTwins(t *testing.T) {
	for _, name := range []string{"sonnet.yaml", "haiku.yaml", "smoke.yaml"} {
		f := load(t, name)
		byName := map[string]experiment.Experiment{}
		for _, e := range f.Experiments {
			byName[e.Name] = e
		}
		controls := 0
		for _, e := range f.Experiments {
			base, ok := strings.CutSuffix(e.Name, " (control)")
			if !ok {
				continue
			}
			controls++
			twin, ok := byName[base]
			if !ok {
				t.Errorf("%s: control %q has no fault twin", name, e.Name)
				continue
			}
			if e.Fault.Type != "none" {
				t.Errorf("%s: %q fault type = %q, want none", name, e.Name, e.Fault.Type)
			}
			if e.Task != twin.Task {
				t.Errorf("%s: %q task differs from twin", name, e.Name)
			}
			if !reflect.DeepEqual(e.Expect, twin.Expect) {
				t.Errorf("%s: %q expect differs from twin", name, e.Name)
			}
		}
		if controls == 0 {
			t.Errorf("%s: no controls", name)
		}
	}
}
