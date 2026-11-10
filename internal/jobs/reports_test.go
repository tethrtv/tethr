package jobs

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/dbtest"
	"github.com/tethrtv/tethr/internal/mail"
)

type outbox []mail.Message

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	*o = append(*o, m)
	return nil
}

func TestReportSummary(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	profile := func(username string) int64 {
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
	owner, ann := profile("owner"), profile("ann")

	var sent outbox
	job := Reports{DB: db, Mail: &sent, To: "reports@example.com", AppURL: "https://tethr.example", Zone: time.UTC, Log: slog.New(slog.DiscardHandler)}
	nine := time.Date(2026, 11, 10, 9, 20, 0, 0, time.UTC)
	if err := job.summarise(ctx, nine); err != nil || len(sent) != 0 {
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
	if err := job.summarise(ctx, nine.Add(time.Hour)); err != nil || len(sent) != 0 {
		t.Fatalf("outside the morning = %d emails, %v", len(sent), err)
	}
	if err := job.summarise(ctx, nine); err != nil || len(sent) != 1 {
		t.Fatalf("in the morning = %d emails, %v", len(sent), err)
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

func ptr[T any](v T) *T { return &v }
