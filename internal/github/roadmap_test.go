package github

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestOpenRoadmapPreservesMetadataWithoutInferringEndpoints(t *testing.T) {
	// Synthetic fixture matching the observed shape, not a stored project response.
	// Legacy display fields and project DATE definitions are not endpoint mappings.
	for _, kind := range []OwnerKind{UserOwner, OrganizationOwner} {
		t.Run(string(kind), func(t *testing.T) {
			branch := "organization"
			if kind == UserOwner {
				branch = "user"
			}
			graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
				func(query string, variables map[string]interface{}, response interface{}) error {
					if !strings.Contains(query, branch+"(login: $login)") || variables["project"] != 7 || variables["view"] != 3 {
						t.Fatalf("wrong owner or view selection")
					}
					return json.Unmarshal([]byte(fmt.Sprintf(`{"%s":{"projectV2":{
					  "id":"project", "viewerCanUpdate":true,
					  "projectFields":{"nodes":[
					    {"__typename":"ProjectV2Field","id":"date-a","name":"Date A","dataType":"DATE"},
					    {"__typename":"ProjectV2Field","id":"date-b","name":"Date B","dataType":"DATE"}
					  ],"pageInfo":{"hasNextPage":false}},
					  "view":{"id":"view","number":3,"name":"Timeline","layout":"ROADMAP_LAYOUT","filter":"",
					    "fields":{"nodes":[{"id":"title","name":"Title","dataType":"TITLE"}]},
					    "configuration":{"visibleFields":{"nodes":[],"pageInfo":{"hasNextPage":false}}},
					    "groupByFields":{"nodes":[],"pageInfo":{"hasNextPage":false}},
					    "verticalGroupByFields":{"nodes":[],"pageInfo":{"hasNextPage":false}},
					    "sortByFields":{"nodes":[],"pageInfo":{"hasNextPage":false}}
					  }
					}}}`, branch)), response)
				},
			}}
			view, err := newClient(graphql, &fakeREST{}).OpenView(context.Background(), Owner{Login: "owner", Kind: kind}, 7, 3)
			if err != nil {
				t.Fatal(err)
			}
			if view.Layout != RoadmapLayout || !view.ViewerCanUpdate || len(view.ProjectFields) != 2 {
				t.Fatalf("roadmap metadata was lost: %#v", view)
			}
			for _, field := range view.ProjectFields {
				if field.DataType != "DATE" {
					t.Fatalf("date definition was lost: %#v", field)
				}
			}
			if len(view.Fields) != 0 || len(view.GroupByFields) != 0 || len(view.VerticalGroupBy) != 0 || len(view.SortByFields) != 0 {
				t.Fatalf("display configuration was inferred: %#v", view)
			}
			if len(graphql.queries) != 1 {
				t.Fatalf("opening metadata made %d requests", len(graphql.queries))
			}
		})
	}
}

