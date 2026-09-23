package github

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func TestUpdateItemFieldValueBuildsSingleSelectInput(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "updateProjectV2ItemFieldValue") {
				t.Fatalf("mutation query = %q", query)
			}
			input := variables["input"].(map[string]interface{})
			if input["projectId"] != "project" || input["itemId"] != "item" || input["fieldId"] != "status" {
				t.Fatalf("mutation input = %#v", input)
			}
			if !reflect.DeepEqual(input["value"], map[string]interface{}{"singleSelectOptionId": "progress"}) {
				t.Fatalf("field value = %#v", input["value"])
			}
			result := response.(*updateItemFieldValueResponse)
			result.Update = &struct {
				ProjectV2Item *struct {
					ID string `json:"id"`
				} `json:"projectV2Item"`
			}{ProjectV2Item: &struct {
				ID string `json:"id"`
			}{ID: "item"}}
			return nil
		},
	}}

	option := "progress"
	err := newClient(graphql, &fakeREST{}).UpdateItemFieldValue(context.Background(), ItemFieldValueUpdate{
		ProjectID: "project",
		ItemID:    "item",
		FieldID:   "status",
		Value:     FieldValueInput{SingleSelectOptionID: &option},
	})
	if err != nil {
		t.Fatalf("UpdateItemFieldValue() error = %v", err)
	}
}

func TestClearItemFieldValueOmitsValue(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "clearProjectV2ItemFieldValue") {
				t.Fatalf("mutation query = %q", query)
			}
			input := variables["input"].(map[string]interface{})
			if _, ok := input["value"]; ok {
				t.Fatalf("clear input unexpectedly contains value: %#v", input)
			}
			result := response.(*clearItemFieldValueResponse)
			result.Clear = &struct {
				ProjectV2Item *struct {
					ID string `json:"id"`
				} `json:"projectV2Item"`
			}{ProjectV2Item: &struct {
				ID string `json:"id"`
			}{ID: "item"}}
			return nil
		},
	}}

	err := newClient(graphql, &fakeREST{}).ClearItemFieldValue(context.Background(), ItemFieldValueClear{
		ProjectID: "project",
		ItemID:    "item",
		FieldID:   "status",
	})
	if err != nil {
		t.Fatalf("ClearItemFieldValue() error = %v", err)
	}
}

func TestUpdateItemPositionSupportsTopAndAnchor(t *testing.T) {
	cases := []struct {
		name    string
		afterID *string
		check   func(t *testing.T, input map[string]interface{})
	}{
		{
			name: "top",
			check: func(t *testing.T, input map[string]interface{}) {
				if _, ok := input["afterId"]; ok {
					t.Fatalf("top input contains afterId: %#v", input)
				}
			},
		},
		{
			name:    "anchor",
			afterID: stringPtr("anchor"),
			check: func(t *testing.T, input map[string]interface{}) {
				if input["afterId"] != "anchor" {
					t.Fatalf("anchored input = %#v", input)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
				func(query string, variables map[string]interface{}, response interface{}) error {
					if !strings.Contains(query, "updateProjectV2ItemPosition") {
						t.Fatalf("mutation query = %q", query)
					}
					input := variables["input"].(map[string]interface{})
					tc.check(t, input)
					result := response.(*updateItemPositionResponse)
					result.Update = &struct {
						ClientMutationID *string `json:"clientMutationId"`
					}{}
					return nil
				},
			}}

			err := newClient(graphql, &fakeREST{}).UpdateItemPosition(context.Background(), ItemPositionUpdate{
				ProjectID: "project",
				ItemID:    "item",
				AfterID:   tc.afterID,
			})
			if err != nil {
				t.Fatalf("UpdateItemPosition() error = %v", err)
			}
		})
	}
}

func TestFieldValueInputRequiresExactlyOneVariant(t *testing.T) {
	if _, err := (FieldValueInput{}).input(); err == nil {
		t.Fatal("empty field value was accepted")
	}
	text := "text"
	number := 1.0
	if _, err := (FieldValueInput{Text: &text, Number: &number}).input(); err == nil {
		t.Fatal("multiple field values were accepted")
	}
}

func TestMutationRejectsMissingPayload(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, _ map[string]interface{}, _ interface{}) error { return nil },
		func(_ string, _ map[string]interface{}, _ interface{}) error { return nil },
		func(_ string, _ map[string]interface{}, _ interface{}) error { return nil },
	}}
	client := newClient(graphql, &fakeREST{})
	option := "progress"
	if err := client.UpdateItemFieldValue(context.Background(), ItemFieldValueUpdate{ProjectID: "p", ItemID: "i", FieldID: "f", Value: FieldValueInput{SingleSelectOptionID: &option}}); err == nil {
		t.Fatal("missing field-value payload was accepted")
	} else if !IsAmbiguousMutationError(err) {
		t.Fatalf("missing field-value payload was not ambiguous: %v", err)
	}
	if err := client.ClearItemFieldValue(context.Background(), ItemFieldValueClear{ProjectID: "p", ItemID: "i", FieldID: "f"}); err == nil {
		t.Fatal("missing clear payload was accepted")
	} else if !IsAmbiguousMutationError(err) {
		t.Fatalf("missing clear payload was not ambiguous: %v", err)
	}
	if err := client.UpdateItemPosition(context.Background(), ItemPositionUpdate{ProjectID: "p", ItemID: "i"}); err == nil {
		t.Fatal("missing position payload was accepted")
	} else if !IsAmbiguousMutationError(err) {
		t.Fatalf("missing position payload was not ambiguous: %v", err)
	}
}

func TestMutationPropagatesGraphQLError(t *testing.T) {
	failure := &api.GraphQLError{Errors: []api.GraphQLErrorItem{{Message: "permission denied"}}}
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, _ map[string]interface{}, _ interface{}) error { return failure },
	}}
	option := "progress"
	err := newClient(graphql, &fakeREST{}).UpdateItemFieldValue(context.Background(), ItemFieldValueUpdate{ProjectID: "p", ItemID: "i", FieldID: "f", Value: FieldValueInput{SingleSelectOptionID: &option}})
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want %v", err, failure)
	}
	if IsAmbiguousMutationError(err) {
		t.Fatalf("GraphQL rejection was treated as ambiguous: %v", err)
	}
}

func TestMutationErrorClassifiesTransportAndServerOutcomes(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		ambiguous bool
	}{
		{name: "graphql rejection", err: &api.GraphQLError{Errors: []api.GraphQLErrorItem{{Message: "permission denied"}}}},
		{name: "client http rejection", err: &api.HTTPError{StatusCode: 422}},
		{name: "server http error", err: &api.HTTPError{StatusCode: 502}, ambiguous: true},
		{name: "transport timeout", err: context.DeadlineExceeded, ambiguous: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyMutationError(tc.err)
			if got := IsAmbiguousMutationError(err); got != tc.ambiguous {
				t.Fatalf("ambiguous = %v, want %v (err %v)", got, tc.ambiguous, err)
			}
		})
	}
}

func stringPtr(value string) *string {
	return &value
}
