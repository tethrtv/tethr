package traktcompat

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestLists(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	c, app := signIn(t, db)
	var profile int64
	if err := db.QueryRow(ctx, "SELECT id FROM profiles").Scan(&profile); err != nil {
		t.Fatal(err)
	}
	name := func(s string) *string { return &s }

	family, err := core.CreateList(ctx, db, profile, core.ListChanges{Name: name("Family"), Privacy: name("household")})
	if err != nil {
		t.Fatal(err)
	}
	public, err := core.CreateList(ctx, db, profile, core.ListChanges{Name: name("Best of 2010"), Privacy: name("public"), SortBy: name("year")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.AddToList(ctx, db, profile, public.ID, []core.Listing{
		{Ref: core.Ref{Kind: "movie", IDs: core.IDs{TMDB: 27205}}, Notes: "Twice"},
		{Ref: core.Ref{Kind: "show", IDs: core.IDs{TMDB: 1396}}},
	}); err != nil {
		t.Fatal(err)
	}

	mine := c.list("GET", "/users/me/lists", nil)
	if len(mine) != 2 || mine[0]["name"] != "Best of 2010" || mine[1]["privacy"] != "friends" || mine[0]["sort_by"] != "released" || mine[0]["item_count"] != 2.0 {
		t.Errorf("my lists = %v", mine)
	}
	if mine[0]["share_link"] != "https://tethr.tv/@nathan/lists/best-of-2010" {
		t.Errorf("share link = %v", mine[0]["share_link"])
	}

	anyone := newClient(t, db, app.ClientID)
	theirs := anyone.list("GET", "/users/nathan/lists", nil)
	if len(theirs) != 1 || theirs[0]["ids"].(map[string]any)["trakt"] != float64(public.ID+listOffset) {
		t.Errorf("someone else's view = %v", theirs)
	}
	if rec := anyone.do("GET", "/users/me/lists", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("me without a token = %d", rec.Code)
	}
	if rec := anyone.do("GET", "/users/nathan/lists/family/items", nil); rec.Code != http.StatusNotFound {
		t.Errorf("a household list signed out = %d %s", rec.Code, rec.Body)
	}
	if rec := anyone.do("GET", "/users/nobody/lists", nil); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown user = %d", rec.Code)
	}

	items := func(cl *client, path string) []map[string]any {
		t.Helper()
		rec := cl.do("GET", path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body)
		}
		var out []map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	bySlug := items(anyone, "/users/nathan/lists/best-of-2010/items")
	if len(bySlug) != 2 || bySlug[0]["type"] != "movie" || bySlug[0]["notes"] != "Twice" || bySlug[0]["movie"].(map[string]any)["ids"].(map[string]any)["tmdb"] != 27205.0 {
		t.Errorf("items by slug = %v", bySlug)
	}
	byID := items(anyone, "/lists/"+strconv.FormatInt(public.ID+listOffset, 10)+"/items/shows")
	if len(byID) != 1 || byID[0]["type"] != "show" {
		t.Errorf("shows by list ID = %v", byID)
	}
	if got := items(c, "/users/me/lists/"+strconv.FormatInt(family.ID+listOffset, 10)+"/items"); len(got) != 0 {
		t.Errorf("an empty list = %v", got)
	}
	if rec := anyone.do("GET", "/lists/"+strconv.FormatInt(family.ID+listOffset, 10), nil); rec.Code != http.StatusNotFound {
		t.Errorf("a household list by ID signed out = %d", rec.Code)
	}

	settings := c.valid("GET", "/users/settings", nil, http.StatusOK)
	if settings["limits"].(map[string]any)["list"].(map[string]any)["count"] != 100.0 {
		t.Errorf("limits = %v", settings["limits"])
	}
}