func TestRoadmapCandidateValuesPreserveDatesIterationsAndUnsetItems(t *testing.T) {
	// These invented values prove decoding only, not GitHub timeline placement.
	for _, kind := range []OwnerKind{UserOwner, OrganizationOwner} {
		t.Run(string(kind), func(t *testing.T) {
			branch := "organization"
			if kind == UserOwner {
				branch = "user"
			}
			graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
				func(query string, variables map[string]interface{}, response interface{}) error {
					if variables["filter"] != nil || variables["after"] != nil || !strings.Contains(query, "orderBy: {field: POSITION, direction: ASC}") {
						t.Fatal("candidate baseline must be unfiltered and position ordered")
					}
					return json.Unmarshal([]byte(fmt.Sprintf(`{"%s":{"projectV2":{"items":{
					  "nodes":[
					    {"id":"dated","type":"ISSUE","fieldValues":{
					      "nodes":[{"__typename":"ProjectV2ItemFieldDateValue","field":{"id":"date-a","name":"Date A"},"date":"2026-09-01"}],
					      "pageInfo":{"hasNextPage":true,"endCursor":"values-next"}
					    }},
					    {"id":"undated","type":"DRAFT_ISSUE","fieldValues":{"nodes":[],"pageInfo":{"hasNextPage":false}}}
					  ],"pageInfo":{"hasNextPage":true,"endCursor":"items-next"}
					}}}}`, branch)), response)
				},
				func(query string, variables map[string]interface{}, response interface{}) error {
					if variables["id"] != "dated" || variables["after"] != "values-next" || !strings.Contains(query, "ProjectItemFieldValues") {
						t.Fatal("date value connection was not paginated")
					}
					return json.Unmarshal([]byte(`{"node":{"fieldValues":{
					  "nodes":[
					    {"__typename":"ProjectV2ItemFieldDateValue","field":{"id":"date-b","name":"Date B"},"date":"2026-09-14"},
					    {"__typename":"ProjectV2ItemFieldIterationValue","field":{"id":"sprint","name":"Sprint"},"iterationId":"iteration","title":"Sprint A"}
					  ],"pageInfo":{"hasNextPage":false}
					}}}`), response)
				},
				func(_ string, variables map[string]interface{}, response interface{}) error {
					if variables["filter"] != nil || variables["after"] != "items-next" {
						t.Fatal("item baseline was not paginated unchanged")
					}
					return json.Unmarshal([]byte(fmt.Sprintf(`{"%s":{"projectV2":{"items":{
					  "nodes":[{"id":"partial","type":"PULL_REQUEST","fieldValues":{
					    "nodes":[{"__typename":"ProjectV2ItemFieldDateValue","field":{"id":"date-b","name":"Date B"},"date":"2026-10-01"}],
					    "pageInfo":{"hasNextPage":false}
					  }}],"pageInfo":{"hasNextPage":false}
					}}}}`, branch)), response)
				},
			}}
			client := newClient(graphql, &fakeREST{})
			owner := Owner{Login: "owner", Kind: kind}
			first, err := client.PageItems(context.Background(), owner, 7, "", "")
			if err != nil {
				t.Fatal(err)
			}
			want := []FieldValue{
				{Kind: "ProjectV2ItemFieldDateValue", FieldID: "date-a", FieldName: "Date A", Value: "2026-09-01", Available: true},
				{Kind: "ProjectV2ItemFieldDateValue", FieldID: "date-b", FieldName: "Date B", Value: "2026-09-14", Available: true},
				{Kind: "ProjectV2ItemFieldIterationValue", FieldID: "sprint", FieldName: "Sprint", IterationID: "iteration", Value: "Sprint A", Available: true},
			}
			if len(first.Items) != 2 || !first.HasNext || !reflect.DeepEqual(first.Items[0].FieldValues, want) || len(first.Items[1].FieldValues) != 0 {
				t.Fatalf("date/iteration/unset decoding = %#v", first)
			}
			second, err := client.PageItems(context.Background(), owner, 7, "", first.EndCursor)
			if err != nil {
				t.Fatal(err)
			}
			if second.HasNext || len(second.Items) != 1 || len(second.Items[0].FieldValues) != 1 || second.Items[0].FieldValues[0].FieldID != "date-b" || second.Items[0].FieldValues[0].Value != "2026-10-01" {
				t.Fatalf("one-date item acquired inferred values: %#v", second)
			}
			if len(graphql.queries) != 3 {
				t.Fatalf("baseline used %d requests, want three read queries", len(graphql.queries))
			}
		})
	}
}

func TestOpenViewHydratesNonVisibleIterationDefinitionsAcrossPages(t *testing.T) {
	for _, kind := range []OwnerKind{UserOwner, OrganizationOwner} {
		branch := "user"
		if kind == OrganizationOwner {
			branch = "organization"
		}
		graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
			func(query string, variables map[string]interface{}, response interface{}) error {
				if !strings.Contains(query, "completedIterations") {
					t.Fatal("endpoint iteration metadata not requested")
				}
				return json.Unmarshal([]byte(fmt.Sprintf(`{"%s":{"projectV2":{"id":"project","projectFields":{"nodes":[{"id":"date","dataType":"DATE"}],"pageInfo":{"hasNextPage":true,"endCursor":"fields-next"}},"view":{"id":"view","number":3,"layout":"ROADMAP_LAYOUT","filter":"","configuration":{"visibleFields":{"nodes":[]}},"groupByFields":{"nodes":[]},"verticalGroupByFields":{"nodes":[]},"sortByFields":{"nodes":[]}}}}}`, branch)), response)
			},
			func(query string, variables map[string]interface{}, response interface{}) error {
				if variables["after"] != "fields-next" || !strings.Contains(query, "completedIterations") {
					t.Fatal("iteration definitions missing on later project page")
				}
				return json.Unmarshal([]byte(fmt.Sprintf(`{"%s":{"projectV2":{"fields":{"nodes":[{"__typename":"ProjectV2IterationField","id":"sprint","name":"Sprint","dataType":"ITERATION","configuration":{"iterations":[{"id":"current","title":"Current","startDate":"2026-10-01","duration":7}],"completedIterations":[{"id":"past","title":"Past","startDate":"2026-09-24","duration":7}]}}],"pageInfo":{"hasNextPage":false}}}}}`, branch)), response)
			},
		}}
		view, err := newClient(graphql, &fakeREST{}).OpenView(context.Background(), Owner{Login: "owner", Kind: kind}, 7, 3)
		if err != nil {
			t.Fatal(err)
		}
		fields, err := ResolveRoadmapFields(view.ProjectFields, "sprint", "date")
		if err != nil || len(fields.Start.Iterations) != 2 || !fields.Start.Iterations[1].Completed || len(view.Fields) != 0 {
			t.Fatalf("non-visible definition unavailable: %+v, %v", fields, err)
		}
	}
}
