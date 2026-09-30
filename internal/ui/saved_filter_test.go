package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestSplitSavedFilterTermsPreservesQuotedValues(t *testing.T) {
	tests := []struct {
		filter string
		terms  []string
	}{
		{filter: " \t\n", terms: []string{}},
		{filter: "\tlabel:bug\n is:open  ", terms: []string{"label:bug", "is:open"}},
		{filter: `reason:completed,"not planned" is:issue`, terms: []string{`reason:completed,"not planned"`, "is:issue"}},
		{filter: `title:"Update environment, enhance linting"`, terms: []string{`title:"Update environment, enhance linting"`}},
		{filter: `title:"quote\" and space" is:open`, terms: []string{`title:"quote\" and space"`, "is:open"}},
	}
	for _, test := range tests {
		t.Run(test.filter, func(t *testing.T) {
			terms, err := splitSavedFilterTerms(test.filter)
			if err != nil || !reflect.DeepEqual(terms, test.terms) {
				t.Fatalf("terms = %#v, error = %v; want %#v", terms, err, test.terms)
			}
		})
	}
}

func TestSplitFilterValuesPreservesQuotedCommasAndEscapes(t *testing.T) {
	tests := []struct {
		value string
		parts []string
	}{
		{value: `completed,"not planned"`, parts: []string{"completed", `"not planned"`}},
		{value: `"comma, inside"`, parts: []string{`"comma, inside"`}},
		{value: `"quote\"value",other`, parts: []string{`"quote\"value"`, "other"}},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			parts, err := splitFilterValues(test.value)
			if err != nil || !reflect.DeepEqual(parts, test.parts) {
				t.Fatalf("parts = %#v, error = %v; want %#v", parts, err, test.parts)
			}
		})
	}
	for _, value := range []string{"", ",bug", "bug,", "bug,,support", `"unfinished`, `"unfinished\`} {
		if _, err := splitFilterValues(value); err == nil {
			t.Errorf("malformed value %q was accepted", value)
		}
	}
}

func TestSavedFilterGatedFormsHaveActionableReasons(t *testing.T) {
	tests := []struct {
		filter string
		hint   string
	}{
		{filter: "reviewers:@me", hint: "unquoted usernames"},
		{filter: "reason:reopened", hint: "matching live probes"},
		{filter: "parent-issue:octocat/game#1,octocat/game#2", hint: "multiple references"},
		{filter: "-has:reviewers", hint: "no:FIELD"},
		{filter: `label:"quote\"value"`, hint: "without escapes"},
		{filter: "-sprint:@current", hint: "negated iteration"},
		{filter: "-iteration:@current", hint: "negated iteration"},
		{filter: `title:"quote\"value"`, hint: "without escapes"},
		{filter: "note:hello", hint: "choose a saved view"},
	}
	fields := []github.Field{{Name: "Sprint", DataType: "ITERATION"}, {Name: "Note", DataType: "TEXT"}}
	for _, test := range tests {
		t.Run(test.filter, func(t *testing.T) {
			err := validateSavedFilterWithFields(test.filter, fields)
			if err == nil || !strings.Contains(err.Error(), test.hint) {
				t.Fatalf("error = %v; want hint %q", err, test.hint)
			}
		})
	}
}

func TestSavedFilterRejectsUnverifiedOrAmbiguousProjectFields(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		fields []github.Field
	}{
		{name: "date", filter: "target:2026-09-29", fields: []github.Field{{Name: "Target", DataType: "DATE"}}},
		{name: "text", filter: `note:"hello world"`, fields: []github.Field{{Name: "Note", DataType: "TEXT"}}},
		{name: "multi select", filter: "teams:Core", fields: []github.Field{{Name: "Teams", DataType: "MULTI_SELECT"}}},
		{name: "milestone", filter: `milestone:"QA release"`, fields: []github.Field{{Name: "Milestone", DataType: "MILESTONE"}}},
		{name: "punctuation", filter: "estimate-days:2", fields: []github.Field{{Name: "Estimate (days)", DataType: "NUMBER"}}},
		{name: "collision", filter: "phase:Todo", fields: []github.Field{{Name: "Phase", DataType: "SINGLE_SELECT"}, {Name: "phase", DataType: "SINGLE_SELECT"}}},
		{name: "presence collision", filter: "has:phase", fields: []github.Field{{Name: "Phase", DataType: "SINGLE_SELECT"}, {Name: "phase", DataType: "SINGLE_SELECT"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateSavedFilterWithFields(test.filter, test.fields); err == nil {
				t.Fatalf("unverified or ambiguous field accepted: %q", test.filter)
			}
		})
	}
}

func TestSavedFilterRejectsUnquotedGroupingSyntax(t *testing.T) {
	fields := []github.Field{{Name: "Phase", DataType: "SINGLE_SELECT"}}
	for _, filter := range []string{
		"(", ")", "(bug)", "bug (fix)", "(is:open)",
		"status:Todo)", "status:(Todo)", "phase:Todo)",
		"label:bug)", "title:(fix)", "title:*(fix)*",
	} {
		if err := validateSavedFilterWithFields(filter, fields); err == nil {
			t.Errorf("unverified grouping syntax accepted: %q", filter)
		}
	}
	// Literal punctuation in quoted values is not Boolean grouping.
	for _, filter := range []string{`status:"Todo (later)"`, `phase:"Phase (next)"`, `title:"Fix (bug)"`, `label:"bug (urgent)"`} {
		if err := validateSavedFilterWithFields(filter, fields); err != nil {
			t.Errorf("quoted literal rejected: %q: %v", filter, err)
		}
	}
}

func FuzzSavedFilterParser(f *testing.F) {
	for _, filter := range []string{
		"", `reason:completed,"not planned"`, `title:"A title, with commas"`,
		`parent-issue:"octocat/game#12" is:open`, "label:*bug*", "closed:@today-4d",
		`status:"quote\"value"`, "status:Todo OR is:open", "points:1..3", "sprint:@next",
		"reviewers:octocat,stevecat", "reviewers:@me", "-parent-issue:octocat/game#12",
		"has:closed", "-no:parent-issue", "reason:reopened",
	} {
		f.Add(filter)
	}
	fields := []github.Field{{Name: "Points", DataType: "NUMBER"}, {Name: "Sprint", DataType: "ITERATION"}}
	f.Fuzz(func(t *testing.T, filter string) {
		if err := validateSavedFilterWithFields(filter, fields); err != nil {
			return
		}
		terms, err := splitSavedFilterTerms(filter)
		if err != nil {
			t.Fatal(err)
		}
		for _, term := range terms {
			if strings.EqualFold(term, "AND") || strings.EqualFold(term, "OR") {
				t.Fatalf("Boolean operator accepted as a term: %q", term)
			}
			if _, values, ok := strings.Cut(term, ":"); ok {
				parts, err := splitFilterValues(values)
				if err != nil || strings.Join(parts, ",") != values {
					t.Fatalf("value split changed accepted term %q: %#v, %v", term, parts, err)
				}
			}
		}
		if err := validateSavedFilterWithFields(strings.Join(terms, " "), fields); err != nil {
			t.Fatalf("normalizing separators changed validity: %v", err)
		}
	})
}
