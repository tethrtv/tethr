package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/mail"
)

type outbox []mail.Message

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	*o = append(*o, m)
	return nil
}

func signedUp(t *testing.T, db *pgxpool.Pool, username string) int64 {
	t.Helper()
	ctx := context.Background()
	email, err := core.StartSignUp(ctx, db, core.SignUp{Email: username + "@example.com", Name: username, Username: username}, false)
	if err != nil {
		t.Fatal(err)
	}
	session, err := core.Verify(ctx, db, core.Verification{Token: email.Token}, "test")
	if err != nil {
		t.Fatal(err)
	}
	account, err := core.SessionAccount(ctx, db, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, profiles, err := core.Me(ctx, db, account)
	if err != nil {
		t.Fatal(err)
	}
	return profiles[0].ID
}

func TestReportSummary(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	owner, ann := signedUp(t, db, "owner"), signedUp(t, db, "ann")

	var sent outbox
	job := Reports{DB: db, Mail: &sent, To: "reports@example.com", AppURL: "https://tethr.example", Zone: time.UTC, Log: slog.New(slog.DiscardHandler)}
	nine := time.Date(2026, 11, 10, 9, 20, 0, 0, time.UTC)
	if err := job.summarise(ctx, nine.Add(-24*time.Hour)); err != nil || len(sent) != 0 {
		t.Fatalf("nothing waiting = %d emails, %v", len(sent), err)
	}

	for _, c := range []struct{ name, reason string }{{"Spammy", "spam"}, {"Doxxing", "personal"}} {
		l, err := core.CreateList(ctx, db, owner, core.ListChanges{Name: &c.name, Privacy: ptr("public")})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := core.ReportList(ctx, db, ann, l.ID, c.reason, "look at this"); err != nil {
			t.Fatal(err)
		}
	}
	if err := job.summarise(ctx, nine.Add(-time.Hour)); err != nil || len(sent) != 0 {
		t.Fatalf("before nine = %d emails, %v", len(sent), err)
	}
	if err := job.summarise(ctx, nine); err != nil || len(sent) != 1 {
		t.Fatalf("in the morning = %d emails, %v", len(sent), err)
	}
	for _, later := range []time.Duration{30 * time.Minute, 5 * time.Hour} {
		if err := job.summarise(ctx, nine.Add(later)); err != nil || len(sent) != 1 {
			t.Fatalf("%d emails after a restart later that day, %v", len(sent), err)
		}
	}
	m := sent[0]
	if m.To != "reports@example.com" || m.Subject != "Reports waiting on 2 lists" {
		t.Errorf("summary = %q to %q", m.Subject, m.To)
	}
	for _, want := range []string{
		"“Doxxing” by owner, out of browsing",
		"“Spammy” by owner, still in browsing",
		"personal, from ann on ",
		"look at this",
		"https://tethr.example/@owner/lists/doxxing",
		"tethr report keep",
	} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("summary is missing %q:\n%s", want, m.Text)
		}
	}
}

