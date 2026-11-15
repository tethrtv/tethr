package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func catalogueTitle(t *testing.T, db *pgxpool.Pool, kind string, w WikidataTitle) int64 {
	t.Helper()
	res, err := SeedTitle(context.Background(), db, kind, w, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	return res.Title
}

func names(items []Item) string {
	var out []string
	for _, i := range items {
		out = append(out, fmt.Sprintf("%s %d", deref(i.Title.Name), deref(i.Title.Year)))
	}
	return fmt.Sprint(out)
}

func TestSearchCatalogue(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	acct, ann := household(t, db, "ann")
	kid, err := AddProfile(ctx, db, acct, NewProfile{Name: "Ollie", Kind: "kids"})
	if err != nil {
		t.Fatal(err)
	}
	catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q1", Name: "The Dark Knight", Year: 2008, Sitelinks: 100})
	catalogueTitle(t, db, "show", WikidataTitle{QID: "Q2", Name: "Dark", Year: 2017, Sitelinks: 80})
	darkman := catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q3", Name: "Darkman", Year: 1990, Sitelinks: 30})
	catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q4", Name: "Amélie", Original: "Le Fabuleux Destin d'Amélie Poulain", Year: 2001, Sitelinks: 70})
	catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q5", Name: "Akira", Original: "アキラ", Year: 1988, Sitelinks: 60})
	adult := catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q6", Name: "Dark Desires", Year: 1999, Sitelinks: 5})
	kiss := catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q7", Name: "Dark Kiss", Year: 2000, Sitelinks: 4})
	gone := catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q8", Name: "Dark Days", Year: 2000, Sitelinks: 3})
	catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q9", Name: "Dark Knight", Year: 2000, Sitelinks: 2})
	for _, sql := range []string{
		fmt.Sprintf("UPDATE titles SET adult = true WHERE id = %d", adult),
		fmt.Sprintf("UPDATE titles SET sexual_content = true WHERE id = %d", kiss),
		fmt.Sprintf("UPDATE titles SET merged_into_id = %d WHERE id = %d", darkman, gone),
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AddToWatchlist(ctx, db, ann, []Listing{{Ref: Ref{Kind: "movie", IDs: IDs{Tethr: darkman}}}}); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		profile     int64
		words, kind string
		scope, want string
	}{
		{ann, "dark", "", "library", "[Darkman 1990]"},
		{ann, "dark", "", "catalogue", "[Darkman 1990 Dark 2017 The Dark Knight 2008 Dark Kiss 2000 Dark Knight 2000]"},
		{kid.ID, "dark", "", "catalogue", "[Dark 2017 The Dark Knight 2008 Darkman 1990 Dark Knight 2000]"},
		{ann, "dark knight", "", "catalogue", "[The Dark Knight 2008 Dark Knight 2000]"},
		{ann, "dark", "show", "catalogue", "[Dark 2017]"},
		{ann, "  AMELIE ", "", "catalogue", "[Amélie 2001]"},
		{ann, "destin d amelie", "", "catalogue", "[Amélie 2001]"},
		{ann, "アキラ", "", "catalogue", "[Akira 1988]"},
		{ann, "d", "", "catalogue", "[Darkman 1990]"},
		{ann, "%", "", "catalogue", "[]"},
	} {
		got, err := Search(ctx, db, c.profile, c.words, c.kind, c.scope)
		if err != nil {
			t.Fatal(err)
		}
		if names(got) != c.want {
			t.Errorf("%s search for %q (%s) = %s, want %s", c.scope, c.words, c.kind, names(got), c.want)
		}
	}
}

func TestMatchFilms(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q1", Name: "The Dark Knight", Year: 2008, Sitelinks: 100})
	catalogueTitle(t, db, "show", WikidataTitle{QID: "Q2", Name: "Dark", Year: 2017, Sitelinks: 80})
	catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q3", Name: "Darkman", Year: 1990, Sitelinks: 30})
	catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q4", Name: "Amélie", Original: "Le Fabuleux Destin d'Amélie Poulain", Year: 2001, Sitelinks: 70})
	catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q5", Name: "Solaris", Year: 1972, Sitelinks: 50})
	catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q6", Name: "Solaris", Year: 2002, Sitelinks: 40})
	adult := catalogueTitle(t, db, "movie", WikidataTitle{QID: "Q7", Name: "Dark Desires", Year: 1999, Sitelinks: 5})
	if _, err := db.Exec(ctx, fmt.Sprintf("UPDATE titles SET adult = true WHERE id = %d", adult)); err != nil {
		t.Fatal(err)
	}

	got, err := MatchFilms(ctx, db, []FilmName{
		{Name: "amelie", Year: 2001},
		{Name: "Le Fabuleux Destin d’Amélie Poulain", Year: 2001},
		{Name: "The Dark Knight", Year: 2009},
		{Name: "The Dark Knight", Year: 2012},
		{Name: "Dark Knight", Year: 2008},
		{Name: "Solaris", Year: 2002},
		{Name: "Solaris", Year: 2003},
		{Name: "Solaris"},
		{Name: "Dark", Year: 2017},
		{Name: "Dark Desires", Year: 1999},
		{Name: "   "},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range got {
		s := "none"
		if m.Match != nil {
			s = names([]Item{*m.Match})
		}
		out = append(out, s+" "+names(m.Candidates))
	}
	want := []string{
		"[Amélie 2001] [Amélie 2001]",
		"[Amélie 2001] [Amélie 2001]",
		"none [The Dark Knight 2008]",
		"none [The Dark Knight 2008]",
		"[The Dark Knight 2008] [The Dark Knight 2008]",
		"[Solaris 2002] [Solaris 1972 Solaris 2002]",
		"none [Solaris 2002 Solaris 1972]",
		"none [Solaris 1972 Solaris 2002]",
		"none [The Dark Knight 2008 Darkman 1990]",
		"none []",
		"none []",
	}
	if fmt.Sprint(out) != fmt.Sprint(want) {
		t.Errorf("matches =\n%s\nwant\n%s", out, want)
	}
}
