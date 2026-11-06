package core

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/store"
)

var (
	ErrListName        = errors.New("a list needs a name of up to 100 characters")
	ErrListDescription = errors.New("a list's description is up to 1,000 characters")
	ErrListNotes       = errors.New("notes are up to 500 characters")
	ErrListPrivacy     = errors.New("lists are private, household or public")
	ErrListSort        = errors.New("lists sort by rank, added, title or year, asc or desc")
	ErrPublicList      = errors.New("kids and guest profiles can't make public lists")
	ErrTooManyLists    = errors.New("a profile can have up to 100 lists")
	ErrListFull        = errors.New("a list holds up to 1,000 items")
	ErrListOrder       = errors.New("an order names every item in the list once")
)

const (
	maxLists     = 100
	maxListItems = 1000
)

type ListOwner struct {
	ID       int64
	Username string
	Name     string
}

type List struct {
	ID          int64
	Owner       ListOwner
	Name        string
	Slug        string
	Description *string
	Privacy     string
	SortBy      string
	SortHow     string
	Access      string
	ItemCount   int64
	Watched     *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     string
}

type ListChanges struct {
	Name        *string
	Description *string
	Privacy     *string
	SortBy      *string
	SortHow     *string
}

type ListQuery struct {
	Kind        string
	SortBy      string
	SortHow     string
	HideWatched bool
	Page        Page
}

type ListEntry struct {
	Entry
	Progress *ItemProgress
}

type ItemProgress struct {
	Watched       int
	Total         int
	LastWatchedAt *time.Time
}

func (p ItemProgress) Done() bool {
	return p.Total > 0 && p.Watched >= p.Total
}

func Lists(ctx context.Context, db *pgxpool.Pool, viewer int64) ([]List, error) {
	return visibleLists(ctx, store.New(db), viewer, store.VisibleListsParams{})
}

func ProfileLists(ctx context.Context, db *pgxpool.Pool, viewer int64, username string) ([]List, error) {
	q := store.New(db)
	owner, err := q.ProfileByUsername(ctx, username)
	if err != nil {
		return nil, notFound(err)
	}
	return visibleLists(ctx, q, viewer, store.VisibleListsParams{Owner: &owner.ID})
}

func GetList(ctx context.Context, db *pgxpool.Pool, viewer, id int64) (List, error) {
	return getList(ctx, store.New(db), viewer, id)
}

func getList(ctx context.Context, q *store.Queries, viewer, id int64) (List, error) {
	lists, err := visibleLists(ctx, q, viewer, store.VisibleListsParams{ListID: &id})
	if err != nil {
		return List{}, err
	}
	if len(lists) == 0 {
		return List{}, ErrNotFound
	}
	return lists[0], nil
}

func visibleLists(ctx context.Context, q *store.Queries, viewer int64, params store.VisibleListsParams) ([]List, error) {
	params.Viewer = viewer
	rows, err := q.VisibleLists(ctx, params)
	if err != nil {
		return nil, err
	}
	var latest time.Time
	if viewer != 0 && len(rows) > 0 {
		if latest, err = q.LatestActivity(ctx, viewer); err != nil {
			return nil, err
		}
	}
	lists := make([]List, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		l := row.List
		version := sha256.Sum256(fmt.Appendf(nil, "%d %d %d", l.UpdatedAt.UnixMicro(), row.TitlesCheckedAt.UnixMicro(), latest.UnixMicro()))
		lists = append(lists, List{
			ID:          l.ID,
			Owner:       ListOwner{ID: l.ProfileID, Username: row.OwnerUsername, Name: row.OwnerName},
			Name:        l.Name,
			Slug:        l.Slug,
			Description: l.Description,
			Privacy:     l.Privacy,
			SortBy:      l.SortBy,
			SortHow:     l.SortHow,
			Access:      row.Access,
			ItemCount:   row.ItemCount,
			CreatedAt:   l.CreatedAt,
			UpdatedAt:   l.UpdatedAt,
			Version:     hex.EncodeToString(version[:12]),
		})
		ids = append(ids, l.ID)
	}
	if viewer == 0 || len(ids) == 0 {
		return lists, nil
	}
	items, err := q.ListItemsIn(ctx, ids)
	if err != nil {
		return nil, err
	}
	progress, err := itemProgress(ctx, q, viewer, items)
	if err != nil {
		return nil, err
	}
	watched := map[int64]int64{}
	for _, item := range items {
		if progress[item.ID].Done() {
			watched[item.ListID]++
		}
	}
	for i := range lists {
		n := watched[lists[i].ID]
		lists[i].Watched = &n
	}
	return lists, nil
}

