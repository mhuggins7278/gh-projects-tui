//go:build live

package github

import (
	"context"
	"os"
	"strconv"
	"testing"
)

func TestLiveReadContract(t *testing.T) {
	login := os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER")
	projectNumber, _ := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_PROJECT"))
	viewNumber, _ := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_VIEW"))
	if login == "" || projectNumber == 0 || viewNumber == 0 {
		t.Skip("set GH_PROJECTS_TUI_LIVE_OWNER, GH_PROJECTS_TUI_LIVE_PROJECT, and GH_PROJECTS_TUI_LIVE_VIEW")
	}
	ownerKind := OrganizationOwner
	if os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER_KIND") == "user" {
		ownerKind = UserOwner
	}

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{Login: login, Kind: ownerKind}
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.OpenView(context.Background(), owner, projectNumber, viewNumber); err != nil {
		t.Fatal(err)
	}
	items, err := client.PageItems(context.Background(), owner, projectNumber, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items.Items) > 0 {
		if _, err := client.LoadItemDetail(context.Background(), owner, projectNumber, items.Items[0].ID); err != nil {
			t.Fatal(err)
		}
	}
}
