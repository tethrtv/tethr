package core

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestTitleState(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	if _, err := AddHistory(ctx, db, caller, []Watch{
		{Ref: episode(1, 1), At: *aired(5)},
		{Ref: episode(1, 1), At: *aired(2)},
		{Ref: inception(), At: *aired(3)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ScrobblePause(ctx, db, caller, Scrobble{Ref: episode(1, 2), Progress: 40}); err != nil {
		t.Fatal(err)
	}
	if _, err := Rate(ctx, db, profile, []Rating{{Ref: Ref{Kind: "show", IDs: IDs{TMDB: 1396}}, Rating: 9}, {Ref: episode(1, 1), Rating: 7}}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddToWatchlist(ctx, db, profile, []Listing{{Ref: inception()}}); err != nil {
		t.Fatal(err)
	}
	items, err := LookUp(ctx, db, "show", IDs{TMDB: 1396}, false)
	if err != nil {
		t.Fatal(err)
	}
	show := items[0].Title.ID
	if err := SaveOutline(ctx, db, show, []OutlineEpisode{
		{TMDBID: 900, Season: 0, Number: 1, FirstAired: aired(400)},
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: aired(300)},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: aired(293)},
		{TMDBID: 201, Season: 2, Number: 1, FirstAired: aired(-7)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWikidata(ctx, db, show, "show", WikidataTitle{QID: "Q1079", Name: "Breaking Bad", Year: 2008, TMDB: 1396}); err != nil {
		t.Fatal(err)
	}

	s, err := Title(ctx, db, profile, "breaking-bad-2008", "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Item.Title.ID != show || s.Rating == nil || s.Rating.Rating != 9 || s.Listing != nil || len(s.Watches) != 0 {
		t.Fatalf("show = %+v", s)
	}
	var seasons []int32
	for _, season := range s.Seasons {
		seasons = append(seasons, season.Number)
		if season.ID == 0 {
			t.Errorf("season %d should have an ID once the outline has it", season.Number)
		}
	}
	if len(seasons) != 3 || seasons[0] != 1 || seasons[1] != 2 || seasons[2] != 0 {
		t.Fatalf("seasons = %v, want 1, 2, then specials", seasons)
	}
	first, second := s.Seasons[0].Episodes[0], s.Seasons[0].Episodes[1]
	if len(first.Watches) != 2 || first.Watches[0].At.Before(first.Watches[1].At) {
		t.Errorf("1x01 plays = %+v, want two, newest first", first.Watches)
	}
	if len(second.Watches) != 0 || second.Resume == nil || second.Resume.Progress != 40 {
		t.Errorf("1x02 = %+v, want paused at 40%%", second)
	}
	if len(s.Seasons[1].Episodes) != 1 || s.Seasons[1].Episodes[0].Episode.FirstAired == nil {
		t.Errorf("episodes that haven't aired should be listed: %+v", s.Seasons[1])
	}
	if again, err := Title(ctx, db, profile, strconv.FormatInt(show, 10), ""); err != nil || again.Item.Title.ID != show {
		t.Errorf("by ID = %+v, %v", again.Item, err)
	}

	film, err := LookUp(ctx, db, "movie", IDs{TMDB: 27205}, false)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Title(ctx, db, profile, film[0].Title.Slug, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Watches) != 1 || f.Watches[0].Action != "watch" || f.Listing == nil || f.Rating != nil || f.Seasons != nil {
		t.Errorf("film = %+v", f)
	}

	accountID, _ := signedUpAccount(t, db)
	other, err := AddProfile(ctx, db, accountID, NewProfile{Name: "Sam", Kind: "person"})
	if err != nil {
		t.Fatal(err)
	}
	if s, err := Title(ctx, db, other.ID, "breaking-bad-2008", ""); err != nil || s.Rating != nil || len(s.Seasons[0].Episodes[0].Watches) != 0 {
		t.Errorf("someone else's state leaked: %+v, %v", s, err)
	}
	if byTMDB, err := Title(ctx, db, profile, "tmdb-1396", "show"); err != nil || byTMDB.Item.Title.ID != show {
		t.Errorf("by tmdb id = %v", err)
	}
	if _, err := Title(ctx, db, profile, "tmdb-1396", "movie"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a show's tmdb id as a film = %v", err)
	}
	if _, err := Title(ctx, db, profile, "tmdb-99999", "show"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a tmdb id Tethr hasn't seen = %v", err)
	}
	if _, err := Title(ctx, db, profile, "no-such-show-1999", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown slug = %v", err)
	}
}

func TestSearchLibrary(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	caller, _, _ := connectedProfile(t, db)
	profile := caller.ProfileID

	if _, err := AddHistory(ctx, db, caller, []Watch{{Ref: episode(1, 1), At: *aired(2)}, {Ref: inception(), At: *aired(3)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddToWatchlist(ctx, db, profile, []Listing{{Ref: Ref{Kind: "movie", IDs: IDs{TMDB: 194}}}}); err != nil {
		t.Fatal(err)
	}
	named := func(kind string, tmdb int64, w WikidataTitle) {
		t.Helper()
		items, err := LookUp(ctx, db, kind, IDs{TMDB: tmdb}, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveWikidata(ctx, db, items[0].Title.ID, kind, w); err != nil {
			t.Fatal(err)
		}
	}
	named("show", 1396, WikidataTitle{QID: "Q1079", Name: "Breaking Bad", Year: 2008})
	named("movie", 27205, WikidataTitle{QID: "Q25188", Name: "Inception", Year: 2010})
	named("movie", 194, WikidataTitle{QID: "Q484048", Name: "Amélie", Original: "Le Fabuleux Destin d'Amélie Poulain", Year: 2001})
	named("movie", 155, WikidataTitle{QID: "Q163872", Name: "The Dark Knight", Year: 2008})

	search := func(words, kind string) []string {
		t.Helper()
		items, err := Search(ctx, db, profile, words, kind)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, i := range items {
			names = append(names, *i.Title.Name)
		}
		return names
	}
	if got := search("bad", ""); len(got) != 1 || got[0] != "Breaking Bad" {
		t.Errorf("bad = %v", got)
	}
	if got := search("destin", ""); len(got) != 1 || got[0] != "Amélie" {
		t.Errorf("original names should match: %v", got)
	}
	if got := search("knight", ""); len(got) != 0 {
		t.Errorf("titles outside the library shouldn't match: %v", got)
	}
	if got := search("e", "movie"); len(got) != 2 || got[0] != "Amélie" {
		t.Errorf("films with an e = %v", got)
	}
	if got := search("%", ""); len(got) != 0 {
		t.Errorf("%% should be literal: %v", got)
	}
	if got := search("in", ""); len(got) < 2 || got[0] != "Inception" {
		t.Errorf("names starting with the words should come first: %v", got)
	}
}
