package core

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

type YearDay struct {
	Plays   int
	Minutes int
}

type YearShow struct {
	Show     Item
	Episodes int
	Minutes  int
	New      bool
}

type YearFilm struct {
	Film      Item
	At        time.Time
	Precision string
	New       bool
	Again     bool
}

type YearRun struct {
	Days int
	From time.Time
}

type BusiestDay struct {
	Date       time.Time
	Plays      int
	Minutes    int
	Title      Item
	TitlePlays int
}

type YearInReview struct {
	Year      int
	Through   time.Time
	Plays     int
	Episodes  int
	Films     int
	Minutes   int
	Unknown   int
	NewShows  int
	Rewatched int
	Days      []YearDay
	Hours     [24]int
	Shows     []YearShow
	FilmsSeen []YearFilm
	Run       YearRun
	Busiest   *BusiestDay
	First     *Entry
	Latest    *Entry
	Years     []int
}

func Year(ctx context.Context, db *pgxpool.Pool, profileID int64, year int, loc *time.Location, now time.Time) (YearInReview, error) {
	start := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
	end := time.Date(year+1, 1, 1, 0, 0, 0, 0, loc)
	if year < 1900 || start.After(now) {
		return YearInReview{}, ErrNotFound
	}
	first := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	through := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	if now.Before(end) {
		local := now.In(loc)
		through = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	}
	y := YearInReview{Year: year, Through: through, Days: make([]YearDay, int(through.Sub(first).Hours()/24)+1)}

	q := store.New(db)
	years, err := q.PlayYears(ctx, store.PlayYearsParams{ProfileID: profileID, Tz: loc.String()})
	if err != nil {
		return y, err
	}
	for _, n := range years {
		y.Years = append(y.Years, int(n))
	}
	// Plays known only by their date sit at noon UTC, which can fall outside the local year at its edges.
	plays, err := q.YearPlays(ctx, store.YearPlaysParams{ProfileID: profileID, StartAt: start.AddDate(0, 0, -1), EndAt: end.AddDate(0, 0, 1)})
	if err != nil {
		return y, err
	}
	var titleIDs, episodeIDs []int64
	for _, p := range plays {
		titleIDs = append(titleIDs, p.TitleID)
		if p.EpisodeID != nil {
			episodeIDs = append(episodeIDs, *p.EpisodeID)
		}
	}
	c, err := loadCatalogue(ctx, q, profileID, titleIDs, episodeIDs)
	if err != nil {
		return y, err
	}

	shows := map[int64]*YearShow{}
	perDay := map[int]map[int64]int{}
	again := map[int64]bool{}
	for _, p := range plays {
		local := p.WatchedAt.In(loc)
		date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		if p.Precision == "day" {
			utc := p.WatchedAt.UTC()
			date = time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
		}
		day := int(date.Sub(first).Hours() / 24)
		if day < 0 || day >= len(y.Days) {
			continue
		}
		title := c.titles[p.TitleID]
		minutes := 0
		switch {
		case p.DurationSeconds != nil && *p.DurationSeconds > 0:
			minutes = int(math.Round(*p.DurationSeconds / 60))
		case title.RuntimeMinutes != nil:
			minutes = int(*title.RuntimeMinutes)
		}
		isNew := !p.FirstSeen.Before(start)
		if minutes == 0 {
			y.Unknown++
		}

		y.Plays++
		y.Minutes += minutes
		y.Days[day].Plays++
		y.Days[day].Minutes += minutes
		if p.Precision == "time" {
			y.Hours[local.Hour()]++
		}
		if p.Again && !again[p.TitleID] {
			again[p.TitleID] = true
			y.Rewatched++
		}
		if perDay[day] == nil {
			perDay[day] = map[int64]int{}
		}
		perDay[day][p.TitleID]++

		entry := &Entry{ID: p.ID, At: p.WatchedAt, Precision: p.Precision, Action: p.Action, Item: c.item(p.TitleID, nil, p.EpisodeID)}
		if y.First == nil {
			y.First = entry
		}
		y.Latest = entry

		if title.Kind == "movie" {
			y.Films++
			y.FilmsSeen = append(y.FilmsSeen, YearFilm{Film: c.item(p.TitleID, nil, nil), At: p.WatchedAt, Precision: p.Precision, New: isNew, Again: p.Again})
			continue
		}
		if p.EpisodeID != nil {
			y.Episodes++
		}
		s := shows[p.TitleID]
		if s == nil {
			s = &YearShow{Show: c.item(p.TitleID, nil, nil), New: isNew}
			shows[p.TitleID] = s
			if isNew {
				y.NewShows++
			}
		}
		if p.EpisodeID != nil {
			s.Episodes++
		}
		s.Minutes += minutes
	}

	for _, s := range shows {
		y.Shows = append(y.Shows, *s)
	}
	slices.SortFunc(y.Shows, func(a, b YearShow) int {
		return cmp.Or(cmp.Compare(b.Episodes, a.Episodes), cmp.Compare(b.Minutes, a.Minutes), cmp.Compare(a.Show.Title.ID, b.Show.Title.ID))
	})

	run := YearRun{}
	for i, d := range y.Days {
		if d.Plays == 0 {
			run = YearRun{}
			continue
		}
		if run.Days == 0 {
			run.From = first.AddDate(0, 0, i)
		}
		run.Days++
		if run.Days > y.Run.Days {
			y.Run = run
		}
	}

	busiest := -1
	for i, d := range y.Days {
		if d.Plays > 0 && (busiest < 0 || d.Plays > y.Days[busiest].Plays || d.Plays == y.Days[busiest].Plays && d.Minutes > y.Days[busiest].Minutes) {
			busiest = i
		}
	}
	if busiest >= 0 {
		b := &BusiestDay{Date: first.AddDate(0, 0, busiest), Plays: y.Days[busiest].Plays, Minutes: y.Days[busiest].Minutes}
		var top int64
		for id, n := range perDay[busiest] {
			if n > b.TitlePlays || n == b.TitlePlays && id < top {
				top, b.TitlePlays = id, n
			}
		}
		b.Title = c.item(top, nil, nil)
		y.Busiest = b
	}
	return y, nil
}
