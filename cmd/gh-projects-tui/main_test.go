package main

import (
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/config"
)

func TestStartupSelectionLeavesNoFlagsOnDiscoveryPath(t *testing.T) {
	remembered := config.Selection{Owner: "stale", Project: 7, View: 3}
	if got := startupSelection(config.Selection{}, remembered); got != (config.Selection{}) {
		t.Fatalf("no-flag startup selection = %#v", got)
	}
}

func TestStartupSelectionResolvesExplicitFlags(t *testing.T) {
	remembered := config.Selection{Owner: "org", Project: 7, View: 3}
	cases := []struct {
		name     string
		explicit config.Selection
		want     config.Selection
	}{
		{name: "owner", explicit: config.Selection{Owner: "new"}, want: config.Selection{Owner: "new"}},
		{name: "project", explicit: config.Selection{Project: 9}, want: config.Selection{Owner: "org", Project: 9}},
		{name: "view", explicit: config.Selection{View: 4}, want: config.Selection{Owner: "org", Project: 7, View: 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := startupSelection(tc.explicit, remembered); got != tc.want {
				t.Fatalf("startup selection = %#v, want %#v", got, tc.want)
			}
		})
	}
}
