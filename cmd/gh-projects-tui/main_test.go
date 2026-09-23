package main

import "testing"

func TestValidateFlagsRequiresExplicitAncestors(t *testing.T) {
	tests := []struct {
		name        string
		owner       string
		project     int
		view        int
		wantErrText string
	}{
		{name: "picker startup"},
		{name: "owner picker", owner: "org"},
		{name: "project selection", owner: "org", project: 7},
		{name: "specific view", owner: "org", project: 7, view: 3},
		{name: "project requires owner", project: 7, wantErrText: "--project requires --owner"},
		{name: "view requires project", owner: "org", view: 3, wantErrText: "--view requires --project"},
		{name: "negative value", owner: "org", project: -1, wantErrText: "--project and --view must be positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateFlags(test.owner, test.project, test.view)
			if test.wantErrText == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || err.Error() != test.wantErrText {
				t.Fatalf("error = %v, want %q", err, test.wantErrText)
			}
		})
	}
}