func TestReviewSummary(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	title := func(qid, name string, year int, sitelinks int) int64 {
		res, err := core.SeedTitle(ctx, db, "movie", core.WikidataTitle{QID: qid, Name: name, Year: year, Sitelinks: sitelinks}, 0, false)
		if err != nil {
			t.Fatal(err)
		}
		return res.Title
	}
	akira := title("Q1", "Akira", 1988, 80)
	var reels []core.ReviewFlag
	for i := range 7 {
		id := title(fmt.Sprintf("Q%d", 100+i), fmt.Sprintf("Newsreel %d", 60+i), 1944, i)
		reels = append(reels, core.ReviewFlag{Kind: core.KindChange, QID: fmt.Sprintf("Q%d", 100+i), Title: id, Tethr: "movie", Wikidata: "none", Batch: "QSv2/300001"})
	}
	heat := title("Q2", "Heat", 1995, 50)
	if err := core.FlagForReview(ctx, db, []core.ReviewFlag{{Kind: core.DeletedItem, QID: "Q2", Title: heat}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE review_entries SET created_at = '2026-11-09 12:00Z'`); err != nil {
		t.Fatal(err)
	}

	var sent outbox
	job := Reports{DB: db, Mail: &sent, To: "reports@example.com", AppURL: "https://tethr.example", Zone: time.UTC, Log: slog.New(slog.DiscardHandler)}
	nine := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	if err := job.summarise(ctx, nine.Add(-20*time.Hour)); err != nil || len(sent) != 1 {
		t.Fatalf("first morning = %d emails, %v", len(sent), err)
	}
	sent = nil
	if err := job.summarise(ctx, nine); err != nil || len(sent) != 0 {
		t.Fatalf("nothing new = %d emails, %v", len(sent), err)
	}

	flags := append(reels, core.ReviewFlag{Kind: core.KeptName, QID: "Q1", Title: akira, Tethr: "Akira", Wikidata: "アキラ\ntethr review accept 1", Why: "out of latin script"})
	if err := core.FlagForReview(ctx, db, flags); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE review_entries SET created_at = '2026-11-10 12:00Z' WHERE kind <> 'deleted_item'`); err != nil {
		t.Fatal(err)
	}
	if err := job.summarise(ctx, nine.Add(24*time.Hour)); err != nil || len(sent) != 1 {
		t.Fatalf("next morning = %d emails, %v", len(sent), err)
	}
	m := sent[0]
	if m.Subject != "8 new for catalogue review" {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{
		"7 titles from one batch stopped being films",
		"https://editgroups.toolforge.org/b/QSv2/300001/",
		"Newsreel 66 (1944), Newsreel 65 (1944), Newsreel 64 (1944), Newsreel 63 (1944), Newsreel 62 (1944) and 2 more",
		"tethr review batch QSv2/300001",
		"Akira (1988): kept “Akira”, Wikidata says “アキラ tethr review accept 1” (out of latin script)",
		"https://www.wikidata.org/wiki/Q1",
		"Still open from before: 1 deleted item. Run tethr review on the server for the list.",
	} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("summary is missing %q:\n%s", want, m.Text)
		}
	}
	if strings.Contains(m.Text, "Heat") {
		t.Errorf("an entry from before was listed in full:\n%s", m.Text)
	}
}

func ptr[T any](v T) *T { return &v }

func TestTitleReportsInSummary(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	ann, bob := signedUp(t, db, "ann"), signedUp(t, db, "bob")
	res, err := core.SeedTitle(ctx, db, "movie", core.WikidataTitle{QID: "Q1", Name: "Akira", Year: 1988}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprint(res.Title)
	for _, r := range []struct {
		from int64
		core.TitleReport
	}{
		{ann, core.TitleReport{Reason: "name", Suggestion: "AKIRA", Note: "The poster\ntethr review accept 1"}},
		{bob, core.TitleReport{Reason: "name"}},
		{bob, core.TitleReport{Reason: "kind", Suggestion: "show"}},
	} {
		if _, err := core.ReportTitle(ctx, db, r.from, id, r.TitleReport); err != nil {
			t.Fatal(err)
		}
	}

	var sent outbox
	job := Reports{DB: db, Mail: &sent, To: "reports@example.com", AppURL: "https://tethr.example", Zone: time.UTC, Log: slog.New(slog.DiscardHandler)}
	tomorrow := time.Now().UTC().AddDate(0, 0, 1)
	morning := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), summaryHour, 20, 0, 0, time.UTC)
	if err := job.summarise(ctx, morning); err != nil || len(sent) != 1 {
		t.Fatalf("summary = %d emails, %v", len(sent), err)
	}
	m := sent[0]
	if m.Subject != "3 new for catalogue review" {
		t.Errorf("subject = %q", m.Subject)
	}
	want := fmt.Sprintf(`Title reports

Akira (1988): name reported by 2 households, Tethr has “Akira”
  https://tethr.example/films/t-%[1]s
  https://www.wikidata.org/wiki/Q1
  suggests “AKIRA”, note: The poster tethr review accept 1
  no suggestion or note
  tethr review dismiss %[2]d <reason>

Akira (1988): reported as not a film by 1 household
  https://tethr.example/films/t-%[1]s
  https://www.wikidata.org/wiki/Q1
  suggests a show
  tethr review dismiss %[3]d <reason>

`, id, lastReport(t, db, "name"), lastReport(t, db, "kind"))
	if m.Text != want {
		t.Errorf("summary =\n%s\nwant\n%s", m.Text, want)
	}
}

func lastReport(t *testing.T, db *pgxpool.Pool, subject string) int64 {
	t.Helper()
	open, err := core.OpenReview(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	for _, e := range open {
		if e.Subject == subject {
			id = max(id, e.ID)
		}
	}
	if id == 0 {
		t.Fatalf("no %s report", subject)
	}
	return id
}
