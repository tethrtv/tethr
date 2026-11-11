package jobs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/mail"
	"github.com/tethrtv/tethr/internal/tmdb"
)

func TestWeeklyEmail(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	account, _, err := core.CreateAccount(ctx, db, core.SignUp{Email: "nathan@example.com", Name: "Nathan", Username: "nathan"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "UPDATE accounts SET email_verified_at = now() WHERE id = $1", account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := core.UpdateAccount(ctx, db, account.ID, core.AccountChanges{TimeZone: ptr("America/New_York")}); err != nil {
		t.Fatal(err)
	}
	caller, err := core.AccountCaller(ctx, db, account.ID)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().In(time.UTC)
	for now.In(mustZone(t, "America/New_York")).Weekday() != time.Sunday {
		now = now.AddDate(0, 0, 1)
	}
	ny := now.In(mustZone(t, "America/New_York"))
	sundayEvening := time.Date(ny.Year(), ny.Month(), ny.Day(), 18, 15, 0, 0, ny.Location())
	monday := time.Date(ny.Year(), ny.Month(), ny.Day()+1, 0, 0, 0, 0, time.UTC)
	wednesday := monday.AddDate(0, 0, 2)

	season, number := int32(1), int32(1)
	played, err := core.ScrobbleStop(ctx, db, caller, core.Scrobble{Ref: core.Ref{Kind: "episode", Show: core.IDs{TMDB: 1396}, Season: &season, Number: &number}, Progress: 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := core.SaveOutline(ctx, db, played.Item.Title.ID, []core.OutlineEpisode{
		{TMDBID: 101, Season: 1, Number: 1, FirstAired: &monday},
		{TMDBID: 102, Season: 1, Number: 2, FirstAired: &monday},
		{TMDBID: 103, Season: 1, Number: 3, FirstAired: &wednesday},
		{TMDBID: 104, Season: 1, Number: 4, FirstAired: &wednesday},
		{TMDBID: 106, Season: 1, Number: 6, FirstAired: &wednesday},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "UPDATE titles SET name = 'Breaking Bad' WHERE id = $1", played.Item.Title.ID); err != nil {
		t.Fatal(err)
	}

	tmdbUp := true
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !tmdbUp {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/tv/1396":
			_, _ = w.Write([]byte(`{"name":"Breaking Bad","poster_path":"/bb.jpg"}`))
		case "/3/tv/1396/season/1":
			_, _ = w.Write([]byte(`{"episodes":[{"episode_number":2,"name":"Cat in the Bag"},{"episode_number":3,"name":"And the Bag in the River"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()

	job := Weekly{
		DB: db, Mail: failing{}, AppURL: "https://tethr.example", APIURL: "https://api.tethr.example",
		Zone: time.UTC, Log: slog.New(slog.DiscardHandler),
	}
	if err := job.send(ctx, sundayEvening); err == nil {
		t.Error("a failed send wasn't reported")
	}

	var sent outbox
	job = Weekly{
		DB: db, Mail: &sent, TMDB: &tmdb.Client{BaseURL: fake.URL},
		AppURL: "https://tethr.example", APIURL: "https://api.tethr.example",
		Zone: time.UTC, Log: slog.New(slog.DiscardHandler),
	}

	if err := job.send(ctx, sundayEvening.Add(-time.Hour)); err != nil || len(sent) != 0 {
		t.Fatalf("before 6pm in New York = %d emails, %v", len(sent), err)
	}
	if err := job.send(ctx, sundayEvening); err != nil || len(sent) != 1 {
		t.Fatalf("Sunday evening in New York, after a failed send = %d emails, %v", len(sent), err)
	}
	if err := job.send(ctx, sundayEvening.Add(time.Hour)); err != nil || len(sent) != 1 {
		t.Fatalf("an hour later = %d emails, %v", len(sent), err)
	}

	m := sent[0]
	if m.To != "nathan@example.com" || !strings.HasPrefix(m.Unsubscribe, "https://api.tethr.example/v1/unsubscribe?token=") {
		t.Errorf("email to %q, unsubscribe %q", m.To, m.Unsubscribe)
	}
	for _, want := range []string{
		"Breaking Bad",
		"S1E2, Cat in the Bag",
		"S1E3 to S1E4",
		"S1E6",
		"https://image.tmdb.org/t/p/w154/bb.jpg",
		"https://tethr.example/shows/",
		"https://tethr.example/unsubscribe?token=",
		"isn’t endorsed or certified by TMDB",
		"you watched an episode",
	} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("email is missing %q", want)
		}
	}
	if !strings.Contains(m.Text, "Monday") || !strings.Contains(m.Text, "Wednesday") {
		t.Errorf("text email:\n%s", m.Text)
	}
	if strings.Contains(m.HTML, "S1E1") {
		t.Error("the watched episode is in the email")
	}

	tmdbUp = false
	if _, err := db.Exec(ctx, "UPDATE accounts SET weekly_sent_at = NULL"); err != nil {
		t.Fatal(err)
	}
	if err := job.send(ctx, sundayEvening); err != nil || len(sent) != 2 {
		t.Fatalf("with TMDB down = %d emails, %v", len(sent), err)
	}
	if strings.Contains(sent[1].HTML, "image.tmdb.org") || strings.Contains(sent[1].HTML, "endorsed") || strings.Contains(sent[1].HTML, "Cat in the Bag") || !strings.Contains(sent[1].Text, "Breaking Bad, S1E2\n") {
		t.Errorf("email with TMDB down still leans on it:\n%s", sent[1].Text)
	}

	if err := core.Unsubscribe(ctx, db, strings.TrimPrefix(m.Unsubscribe, "https://api.tethr.example/v1/unsubscribe?token=")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "UPDATE accounts SET weekly_sent_at = NULL"); err != nil {
		t.Fatal(err)
	}
	if err := job.send(ctx, sundayEvening); err != nil || len(sent) != 2 {
		t.Fatalf("after unsubscribing = %d emails, %v", len(sent), err)
	}
}

type failing struct{}

func (failing) Send(context.Context, mail.Message) error { return errors.New("mail service is down") }

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