func ListEntries(ctx context.Context, db *pgxpool.Pool, viewer, id int64, query ListQuery) (List, []ListEntry, int, error) {
	q := store.New(db)
	list, err := getList(ctx, q, viewer, id)
	if err != nil {
		return List{}, nil, 0, err
	}
	rows, err := q.ListItemsIn(ctx, []int64{id})
	if err != nil {
		return List{}, nil, 0, err
	}
	progress, err := itemProgress(ctx, q, viewer, rows)
	if err != nil {
		return List{}, nil, 0, err
	}
	var titleIDs, episodeIDs []int64
	for _, row := range rows {
		titleIDs = append(titleIDs, row.TitleID)
		if row.EpisodeID != nil {
			episodeIDs = append(episodeIDs, *row.EpisodeID)
		}
	}
	c, err := loadCatalogue(ctx, q, viewer, titleIDs, episodeIDs)
	if err != nil {
		return List{}, nil, 0, err
	}
	entries := make([]ListEntry, 0, len(rows))
	for _, row := range rows {
		e := ListEntry{Entry: Entry{ID: row.ID, At: row.ListedAt, Rank: row.Rank, Notes: row.Notes, Item: c.item(row.TitleID, row.SeasonNumber, row.EpisodeID)}}
		if query.Kind != "" && e.Item.Kind() != query.Kind {
			continue
		}
		if p, ok := progress[row.ID]; ok {
			if query.HideWatched && p.Done() {
				continue
			}
			e.Progress = &p
		}
		entries = append(entries, e)
	}
	sortEntries(entries, cmp.Or(query.SortBy, list.SortBy), cmp.Or(query.SortHow, list.SortHow))
	total := len(entries)
	if query.Page.Limit > 0 {
		start := min((max(query.Page.Page, 1)-1)*query.Page.Limit, total)
		entries = entries[start:min(start+query.Page.Limit, total)]
	}
	return list, entries, total, nil
}

func sortEntries(entries []ListEntry, by, how string) {
	key := func(e ListEntry) (string, bool) {
		switch by {
		case "title":
			if e.Item.Title.Name == nil {
				return "", false
			}
			return strings.ToLower(*e.Item.Title.Name), true
		case "year":
			if e.Item.Title.Year == nil {
				return "", false
			}
			return fmt.Sprintf("%05d", *e.Item.Title.Year), true
		case "added":
			return e.At.UTC().Format(time.RFC3339Nano), true
		}
		return fmt.Sprintf("%010d", e.Rank), true
	}
	slices.SortStableFunc(entries, func(a, b ListEntry) int {
		ka, okA := key(a)
		kb, okB := key(b)
		if okA != okB {
			if okA {
				return -1
			}
			return 1
		}
		order := cmp.Or(strings.Compare(ka, kb), cmp.Compare(a.Rank, b.Rank), cmp.Compare(a.ID, b.ID))
		if how == "desc" {
			return -order
		}
		return order
	})
}

func itemProgress(ctx context.Context, q *store.Queries, viewer int64, items []store.ListItemsInRow) (map[int64]ItemProgress, error) {
	if viewer == 0 || len(items) == 0 {
		return nil, nil
	}
	var titleIDs, showIDs []int64
	for _, item := range items {
		titleIDs = append(titleIDs, item.TitleID)
		if item.TitleKind == "show" && item.EpisodeID == nil {
			showIDs = append(showIDs, item.TitleID)
		}
	}
	watches, err := q.WatchedInTitles(ctx, store.WatchedInTitlesParams{ProfileID: viewer, TitleIds: titleIDs})
	if err != nil {
		return nil, err
	}
	films := map[int64]time.Time{}
	episodes := map[int64]time.Time{}
	for _, w := range watches {
		if w.EpisodeID == nil {
			films[w.TitleID] = w.LastWatchedAt
		} else {
			episodes[*w.EpisodeID] = w.LastWatchedAt
		}
	}
	shows, err := showEpisodes(ctx, q, viewer, showIDs)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make(map[int64]ItemProgress, len(items))
	for _, item := range items {
		p := ItemProgress{Total: 1}
		count := func(at time.Time, ok bool) {
			if !ok {
				return
			}
			p.Watched++
			if p.LastWatchedAt == nil || at.After(*p.LastWatchedAt) {
				p.LastWatchedAt = &at
			}
		}
		switch {
		case item.EpisodeID != nil:
			at, ok := episodes[*item.EpisodeID]
			count(at, ok)
		case item.TitleKind == "movie":
			at, ok := films[item.TitleID]
			count(at, ok)
		default:
			p.Total = 0
			for _, e := range shows[item.TitleID] {
				if e.FirstAired == nil || e.FirstAired.After(now) {
					continue
				}
				if item.SeasonNumber == nil && e.Season == 0 || item.SeasonNumber != nil && e.SeasonNumber != *item.SeasonNumber {
					continue
				}
				p.Total++
				at, ok := episodes[e.ID]
				count(at, ok)
			}
		}
		out[item.ID] = p
	}
	return out, nil
}

