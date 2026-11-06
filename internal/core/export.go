package core

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrSignInAgain    = errors.New("sign in again")
	ErrTooManyExports = errors.New("too many exports")
)

const (
	exportFreshness = 10 * time.Minute
	exportsPerDay   = 2
)

var hiddenSections = []string{"calendar", "progress_watched", "progress_watched_reset", "progress_collected", "recommendations", "comments", "dropped"}

type ExportStart struct {
	AccountID int64
	Email     string
	Device    string
	At        time.Time
}

type HiddenEntry struct {
	Section string
	Entry
}

type ProfileExport struct {
	Profile   store.Profile
	History   []Entry
	Playback  []Entry
	Ratings   []Entry
	Watchlist []Entry
	Lists     []ListExport
	Hidden    []HiddenEntry
	Imports   []Import
}

type ListExport struct {
	List    List
	Entries []ListEntry
}

type HouseholdExport struct {
	Account      store.Account
	Passkeys     []store.Passkey
	Apps         []store.AccountAppsRow
	Connections  []store.HouseholdGrantsRow
	MediaServers []MediaServer
	Profiles     []ProfileExport
}

func StartExport(ctx context.Context, db *pgxpool.Pool, token string) (ExportStart, error) {
	session, err := currentSession(ctx, db, token)
	if err != nil {
		return ExportStart{}, err
	}
	if time.Since(session.CreatedAt) > exportFreshness {
		return ExportStart{}, ErrSignInAgain
	}
	start := ExportStart{AccountID: session.AccountID, At: time.Now()}
	if session.UserAgent != nil {
		start.Device = *session.UserAgent
	}
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockAccount(ctx, start.AccountID); err != nil {
			return err
		}
		recent, err := q.CountAudit(ctx, store.CountAuditParams{AccountID: start.AccountID, Kind: "export", CreatedAt: start.At.Add(-24 * time.Hour)})
		if err != nil {
			return err
		}
		if recent >= exportsPerDay {
			return ErrTooManyExports
		}
		account, err := q.AccountByID(ctx, start.AccountID)
		if err != nil {
			return err
		}
		start.Email = account.Email
		return audit(ctx, q, start.AccountID, "export", start.Device)
	})
	return start, err
}

func Export(ctx context.Context, db *pgxpool.Pool, accountID int64) (HouseholdExport, error) {
	var out HouseholdExport
	account, profiles, err := Me(ctx, db, accountID)
	if err != nil {
		return out, err
	}
	out.Account = account
	if out.Passkeys, err = ListPasskeys(ctx, db, accountID); err != nil {
		return out, err
	}
	if out.Apps, err = ListApps(ctx, db, accountID); err != nil {
		return out, err
	}
	if out.Connections, err = ListGrants(ctx, db, accountID); err != nil {
		return out, err
	}
	if out.MediaServers, err = MediaServers(ctx, db, accountID); err != nil {
		return out, err
	}
	all := Page{Page: 1, Limit: math.MaxInt32}
	for _, p := range profiles {
		e := ProfileExport{Profile: p}
		if e.History, _, err = History(ctx, db, p.ID, HistoryFilter{}, all); err != nil {
			return out, err
		}
		if e.Playback, err = Playback(ctx, db, p.ID, "", Range{}, all); err != nil {
			return out, err
		}
		if e.Ratings, err = Ratings(ctx, db, p.ID, "", nil); err != nil {
			return out, err
		}
		if e.Watchlist, err = Watchlist(ctx, db, p.ID, ""); err != nil {
			return out, err
		}
		lists, err := visibleLists(ctx, store.New(db), p.ID, store.VisibleListsParams{Owner: &p.ID})
		if err != nil {
			return out, err
		}
		for _, l := range lists {
			_, entries, _, err := ListEntries(ctx, db, p.ID, l.ID, ListQuery{SortBy: "rank", SortHow: "asc"})
			if err != nil {
				return out, err
			}
			e.Lists = append(e.Lists, ListExport{List: l, Entries: entries})
		}
		for _, section := range hiddenSections {
			hidden, err := Hidden(ctx, db, p.ID, section, "")
			if err != nil {
				return out, err
			}
			for _, h := range hidden {
				e.Hidden = append(e.Hidden, HiddenEntry{Section: section, Entry: h})
			}
		}
		if e.Imports, err = Imports(ctx, db, p.ID); err != nil {
			return out, err
		}
		out.Profiles = append(out.Profiles, e)
	}
	return out, nil
}
