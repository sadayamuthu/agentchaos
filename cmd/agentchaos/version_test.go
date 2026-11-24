package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	info := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/sadayamuthu/agentchaos", Version: v}}
	}
	cases := []struct {
		name    string
		ldflags string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{"ldflags wins", "v9.9.9", info("v0.1.1"), true, "v9.9.9"},
		{"go install module version", "", info("v0.1.1"), true, "v0.1.1"},
		{"local build is devel", "", info("(devel)"), true, "dev"},
		{"empty module version", "", info(""), true, "dev"},
		{"no build info", "", nil, false, "dev"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveVersion(c.ldflags, c.info, c.ok); got != c.want {
				t.Fatalf("resolveVersion = %q, want %q", got, c.want)
			}
		})
	}
}