func CreateList(ctx context.Context, db *pgxpool.Pool, profileID int64, c ListChanges) (List, error) {
	l := store.List{Privacy: "private", SortBy: "rank", SortHow: "asc"}
	if err := c.apply(&l); err != nil {
		return List{}, err
	}
	var out List
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := checkPrivacy(ctx, q, profileID, l.Privacy); err != nil {
			return err
		}
		n, err := q.CountLists(ctx, profileID)
		if err != nil {
			return err
		}
		if n >= maxLists {
			return ErrTooManyLists
		}
		slug, err := listSlug(ctx, q, profileID, 0, l.Name)
		if err != nil {
			return err
		}
		created, err := q.CreateList(ctx, store.CreateListParams{
			ProfileID:   profileID,
			Name:        l.Name,
			Slug:        slug,
			Description: l.Description,
			Privacy:     l.Privacy,
			SortBy:      l.SortBy,
			SortHow:     l.SortHow,
		})
		if err != nil {
			return err
		}
		if err := markLists(ctx, q, profileID); err != nil {
			return err
		}
		out, err = getList(ctx, q, profileID, created.ID)
		return err
	})
	return out, err
}

func UpdateList(ctx context.Context, db *pgxpool.Pool, profileID, id int64, c ListChanges) (List, error) {
	err := changeList(ctx, db, profileID, id, func(q *store.Queries, l store.List) error {
		name := l.Name
		if err := c.apply(&l); err != nil {
			return err
		}
		if err := checkPrivacy(ctx, q, profileID, l.Privacy); err != nil {
			return err
		}
		if l.Name != name {
			slug, err := listSlug(ctx, q, profileID, l.ID, l.Name)
			if err != nil {
				return err
			}
			l.Slug = slug
		}
		_, err := q.UpdateList(ctx, store.UpdateListParams{
			ID:          l.ID,
			Name:        l.Name,
			Slug:        l.Slug,
			Description: l.Description,
			Privacy:     l.Privacy,
			SortBy:      l.SortBy,
			SortHow:     l.SortHow,
		})
		return err
	})
	if err != nil {
		return List{}, err
	}
	return GetList(ctx, db, profileID, id)
}

func DeleteList(ctx context.Context, db *pgxpool.Pool, profileID, id int64) error {
	return changeList(ctx, db, profileID, id, func(q *store.Queries, l store.List) error {
		return q.DeleteList(ctx, l.ID)
	})
}

