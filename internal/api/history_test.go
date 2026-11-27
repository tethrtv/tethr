package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestDatedWatches(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	if rec := do(t, h, "POST", "/v1/history", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}},"precision":"none"}]}`, cookie); rec.Code != http.StatusOK {
		t.Fatalf("add history = %d %s", rec.Code, rec.Body)
	}
	if _, err := s.DB.Exec(context.Background(), `UPDATE titles SET released = '2010-07-08'`); err != nil {
		t.Fatal(err)
	}
	rec := do(t, h, "POST", "/v1/history", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}},"released":true},{"item":{"type":"movie","ids":{"tmdb":27205}},"watched_at":"2024-06-02T23:30:00Z","precision":"day"}]}`, cookie)
	var added SyncResult
	_ = json.Unmarshal(rec.Body.Bytes(), &added)
	if rec.Code != http.StatusOK || added.Added != 2 || added.Ids == nil || len(*added.Ids) != 2 {
		t.Fatalf("add dated history = %d %s", rec.Code, rec.Body)
	}

	var history HistoryPage
	_ = json.Unmarshal(do(t, h, "GET", "/v1/history", "", cookie).Body.Bytes(), &history)
	got := map[string]string{}
	for _, w := range history.Items {
		got[w.WatchedAt.UTC().Format("2006-01-02 15:04")] = string(w.Precision)
	}
	want := map[string]string{"1970-01-01 00:00": "none", "2010-07-08 12:00": "day", "2024-06-02 12:00": "day"}
	if len(got) != len(want) {
		t.Fatalf("history = %v", got)
	}
	for at, p := range want {
		if got[at] != p {
			t.Errorf("history = %v, want %v", got, want)
		}
	}
	if r := history.Items[0].Item.Title.Released; r == nil || r.String() != "2010-07-08" {
		t.Errorf("title released = %v", r)
	}
}
