package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestTitleFactsAndIDs(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	ctx := context.Background()
	res, err := core.SeedTitle(ctx, s.DB, "show", core.WikidataTitle{QID: "Q1079", Name: "Breaking Bad", Year: 2008, Sitelinks: 90}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	show := res.Title
	bare, err := core.SeedTitle(ctx, s.DB, "movie", core.WikidataTitle{QID: "Q2", Name: "Breaking Glass", Year: 1980, Sitelinks: 5}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		fmt.Sprintf(`UPDATE titles SET genres = '{drama,crime}', countries = '{US}', languages = '{en,es}', type_flags = '{}', started = '2008-01-20', ended = '2013-09' WHERE id = %d`, show),
		fmt.Sprintf(`DELETE FROM external_ids WHERE title_id = %d`, show),
		fmt.Sprintf(`UPDATE titles SET wikidata_deleted_at = now() WHERE id = %d`, bare.Title),
		fmt.Sprintf(`INSERT INTO external_ids (title_id, provider, value, source) VALUES
			(%[1]d, 'imdb', 'tt0903747', 'wikidata'), (%[1]d, 'tvmaze', '169', 'wikidata'), (%[1]d, 'tvmaze', '9999', 'wikidata'),
			(%[1]d, 'netflix', '70143836', 'wikidata'), (%[1]d, 'rottentomatoes', 'tv/breaking_bad', 'wikidata'),
			(%[1]d, 'mal', 'not-a-number', 'wikidata'), (%[1]d, 'trakt_slug', 'shows/breaking-bad', 'wikidata')`, show),
	} {
		if _, err := s.DB.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}

	rec := do(t, h, "GET", "/v1/titles/"+strconv.FormatInt(show, 10), "", cookie)
	var detail TitleDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("title = %d %s", rec.Code, rec.Body)
	}
	f := detail.Facts
	if f == nil || fmt.Sprintf("%v %v %v %s %s", value(f.Genres), value(f.Countries), value(f.Languages), value(f.Started), value(f.Ended)) != "[drama crime] [US] [en es] 2008-01-20 2013-09" || f.TypeFlags != nil {
		t.Errorf("facts = %s", rec.Body)
	}
	ids := detail.Title.Ids
	if value(ids.Wikidata) != "Q1079" || value(ids.Tvmaze) != 169 || value(ids.Netflix) != "70143836" || value(ids.Rottentomatoes) != "tv/breaking_bad" ||
		value(ids.TraktSlug) != "shows/breaking-bad" || value(ids.Imdb) != "tt0903747" || ids.Mal != nil || ids.Letterboxd != nil {
		t.Errorf("ids = %s", rec.Body)
	}

	rec = do(t, h, "GET", "/v1/titles/"+strconv.FormatInt(bare.Title, 10), "", cookie)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"facts"`) || strings.Contains(rec.Body.String(), `"wikidata"`) {
		t.Errorf("a title with no facts = %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, "GET", "/v1/search?q=breaking+bad&scope=catalogue", "", cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tvmaze":169`) || strings.Contains(rec.Body.String(), `"genres"`) {
		t.Errorf("search results carry every id and no facts: %d %s", rec.Code, rec.Body)
	}
}
