package core

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrOutsideHousehold = errors.New("kids and guest profiles only see their own household's lists")
	ErrFollowHousehold  = errors.New("only lists from outside your household can be followed")
	ErrFollowersOwner   = errors.New("only a list's owner can see who follows it")
)

type Follower struct {
	Username   string
	Name       string
	FollowedAt time.Time
}

type PublicProfile struct {
	Username string
	Name     string
	Private  bool
	Lists    []List
}

type BrowseQuery struct {
	Popular bool
	Search  string
	Page    Page
}

func goesOutside(kind string) bool {
	return kind != "kids" && kind != "guest"
}

func checkOutside(ctx context.Context, q *store.Queries, profileID int64) error {
	p, err := q.ProfileByID(ctx, profileID)
	if err != nil {
		return err
	}
	if !goesOutside(p.Kind) {
		return ErrOutsideHousehold
	}
	return nil
}

func FollowList(ctx context.Context, db *pgxpool.Pool, profileID, listID int64) (List, error) {
	var out List
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		l, err := getList(ctx, q, profileID, listID)
		if err != nil {
			return err
		}
		if l.Access != "public" {
			return ErrFollowHousehold
		}
		if err := checkOutside(ctx, q, profileID); err != nil {
			return err
		}
		if err := q.FollowList(ctx, store.FollowListParams{ProfileID: profileID, ListID: listID}); err != nil {
			return err
		}
		if err := mark(ctx, q, profileID, "lists.updated_at"); err != nil {
			return err
		}
		out, err = getList(ctx, q, profileID, listID)
		return err
	})
	return out, err
}

func UnfollowList(ctx context.Context, db *pgxpool.Pool, profileID, listID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		n, err := q.UnfollowList(ctx, store.UnfollowListParams{ProfileID: profileID, ListID: listID})
		if err != nil || n == 0 {
			return err
		}
		return mark(ctx, q, profileID, "lists.updated_at")
	})
}

func FollowedLists(ctx context.Context, db *pgxpool.Pool, profileID int64) ([]List, error) {
	q := store.New(db)
	ids, err := q.FollowedListIDs(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []List{}, nil
	}
	lists, err := visibleLists(ctx, q, profileID, store.VisibleListsParams{ListIds: ids})
	if err != nil {
		return nil, err
	}
	inOrder(lists, ids)
	return lists, nil
}

func ListFollowers(ctx context.Context, db *pgxpool.Pool, profileID, listID int64) ([]Follower, error) {
	q := store.New(db)
	l, err := getList(ctx, q, profileID, listID)
	if err != nil {
		return nil, err
	}
	if l.Access != "mine" {
		return nil, ErrFollowersOwner
	}
	rows, err := q.ListFollowers(ctx, listID)
	if err != nil {
		return nil, err
	}
	out := make([]Follower, 0, len(rows))
	for _, r := range rows {
		f := Follower{Username: r.Username, FollowedAt: r.FollowedAt}
		if !r.Private {
			f.Name = r.Name
		}
		out = append(out, f)
	}
	return out, nil
}

func BrowseLists(ctx context.Context, db *pgxpool.Pool, viewer int64, query BrowseQuery) ([]List, int, error) {
	q := store.New(db)
	if err := checkOutside(ctx, q, viewer); err != nil {
		return nil, 0, err
	}
	var search *string
	if s := strings.TrimSpace(query.Search); s != "" {
		s = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
		search = &s
	}
	rows, err := q.BrowseLists(ctx, store.BrowseListsParams{
		Search:  search,
		Popular: query.Popular,
		Lim:     int32(query.Page.Limit),
		Off:     query.Page.offset(),
	})
	if err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return []List{}, 0, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	lists, err := visibleLists(ctx, q, viewer, store.VisibleListsParams{ListIds: ids})
	if err != nil {
		return nil, 0, err
	}
	inOrder(lists, ids)
	return lists, int(rows[0].Total), nil
}

func ProfilePage(ctx context.Context, db *pgxpool.Pool, viewer int64, username string) (PublicProfile, error) {
	q := store.New(db)
	p, err := profileByUsername(ctx, q, username)
	if err != nil {
		return PublicProfile{}, err
	}
	home := false
	if viewer != 0 {
		v, err := q.ProfileByID(ctx, viewer)
		if err != nil {
			return PublicProfile{}, err
		}
		home = v.HouseholdID == p.HouseholdID
		if !home && !goesOutside(v.Kind) {
			return PublicProfile{}, ErrOutsideHousehold
		}
	}
	lists, err := visibleLists(ctx, q, viewer, store.VisibleListsParams{Owner: &p.ID})
	if err != nil {
		return PublicProfile{}, err
	}
	out := PublicProfile{Username: p.Username, Private: p.Private, Lists: lists}
	if home || !p.Private && p.Kind != "kids" {
		out.Name = p.Name
	}
	return out, nil
}

func inOrder(lists []List, ids []int64) {
	slices.SortStableFunc(lists, func(a, b List) int {
		return slices.Index(ids, a.ID) - slices.Index(ids, b.ID)
	})
}
