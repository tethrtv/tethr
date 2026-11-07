package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/store"
)

func TestAutoLists(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	me := caller.ProfileID
	ago := func(days int) time.Time { return time.Now().AddDate(0, 0, -days) }
	darkKnight := Ref{Kind: "movie", IDs: IDs{TMDB: 155}}
	matrix := Ref{Kind: "movie", IDs: IDs{TMDB: 603}}
	breaking := Ref{Kind: "show", IDs: IDs{TMDB: 1396}}
	other := func(season, number int32) Ref {
		return Ref{Kind: "episode", Show: IDs{TMDB: 1399}, Season: &season, Number: &number}
	}
	if _, err := AddHistory(ctx, db, caller, []Watch{
		{Ref: episode(1, 1), At: ago(400)},
		{Ref: other(1, 1), At: ago(5)},
		{Ref: inception(), At: ago(730)},
		{Ref: matrix, At: ago(700)},
		{Ref: darkKnight, At: ago(30)},
	}); err != nil {
		t.Fatal(err)
	}
	outline := []OutlineEpisode{
		{TMDBID: 1, Season: 1, Number: 1, FirstAired: aired(900)},
		{TMDBID: 2, Season: 1, Number: 2, FirstAired: aired(890)},
	}
	var shows []int64
	for _, tmdb := range []int32{1396, 1399} {
		show, err := store.New(db).TitleByTMDB(ctx, store.TitleByTMDBParams{TmdbID: &tmdb, Kind: "show"})
		if err != nil {
			t.Fatal(err)
		}
		for i := range outline {
			outline[i].TMDBID = tmdb*10 + int32(i)
		}
		if err := SaveOutline(ctx, db, show.ID, outline); err != nil {
			t.Fatal(err)
		}
		shows = append(shows, show.ID)
	}
	if _, err := Rate(ctx, db, me, []Rating{
		{Ref: inception(), Rating: 10},
		{Ref: darkKnight, Rating: 9},
		{Ref: breaking, Rating: 8},
		{Ref: matrix, Rating: 7},
	}); err != nil {
		t.Fatal(err)
	}

	entries := func(profileID int64, kind string, query ListQuery) []AutoEntry {
		t.Helper()
		_, got, _, err := AutoListEntries(ctx, db, profileID, kind, query)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	ids := func(got []AutoEntry) []int32 {
		var out []int32
		for _, e := range got {
			out = append(out, *e.Item.Title.TmdbID)
		}
		return out
	}
	same := func(a, b []int32) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	lists, err := AutoLists(ctx, db, me)
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 3 || lists[0].Kind != "stopped" || lists[1].Count != 2 || len(lists[1].Cover) != 2 {
		t.Fatalf("auto lists on a one-person household = %+v", lists)
	}
	if got := ids(entries(me, "stopped", ListQuery{})); !same(got, []int32{1396}) {
		t.Errorf("stopped = %v", got)
	}
	if err := HideFromUpNext(ctx, db, me, shows[1]); err != nil {
		t.Fatal(err)
	}
	if got := ids(entries(me, "stopped", ListQuery{})); !same(got, []int32{1399, 1396}) {
		t.Errorf("stopped after hiding a show = %v", got)
	}
	favourites := entries(me, "favourites", ListQuery{})
	if !same(ids(favourites), []int32{27205, 155}) || *favourites[0].Rating != 10 || favourites[0].Progress.Watched != 1 {
		t.Errorf("favourites = %+v", favourites)
	}
	if got := ids(entries(me, "favourites", ListQuery{SortBy: "rank", SortHow: "desc"})); !same(got, []int32{155, 27205}) {
		t.Errorf("favourites reversed = %v", got)
	}
	if got := ids(entries(me, "rewatch", ListQuery{})); !same(got, []int32{27205, 1396}) {
		t.Errorf("due a rewatch = %v", got)
	}
	if got := ids(entries(me, "rewatch", ListQuery{Kind: "show"})); !same(got, []int32{1396}) {
		t.Errorf("shows due a rewatch = %v", got)
	}
	if _, _, _, err := AutoListEntries(ctx, db, me, "household", ListQuery{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("household list on a one-person household = %v", err)
	}
	if _, _, _, err := AutoListEntries(ctx, db, me, "nope", ListQuery{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown kind = %v", err)
	}

	sam, err := AddProfile(ctx, db, caller.AccountID, NewProfile{Name: "Sam", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}
	kid, err := AddProfile(ctx, db, caller.AccountID, NewProfile{Name: "Ollie", Kind: "kids"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddProfile(ctx, db, caller.AccountID, NewProfile{Name: "Visitor", Kind: "guest"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Rate(ctx, db, sam.ID, []Rating{{Ref: darkKnight, Rating: 10}, {Ref: matrix, Rating: 9}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Rate(ctx, db, kid.ID, []Rating{{Ref: Ref{Kind: "movie", IDs: IDs{TMDB: 862}}, Rating: 10}}); err != nil {
		t.Fatal(err)
	}
	household := entries(me, "household", ListQuery{})
	if got := ids(household); len(got) != 4 || got[0] != 155 || *household[0].Rating != 9 {
		t.Errorf("household favourites = %v", got)
	}
	if _, _, _, err := AutoListEntries(ctx, db, kid.ID, "household", ListQuery{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("household list for the only kids profile = %v", err)
	}

	copied, err := CopyAutoList(ctx, db, me, "favourites", "Favourites")
	if err != nil {
		t.Fatal(err)
	}
	if copied.Name != "Favourites" || copied.Privacy != "private" || copied.ItemCount != 2 || copied.Access != "mine" {
		t.Errorf("copied = %+v", copied)
	}
	if _, err := CopyAutoList(ctx, db, kid.ID, "household", "Household favourites"); !errors.Is(err, ErrNotFound) {
		t.Errorf("copying a list the profile doesn't have = %v", err)
	}
}
