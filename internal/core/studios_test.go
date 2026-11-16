package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestStudiosAndNetworks(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	items, err := LookUp(ctx, db, "show", IDs{TMDB: 66732}, true)
	if err != nil {
		t.Fatal(err)
	}
	id := items[0].Title.ID
	w := WikidataTitle{QID: "Q22137573", Name: "Stranger Things", Year: 2016, TMDB: 66732,
		Studios: []string{"Q21000155", "Q28407050", "Q43994"}, Networks: []string{"Q907311"}}
	if _, err := SaveWikidata(ctx, db, id, "show", w); err != nil {
		t.Fatal(err)
	}

	needed, err := NamesNeeded(ctx, db, 10)
	if err != nil || fmt.Sprint(needed) != "[Q21000155 Q28407050 Q43994 Q907311]" {
		t.Errorf("names needed = %v, %v", needed, err)
	}
	if err := SaveClasses(ctx, db, []WikidataClass{{QID: "Q21000155", Label: "21 Laps Entertainment"}, {QID: "Q907311", Label: "Netflix"}, {QID: "Q28407050"}, {QID: "Q43994", Label: "Matt Groening", Human: true}}); err != nil {
		t.Fatal(err)
	}
	if needed, err := NamesNeeded(ctx, db, 10); err != nil || len(needed) != 0 {
		t.Errorf("names still needed = %v, %v", needed, err)
	}

	s, err := Title(ctx, db, 0, fmt.Sprint(id), "show")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(s.Facts.Studios, s.Facts.Networks); got != "[{Q21000155 21 Laps Entertainment}] [{Q907311 Netflix}]" {
		t.Errorf("studios and networks = %s", got)
	}

	w.Studios, w.Networks = nil, nil
	if _, err := SaveWikidata(ctx, db, id, "show", w); err != nil {
		t.Fatal(err)
	}
	if s, err := Title(ctx, db, 0, fmt.Sprint(id), "show"); err != nil || s.Facts.Studios != nil || s.Facts.Networks != nil {
		t.Errorf("after Wikidata dropped them = %+v, %v", s.Facts, err)
	}
}
