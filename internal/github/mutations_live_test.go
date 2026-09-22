//go:build live

package github

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestLiveMutationsContract(t *testing.T) {
	if os.Getenv("GH_PROJECTS_TUI_LIVE_MUTATIONS") != "1" {
		t.Skip("set GH_PROJECTS_TUI_LIVE_MUTATIONS=1 to run disposable-project mutation probes")
	}

	ownerLogin := liveMutationValue(t, "GH_PROJECTS_TUI_LIVE_MUTATION_OWNER")
	projectID := liveMutationValue(t, "GH_PROJECTS_TUI_LIVE_MUTATION_PROJECT_ID")
	projectNumber := liveMutationNumber(t, "GH_PROJECTS_TUI_LIVE_MUTATION_PROJECT")
	fieldID := liveMutationValue(t, "GH_PROJECTS_TUI_LIVE_MUTATION_FIELD_ID")
	optionID := liveMutationValue(t, "GH_PROJECTS_TUI_LIVE_MUTATION_OPTION_ID")
	alphaID := liveMutationValue(t, "GH_PROJECTS_TUI_LIVE_MUTATION_ALPHA_ID")
	betaID := liveMutationValue(t, "GH_PROJECTS_TUI_LIVE_MUTATION_BETA_ID")
	gammaID := liveMutationValue(t, "GH_PROJECTS_TUI_LIVE_MUTATION_GAMMA_ID")

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{Login: ownerLogin, Kind: UserOwner}
	ctx := context.Background()

	defer func() {
		// Leave the sandbox in a predictable state even if a later assertion fails.
		_ = client.ClearItemFieldValue(ctx, ItemFieldValueClear{ProjectID: projectID, ItemID: alphaID, FieldID: fieldID})
		_ = client.UpdateItemPosition(ctx, ItemPositionUpdate{ProjectID: projectID, ItemID: alphaID})
		_ = client.UpdateItemPosition(ctx, ItemPositionUpdate{ProjectID: projectID, ItemID: betaID, AfterID: &alphaID})
		_ = client.UpdateItemPosition(ctx, ItemPositionUpdate{ProjectID: projectID, ItemID: gammaID, AfterID: &betaID})
	}()

	if err := client.UpdateItemPosition(ctx, ItemPositionUpdate{ProjectID: projectID, ItemID: alphaID}); err != nil {
		t.Fatalf("normalize alpha to top: %v", err)
	}
	if err := client.UpdateItemPosition(ctx, ItemPositionUpdate{ProjectID: projectID, ItemID: betaID, AfterID: &alphaID}); err != nil {
		t.Fatalf("normalize beta after alpha: %v", err)
	}
	if err := client.UpdateItemPosition(ctx, ItemPositionUpdate{ProjectID: projectID, ItemID: gammaID, AfterID: &betaID}); err != nil {
		t.Fatalf("normalize gamma after beta: %v", err)
	}

	option := optionID
	if err := client.UpdateItemFieldValue(ctx, ItemFieldValueUpdate{
		ProjectID: projectID,
		ItemID:    alphaID,
		FieldID:   fieldID,
		Value:     FieldValueInput{SingleSelectOptionID: &option},
	}); err != nil {
		t.Fatalf("set status: %v", err)
	}
	page := liveMutationItems(t, client, ctx, owner, projectNumber)
	if value, ok := liveMutationField(page, alphaID, fieldID); !ok || value.OptionID != optionID || !value.Available {
		t.Fatalf("set status readback = %#v, found = %v", value, ok)
	}

	if err := client.ClearItemFieldValue(ctx, ItemFieldValueClear{ProjectID: projectID, ItemID: alphaID, FieldID: fieldID}); err != nil {
		t.Fatalf("clear status: %v", err)
	}
	page = liveMutationItems(t, client, ctx, owner, projectNumber)
	if _, ok := liveMutationField(page, alphaID, fieldID); ok {
		t.Fatalf("cleared status still returned: %#v", page.Items)
	}

	if err := client.UpdateItemPosition(ctx, ItemPositionUpdate{ProjectID: projectID, ItemID: gammaID, AfterID: &alphaID}); err != nil {
		t.Fatalf("move gamma after alpha: %v", err)
	}
	liveMutationOrder(t, client, ctx, owner, projectNumber, []string{alphaID, gammaID, betaID})

	if err := client.UpdateItemPosition(ctx, ItemPositionUpdate{ProjectID: projectID, ItemID: betaID}); err != nil {
		t.Fatalf("move beta to project top: %v", err)
	}
	liveMutationOrder(t, client, ctx, owner, projectNumber, []string{betaID, alphaID, gammaID})
}

func liveMutationValue(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func liveMutationNumber(t *testing.T, name string) int {
	t.Helper()
	value := liveMutationValue(t, name)
	var number int
	if _, err := fmt.Sscanf(value, "%d", &number); err != nil || number == 0 {
		t.Fatalf("%s must be a positive integer, got %q", name, value)
	}
	return number
}

func liveMutationItems(t *testing.T, client *Client, ctx context.Context, owner Owner, projectNumber int) ItemsPage {
	t.Helper()
	page, err := client.PageItems(ctx, owner, projectNumber, "", "")
	if err != nil {
		t.Fatalf("read sandbox items: %v", err)
	}
	return page
}

func liveMutationField(page ItemsPage, itemID, fieldID string) (FieldValue, bool) {
	for _, item := range page.Items {
		if item.ID != itemID {
			continue
		}
		for _, value := range item.FieldValues {
			if value.FieldID == fieldID {
				return value, true
			}
		}
	}
	return FieldValue{}, false
}

func liveMutationOrder(t *testing.T, client *Client, ctx context.Context, owner Owner, projectNumber int, want []string) {
	t.Helper()
	page := liveMutationItems(t, client, ctx, owner, projectNumber)
	got := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		got = append(got, item.ID)
	}
	if len(got) < len(want) {
		t.Fatalf("sandbox order = %#v, want prefix %#v", got, want)
	}
	if !reflect.DeepEqual(got[:len(want)], want) {
		t.Fatalf("sandbox order = %#v, want prefix %#v", got, want)
	}
}
