package jobs

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/tmdb"
)

func TestRefreshOutlines(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	account, _, err := core.CreateAccount(ctx, db, core.SignUp{Email: "nathan@example.com", Name: "Nathan", Username: "nathan"})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := core.AccountCaller(ctx, db, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tmdbID := range []int64{1396, 404} {
		season, number := int32(1), int32(1)
		ref := core.Ref{Kind: "episode", Show: core.IDs{TMDB: tmdbID}, Season: &season, Number: &number}
		if _, err := core.ScrobbleStop(ctx, db, caller, core.Scrobble{Ref: ref, Progress: 100}); err != nil {
			t.Fatal(err)
		}
	}

	var requests []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.RequestURI() {
		case "/3/tv/1396":
			_, _ = w.Write([]byte(`{"name":"ignored","seasons":[{"season_number":0},{"season_number":1}]}`))
		case "/3/tv/1396?append_to_response=season/0,season/1":
			_, _ = w.Write([]byte(`{"name":"ignored",
				"season/0":{"episodes":[{"id":62085,"season_number":0,"episode_number":1,"air_date":"2009-02-17","name":"ignored"}]},
				"season/1":{"episodes":[
					{"id":62092,"season_number":1,"episode_number":1,"air_date":"2008-01-20"},
					{"id":62093,"season_number":1,"episode_number":2,"air_date":"2008-01-27"},
					{"id":62094,"season_number":1,"episode_number":3,"air_date":""}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()

	job := Outlines{DB: db, TMDB: &tmdb.Client{Token: "secret", BaseURL: fake.URL}, Log: slog.New(slog.DiscardHandler)}
	if err := job.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Errorf("requests = %v", requests)
	}

	next, err := core.UpNext(ctx, db, caller.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 || next[0].Next.EpisodeNumber != 2 || *next[0].Next.TmdbEpisodeID != 62093 || next[0].Aired != 2 {
		t.Errorf("up next = %+v", next)
	}
	if due, _ := core.DueOutlines(ctx, db, 10); len(due) != 0 {
		t.Errorf("a show TMDB doesn't have should wait like the rest, not retry every minute: %+v", due)
	}
	requests = nil
	if err := job.Refresh(ctx); err != nil || len(requests) != 0 {
		t.Errorf("second refresh = %v, %v", requests, err)
	}
}