func AddToList(ctx context.Context, db *pgxpool.Pool, profileID, id int64, items []Listing) (SyncResult, error) {
	for _, l := range items {
		if utf8.RuneCountInString(strings.TrimSpace(l.Notes)) > 500 {
			return SyncResult{}, ErrListNotes
		}
	}
	var result SyncResult
	err := changeList(ctx, db, profileID, id, func(q *store.Queries, list store.List) error {
		for _, l := range items {
			r, err := resolve(ctx, q, l.Ref, true)
			if errors.Is(err, ErrNotFound) {
				result.NotFound = append(result.NotFound, l.Ref)
				continue
			}
			if err != nil {
				return err
			}
			_, err = q.AddListItem(ctx, store.AddListItemParams{
				ListID:       list.ID,
				TitleID:      r.title.ID,
				SeasonNumber: r.season,
				EpisodeID:    r.episodeID(),
				Notes:        optional(strings.TrimSpace(l.Notes)),
				ListedAt:     l.At,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				result.Existing.add(l.Ref.Kind)
				continue
			}
			if err != nil {
				return err
			}
			result.Added.add(l.Ref.Kind)
		}
		var err error
		if result.Listed, err = q.CountListItems(ctx, list.ID); err != nil {
			return err
		}
		if result.Listed > maxListItems {
			return ErrListFull
		}
		return nil
	})
	return result, err
}

func RemoveFromList(ctx context.Context, db *pgxpool.Pool, profileID, listID, itemID int64) error {
	return changeList(ctx, db, profileID, listID, func(q *store.Queries, l store.List) error {
		n, err := q.RemoveListEntry(ctx, store.RemoveListEntryParams{ID: itemID, ListID: l.ID})
		if err == nil && n == 0 {
			return ErrNotFound
		}
		return err
	})
}

func SetListNotes(ctx context.Context, db *pgxpool.Pool, profileID, listID, itemID int64, notes string) error {
	notes = strings.TrimSpace(notes)
	if utf8.RuneCountInString(notes) > 500 {
		return ErrListNotes
	}
	return changeList(ctx, db, profileID, listID, func(q *store.Queries, l store.List) error {
		n, err := q.SetListItemNotes(ctx, store.SetListItemNotesParams{ID: itemID, ListID: l.ID, Notes: optional(notes)})
		if err == nil && n == 0 {
			return ErrNotFound
		}
		return err
	})
}

func ReorderList(ctx context.Context, db *pgxpool.Pool, profileID, listID int64, order []int64) error {
	return changeList(ctx, db, profileID, listID, func(q *store.Queries, l store.List) error {
		ids, err := q.ListEntryIDs(ctx, l.ID)
		if err != nil {
			return err
		}
		sorted := slices.Clone(order)
		slices.Sort(sorted)
		slices.Sort(ids)
		if !slices.Equal(sorted, ids) {
			return ErrListOrder
		}
		return q.RankListItems(ctx, store.RankListItemsParams{Ids: order, ListID: l.ID})
	})
}

func changeList(ctx context.Context, db *pgxpool.Pool, profileID, id int64, change func(*store.Queries, store.List) error) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		l, err := q.OwnedList(ctx, store.OwnedListParams{ID: id, ProfileID: profileID})
		if err != nil {
			return notFound(err)
		}
		if err := change(q, l); err != nil {
			return err
		}
		if _, err := q.TouchList(ctx, l.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return markLists(ctx, q, profileID)
	})
}

func (c ListChanges) apply(l *store.List) error {
	if c.Name != nil {
		l.Name = strings.TrimSpace(*c.Name)
	}
	if n := utf8.RuneCountInString(l.Name); n == 0 || n > 100 {
		return ErrListName
	}
	if c.Description != nil {
		l.Description = optional(strings.TrimSpace(*c.Description))
	}
	if l.Description != nil && utf8.RuneCountInString(*l.Description) > 1000 {
		return ErrListDescription
	}
	if c.Privacy != nil {
		l.Privacy = *c.Privacy
	}
	if !slices.Contains([]string{"private", "household", "public"}, l.Privacy) {
		return ErrListPrivacy
	}
	if c.SortBy != nil {
		l.SortBy = *c.SortBy
	}
	if c.SortHow != nil {
		l.SortHow = *c.SortHow
	}
	if !slices.Contains([]string{"rank", "added", "title", "year"}, l.SortBy) || l.SortHow != "asc" && l.SortHow != "desc" {
		return ErrListSort
	}
	return nil
}

func checkPrivacy(ctx context.Context, q *store.Queries, profileID int64, privacy string) error {
	if privacy != "public" {
		return nil
	}
	p, err := q.ProfileByID(ctx, profileID)
	if err != nil {
		return err
	}
	if p.Kind == "kids" || p.Kind == "guest" {
		return ErrPublicList
	}
	return nil
}

func listSlug(ctx context.Context, q *store.Queries, profileID, listID int64, name string) (string, error) {
	base := slugify(name)
	if len(base) > 80 {
		base = strings.TrimRight(base[:80], "-")
	}
	if base == "" {
		base = "list"
	}
	taken, err := q.ListSlugs(ctx, store.ListSlugsParams{ProfileID: profileID, ExceptID: listID, Slug: base})
	if err != nil {
		return "", err
	}
	slug := base
	for n := 2; slug == "watchlist" || slices.Contains(taken, slug); n++ {
		slug = base + "-" + strconv.Itoa(n)
	}
	return slug, nil
}

func markLists(ctx context.Context, q *store.Queries, profileID int64) error {
	household, err := q.HouseholdOf(ctx, profileID)
	if err != nil {
		return err
	}
	for _, id := range household {
		if err := mark(ctx, q, id, "lists.updated_at"); err != nil {
			return err
		}
	}
	return nil
}
