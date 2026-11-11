package jobs

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/mail"
	"github.com/tethrtv/tethr/internal/store"
	"github.com/tethrtv/tethr/internal/tmdb"
)

const (
	weeklyHour    = 18
	weeklyPopular = 5
	posters       = "https://image.tmdb.org/t/p/w154"
)

//go:embed weekly.html
var weeklyHTML string

var weeklyPage = template.Must(template.New("weekly").Parse(weeklyHTML))

type Weekly struct {
	DB     *pgxpool.Pool
	Mail   Sender
	TMDB   *tmdb.Client
	AppURL string
	APIURL string
	Zone   *time.Location
	Log    *slog.Logger
}

func (w Weekly) Run(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if err := w.send(ctx, time.Now()); err != nil && ctx.Err() == nil {
			w.Log.Error("sending weekly emails", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (w Weekly) send(ctx context.Context, now time.Time) error {
	q := store.New(w.DB)
	sentBefore := now.AddDate(0, 0, -6)
	due, err := q.WeeklyRecipients(ctx, sentBefore)
	if err != nil {
		return err
	}
	art := &decor{tmdb: w.TMDB, titles: map[string]tmdb.Title{}, seasons: map[string]tmdb.Season{}, log: w.Log}
	for _, r := range due {
		if err := ctx.Err(); err != nil {
			return err
		}
		loc := w.Zone
		if r.TimeZone != nil {
			if l, err := core.ParseTimeZone(*r.TimeZone); err == nil {
				loc = l
			}
		}
		if local := now.In(loc); local.Weekday() != time.Sunday || local.Hour() < weeklyHour {
			continue
		}
		claimed, err := q.ClaimWeekly(ctx, store.ClaimWeeklyParams{ID: r.AccountID, SentBefore: sentBefore})
		if err != nil {
			return err
		}
		if claimed == 0 {
			continue
		}
		week, err := core.WeekAhead(ctx, w.DB, r.ProfileID, now, loc)
		if err != nil {
			if ctx.Err() != nil {
				return w.release(r, ctx.Err())
			}
			w.Log.Error("building a weekly email", "account", r.AccountID, "err", err)
			continue
		}
		if week.Empty() {
			continue
		}
		m, ok, err := w.message(ctx, r, week, art)
		if err != nil {
			w.Log.Error("writing a weekly email", "account", r.AccountID, "err", err)
			continue
		}
		if !ok {
			continue
		}
		// A failed send is usually the mail service, not this account, so stop and let the next hour retry everyone.
		if err := w.Mail.Send(ctx, m); err != nil {
			return w.release(r, fmt.Errorf("account %d: %w", r.AccountID, err))
		}
	}
	return nil
}

func (w Weekly) release(r store.WeeklyRecipientsRow, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.New(w.DB).ReleaseWeekly(ctx, store.ReleaseWeeklyParams{ID: r.AccountID, SentAt: r.WeeklySentAt}); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

type weeklyRow struct {
	Name, Link, Poster, Detail string
}

type weeklyDay struct {
	Day  string
	Rows []weeklyRow
}

type weeklyList struct {
	Name, Owner, Link, Added string
}

type weeklyData struct {
	Title, Preview, Range, Watched string
	Days                           []weeklyDay
	Premieres                      []weeklyRow
	Lists                          []weeklyList
	Popular                        []weeklyRow
	AppURL, Unsubscribe            string
	TMDB                           bool
}

func (w Weekly) message(ctx context.Context, r store.WeeklyRecipientsRow, week core.Weekly, art *decor) (mail.Message, bool, error) {
	art.used = false
	token := r.UnsubscribeToken.String()
	d := weeklyData{
		Title:       "Your week ahead",
		Range:       weekRange(week.From),
		Watched:     watched(week.Watched),
		AppURL:      w.AppURL,
		Unsubscribe: w.AppURL + "/unsubscribe?token=" + token,
	}

	for i := 0; i < len(week.Episodes); i++ {
		e := week.Episodes[i]
		last := i
		for last+1 < len(week.Episodes) {
			next, prev := week.Episodes[last+1], week.Episodes[last]
			if next.Show.Title.ID != e.Show.Title.ID || !next.Episode.FirstAired.Equal(*e.Episode.FirstAired) || next.Episode.Season != prev.Episode.Season || next.Episode.Number != prev.Episode.Number+1 {
				break
			}
			last++
		}
		row, ok := w.row(ctx, e.Show, art)
		if !ok {
			i = last
			continue
		}
		row.Detail = fmt.Sprintf("S%dE%d", e.Episode.Season, e.Episode.Number)
		if last > i {
			end := week.Episodes[last].Episode
			row.Detail += fmt.Sprintf(" to S%dE%d", end.Season, end.Number)
		} else if n := art.episodeName(ctx, e.Show, e.Episode); n != "" {
			row.Detail += ", " + n
		}
		name := e.Episode.FirstAired.Format("Monday")
		if n := len(d.Days); n == 0 || d.Days[n-1].Day != name {
			d.Days = append(d.Days, weeklyDay{Day: name})
		}
		d.Days[len(d.Days)-1].Rows = append(d.Days[len(d.Days)-1].Rows, row)
		i = last
	}
	for _, e := range week.Premieres {
		if row, ok := w.row(ctx, e.Show, art); ok {
			row.Detail = "Starts " + e.Episode.FirstAired.Format("Monday")
			if e.Episode.Season > 1 {
				row.Detail = fmt.Sprintf("Season %d starts %s", e.Episode.Season, e.Episode.FirstAired.Format("Monday"))
			}
			d.Premieres = append(d.Premieres, row)
		}
	}
	for _, l := range week.Lists {
		var names []string
		for _, item := range l.Added {
			if n := art.name(ctx, item); n != "" {
				names = append(names, n)
			}
		}
		if len(names) == 0 {
			continue
		}
		added := "Added " + joinNames(names, l.More)
		d.Lists = append(d.Lists, weeklyList{Name: l.List.Name, Owner: l.List.Owner.Username, Link: fmt.Sprintf("%s/@%s/lists/%s", w.AppURL, l.List.Owner.Username, l.List.Slug), Added: added})
	}
	for _, p := range week.Popular[:min(len(week.Popular), weeklyPopular)] {
		if row, ok := w.row(ctx, p.Item, art); ok {
			row.Detail = fmt.Sprintf("%d+ households", p.Households)
			d.Popular = append(d.Popular, row)
		}
	}
	if len(d.Days)+len(d.Premieres)+len(d.Lists) == 0 && d.Watched == "" {
		return mail.Message{}, false, nil
	}
	d.TMDB = art.used
	d.Preview = preview(d)

	var html bytes.Buffer
	if err := weeklyPage.Execute(&html, d); err != nil {
		return mail.Message{}, false, err
	}
	return mail.Message{
		To:          r.Email,
		Subject:     "Your week ahead on Tethr",
		Text:        weeklyText(d),
		HTML:        html.String(),
		Unsubscribe: w.APIURL + "/v1/unsubscribe?token=" + token,
	}, true, nil
}

func (w Weekly) row(ctx context.Context, item core.Item, art *decor) (weeklyRow, bool) {
	name := art.name(ctx, item)
	if name == "" {
		return weeklyRow{}, false
	}
	section := "shows"
	if item.Title.Kind == "movie" {
		section = "films"
	}
	return weeklyRow{Name: name, Link: fmt.Sprintf("%s/%s/%s", w.AppURL, section, item.Title.Slug), Poster: art.poster(ctx, item)}, true
}

func weekRange(from time.Time) string {
	to := from.AddDate(0, 0, 6)
	if from.Month() == to.Month() {
		return fmt.Sprintf("Monday %d to Sunday %d %s", from.Day(), to.Day(), to.Format("January"))
	}
	return fmt.Sprintf("Monday %d %s to Sunday %d %s", from.Day(), from.Format("January"), to.Day(), to.Format("January"))
}

func watched(w core.WeeklyWatched) string {
	var parts []string
	switch {
	case w.Episodes == 1:
		parts = append(parts, "an episode")
	case w.Episodes > 1:
		parts = append(parts, fmt.Sprintf("%d episodes", w.Episodes))
	}
	switch {
	case w.Films == 1:
		parts = append(parts, "a film")
	case w.Films > 1:
		parts = append(parts, fmt.Sprintf("%d films", w.Films))
	}
	if len(parts) == 0 {
		return ""
	}
	s := "In the last seven days you watched " + strings.Join(parts, " and ")
	switch hours := (w.Minutes + 30) / 60; {
	case w.Minutes == 0:
	case w.Minutes == 1:
		s += ", about a minute"
	case w.Minutes < 60:
		s += fmt.Sprintf(", about %d minutes", w.Minutes)
	case hours == 1:
		s += ", about an hour"
	default:
		s += fmt.Sprintf(", about %d hours", hours)
	}
	return s + "."
}

func joinNames(names []string, more int) string {
	if more > 0 {
		return strings.Join(names, ", ") + fmt.Sprintf(" and %d more", more)
	}
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func preview(d weeklyData) string {
	var parts []string
	shows := map[string]bool{}
	for _, day := range d.Days {
		for _, r := range day.Rows {
			shows[r.Name] = true
		}
	}
	switch {
	case len(shows) == 1:
		parts = append(parts, "new episodes of a show you watch")
	case len(shows) > 1:
		parts = append(parts, fmt.Sprintf("new episodes of %d shows you watch", len(shows)))
	}
	if len(d.Premieres) > 0 {
		parts = append(parts, "something new from your watchlist")
	}
	if len(d.Lists) > 0 {
		parts = append(parts, "new picks on lists you follow")
	}
	if len(parts) == 0 {
		return "A look back at your last seven days."
	}
	return "This week: " + joinNames(parts, 0) + "."
}

func weeklyText(d weeklyData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n", d.Title, d.Range)
	section := func(title string) { fmt.Fprintf(&b, "\n%s\n\n", strings.ToUpper(title)) }
	if len(d.Days) > 0 {
		section("New episodes")
		for _, day := range d.Days {
			fmt.Fprintf(&b, "%s\n", day.Day)
			for _, r := range day.Rows {
				fmt.Fprintf(&b, "  %s, %s\n  %s\n", r.Name, r.Detail, r.Link)
			}
		}
	}
	if len(d.Premieres) > 0 {
		section("From your watchlist")
		for _, r := range d.Premieres {
			fmt.Fprintf(&b, "%s, %s\n%s\n", r.Name, r.Detail, r.Link)
		}
	}
	if len(d.Lists) > 0 {
		section("Lists you follow")
		for _, l := range d.Lists {
			fmt.Fprintf(&b, "%s, by %s\n%s\n%s\n", l.Name, l.Owner, l.Added, l.Link)
		}
	}
	if len(d.Popular) > 0 {
		section("Popular on Tethr last week")
		for i, r := range d.Popular {
			fmt.Fprintf(&b, "%d. %s, %s\n", i+1, r.Name, r.Detail)
		}
	}
	if d.Watched != "" {
		section("Your last seven days")
		b.WriteString(d.Watched + "\n")
	}
	fmt.Fprintf(&b, "\nYou get this on Sunday evenings because the weekly email is on. Turn it off on your account page, %s/account, or unsubscribe at %s\n", d.AppURL, d.Unsubscribe)
	if d.TMDB {
		b.WriteString("\nDetails from TMDB. Tethr uses the TMDB API but isn’t endorsed or certified by TMDB.\n")
	}
	return b.String()
}

type decor struct {
	tmdb    *tmdb.Client
	titles  map[string]tmdb.Title
	seasons map[string]tmdb.Season
	down    bool
	used    bool
	log     *slog.Logger
}

func (d *decor) title(ctx context.Context, item core.Item) (tmdb.Title, bool) {
	if d.tmdb == nil || d.down || item.Title.TmdbID == nil {
		return tmdb.Title{}, false
	}
	key := fmt.Sprintf("%s/%d", item.Title.Kind, *item.Title.TmdbID)
	if t, ok := d.titles[key]; ok {
		return t, true
	}
	fetch := d.tmdb.Show
	if item.Title.Kind == "movie" {
		fetch = d.tmdb.Movie
	}
	t, err := fetch(ctx, *item.Title.TmdbID)
	if !d.ok(ctx, err) {
		return tmdb.Title{}, false
	}
	d.titles[key] = t
	return t, true
}

func (d *decor) name(ctx context.Context, item core.Item) string {
	if item.Title.Name != nil {
		return *item.Title.Name
	}
	t, _ := d.title(ctx, item)
	if t.Name != "" {
		d.used = true
	}
	return t.Name
}

func (d *decor) poster(ctx context.Context, item core.Item) string {
	t, ok := d.title(ctx, item)
	if !ok || t.Poster == "" {
		return ""
	}
	d.used = true
	return posters + t.Poster
}

func (d *decor) episodeName(ctx context.Context, show core.Item, e core.OrderedEpisode) string {
	if d.tmdb == nil || d.down || show.Title.TmdbID == nil {
		return ""
	}
	key := fmt.Sprintf("%d/%d", *show.Title.TmdbID, e.SeasonNumber)
	s, ok := d.seasons[key]
	if !ok {
		var err error
		s, err = d.tmdb.Season(ctx, *show.Title.TmdbID, e.SeasonNumber)
		if !d.ok(ctx, err) {
			return ""
		}
		d.seasons[key] = s
	}
	for _, ep := range s.Episodes {
		if ep.Number == e.EpisodeNumber && ep.Name != "" && ep.Name != fmt.Sprintf("Episode %d", ep.Number) {
			d.used = true
			return ep.Name
		}
	}
	return ""
}

// One failure means TMDB is struggling, so the rest of this run's emails go out without pictures.
func (d *decor) ok(ctx context.Context, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, tmdb.ErrNotFound), ctx.Err() != nil:
		return false
	}
	d.down = true
	d.log.Warn("sending weekly emails without TMDB", "err", err)
	return false
}
