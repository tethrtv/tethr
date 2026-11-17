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
	ErrListCover       = errors.New("a list's cover is one of its own items")
	ErrListReadOnly    = errors.New("the list is shared with this profile to read, not to change")
)

const (
	maxLists     = 100
	maxListItems = 1000
	coverItems   = 4
)

type ListOwner struct {
	ID       int64
	Username string
	Name     string
	Private  bool
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
	CanEdit     bool
	InUpNext    bool
	Following   bool
	Followers   int64
	ItemCount   int64
	Matched     int64
	Rule        *Rule
	RuleNames   map[string]string
	Watched     *int64
	CoverItemID *int64
	Cover       []Item
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
	Cover       *int64
	Rule        *Rule
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
	Next          *OrderedEpisode
}

func (p ItemProgress) Done() bool {
	return p.Total > 0 && p.Watched >= p.Total
}

func (l List) Path() string {
	return "/@" + l.Owner.Username + "/lists/" + l.Slug
}

func Lists(ctx context.Context, db *pgxpool.Pool, viewer int64) ([]List, error) {
	return visibleLists(ctx, store.New(db), viewer, store.VisibleListsParams{})
}

func ProfileLists(ctx context.Context, db *pgxpool.Pool, viewer int64, username string) ([]List, error) {
	q := store.New(db)
	owner, err := profileByUsername(ctx, q, username)
	if err != nil {
		return nil, err
	}
	return visibleLists(ctx, q, viewer, store.VisibleListsParams{Owner: &owner.ID})
}

func FindList(ctx context.Context, db *pgxpool.Pool, viewer int64, username, slug string) (List, error) {
	q := store.New(db)
	owner, err := profileByUsername(ctx, q, username)
	if err != nil {
		return List{}, err
	}
	id, err := q.ListIDBySlug(ctx, store.ListIDBySlugParams{ProfileID: owner.ID, Slug: strings.ToLower(slug)})
	if err != nil {
		return List{}, notFound(err)
	}
	return getList(ctx, q, viewer, id)
}

func profileByUsername(ctx context.Context, q *store.Queries, username string) (store.Profile, error) {
	p, err := q.ProfileByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		p, err = q.ProfileByOldUsername(ctx, username)
	}
	return p, notFound(err)
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
	return loadLists(ctx, q, viewer, params, true)
}

func loadLists(ctx context.Context, q *store.Queries, viewer int64, params store.VisibleListsParams, hide bool) ([]List, error) {
	lists, _, err := loadListItems(ctx, q, viewer, params, hide)
	return lists, err
}

func loadListItems(ctx context.Context, q *store.Queries, viewer int64, params store.VisibleListsParams, hide bool) ([]List, []store.ListItemsInRow, error) {
	var viewerKind string
	if viewer != 0 {
		p, err := q.ProfileByID(ctx, viewer)
		if err != nil {
			return nil, nil, err
		}
		viewerKind = p.Kind
	}
	params.Viewer = viewer
	params.ForKids = hide && viewerKind == "kids"
	rows, err := q.VisibleLists(ctx, params)
	if err != nil {
		return nil, nil, err
	}
	var latest time.Time
	if viewer != 0 && len(rows) > 0 {
		if latest, err = q.LatestActivity(ctx, viewer); err != nil {
			return nil, nil, err
		}
	}
	lists := make([]List, 0, len(rows))
	versions := make([]string, 0, len(rows))
	for _, row := range rows {
		l := row.List
		rule, err := readRule(l.Rule)
		if err != nil {
			return nil, nil, err
		}
		versions = append(versions, fmt.Sprintf("%d %d %d %s %d", l.UpdatedAt.UnixMicro(), row.TitlesCheckedAt.UnixMicro(), latest.UnixMicro(), viewerKind, row.ItemCount))
		owner := ListOwner{ID: l.ProfileID, Username: row.OwnerUsername, Name: row.OwnerName, Private: row.OwnerPrivate}
		if row.Access == "public" && owner.Private {
			owner.Name = ""
		}
		lists = append(lists, List{
			ID:          l.ID,
			Owner:       owner,
			Name:        l.Name,
			Slug:        l.Slug,
			Description: l.Description,
			Privacy:     l.Privacy,
			SortBy:      l.SortBy,
			SortHow:     l.SortHow,
			Access:      row.Access,
			CanEdit:     canEditItems(row.Access, l.Privacy, viewerKind),
			InUpNext:    row.InUpNext,
			Following:   row.Following,
			Followers:   row.FollowerCount,
			ItemCount:   row.ItemCount,
			Rule:        rule,
			CoverItemID: l.CoverItemID,
			CreatedAt:   l.CreatedAt,
			UpdatedAt:   l.UpdatedAt,
		})
	}
	if len(lists) == 0 {
		return lists, nil, nil
	}
	items, matched, err := listItemRows(ctx, q, viewer, lists, params.ForKids)
	if err != nil {
		return nil, nil, err
	}
	var rules []Rule
	for _, l := range lists {
		if l.Rule != nil {
			rules = append(rules, *l.Rule)
		}
	}
	names, err := companyNames(ctx, q, rules)
	if err != nil {
		return nil, nil, err
	}
	byList := map[int64][]store.ListItemsInRow{}
	for _, item := range items {
		byList[item.ListID] = append(byList[item.ListID], item)
	}
	for i := range lists {
		v := versions[i]
		if lists[i].Rule != nil {
			lists[i].ItemCount = int64(len(byList[lists[i].ID]))
			lists[i].Matched = matched[lists[i].ID]
			lists[i].RuleNames = names
			v += " " + ruleVersion(byList[lists[i].ID])
		}
		sum := sha256.Sum256([]byte(v))
		lists[i].Version = hex.EncodeToString(sum[:12])
	}
	if err := addCovers(ctx, q, viewer, lists, items); err != nil {
		return nil, nil, err
	}
	if viewer == 0 {
		return lists, items, nil
	}
	progress, err := itemProgress(ctx, q, viewer, items)
	if err != nil {
		return nil, nil, err
	}
	watched := map[int64]int64{}
	for _, item := range items {
		if progress[keyOf(item)].Done() {
			watched[item.ListID]++
		}
	}
	for i := range lists {
		n := watched[lists[i].ID]
		lists[i].Watched = &n
	}
	return lists, items, nil
}

func addCovers(ctx context.Context, q *store.Queries, viewer int64, lists []List, items []store.ListItemsInRow) error {
	listed := map[itemKey]bool{}
	for _, item := range items {
		listed[keyOf(item)] = true
	}
	picked := map[int64]int64{}
	for _, l := range lists {
		if l.CoverItemID != nil && listed[itemKey{l.ID, *l.CoverItemID}] {
			picked[l.ID] = *l.CoverItemID
		}
	}
	covers := map[int64][]store.ListItemsInRow{}
	var titleIDs, episodeIDs []int64
	for _, item := range items {
		id, ok := picked[item.ListID]
		if ok && item.ID != id || !ok && len(covers[item.ListID]) == coverItems {
			continue
		}
		covers[item.ListID] = append(covers[item.ListID], item)
		titleIDs = append(titleIDs, item.TitleID)
		if item.EpisodeID != nil {
			episodeIDs = append(episodeIDs, *item.EpisodeID)
		}
	}
	if len(titleIDs) == 0 {
		return nil
	}
	c, err := loadCatalogue(ctx, q, viewer, titleIDs, episodeIDs)
	if err != nil {
		return err
	}
	for i := range lists {
		for _, item := range covers[lists[i].ID] {
			lists[i].Cover = append(lists[i].Cover, c.item(item.TitleID, seasonOf(item.TitleID, item.SeasonID, item.SeasonNumber), item.EpisodeID))
		}
	}
	return nil
}

func ListEntries(ctx context.Context, db *pgxpool.Pool, viewer, id int64, query ListQuery) (List, []ListEntry, int, error) {
	return listEntries(ctx, db, viewer, id, query, true)
}

func listEntries(ctx context.Context, db *pgxpool.Pool, viewer, id int64, query ListQuery, hide bool) (List, []ListEntry, int, error) {
	q := store.New(db)
	lists, rows, err := loadListItems(ctx, q, viewer, store.VisibleListsParams{ListID: &id}, hide)
	if err != nil {
		return List{}, nil, 0, err
	}
	if len(lists) == 0 {
		return List{}, nil, 0, ErrNotFound
	}
	list := lists[0]
	if hide && list.Rule == nil {
		kids, err := isKids(ctx, q, viewer)
		if err != nil {
			return List{}, nil, 0, err
		}
		if kids {
			for n := range rows {
				rows[n].Rank = int32(n + 1)
			}
		}
	}
	entries, total, err := entriesFor(ctx, q, viewer, rows, query, list.SortBy, list.SortHow)
	return list, entries, total, err
}

func entriesFor(ctx context.Context, q *store.Queries, viewer int64, rows []store.ListItemsInRow, query ListQuery, sortBy, sortHow string) ([]ListEntry, int, error) {
	progress, err := itemProgress(ctx, q, viewer, rows)
	if err != nil {
		return nil, 0, err
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
		return nil, 0, err
	}
	entries := make([]ListEntry, 0, len(rows))
	for _, row := range rows {
		e := ListEntry{Entry: Entry{ID: row.ID, At: row.ListedAt, Rank: row.Rank, Notes: row.Notes, Item: c.item(row.TitleID, seasonOf(row.TitleID, row.SeasonID, row.SeasonNumber), row.EpisodeID)}}
		if query.Kind != "" && e.Item.Kind() != query.Kind {
			continue
		}
		if p, ok := progress[keyOf(row)]; ok {
			if query.HideWatched && p.Done() {
				continue
			}
			e.Progress = &p
		}
		entries = append(entries, e)
	}
	sortEntries(entries, cmp.Or(query.SortBy, sortBy), cmp.Or(query.SortHow, sortHow))
	total := len(entries)
	if query.Page.Limit > 0 {
		start := min((max(query.Page.Page, 1)-1)*query.Page.Limit, total)
		entries = entries[start:min(start+query.Page.Limit, total)]
	}
	return entries, total, nil
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

type itemKey struct{ list, id int64 }

func keyOf(row store.ListItemsInRow) itemKey { return itemKey{row.ListID, row.ID} }

func itemProgress(ctx context.Context, q *store.Queries, viewer int64, items []store.ListItemsInRow) (map[itemKey]ItemProgress, error) {
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
	out := make(map[itemKey]ItemProgress, len(items))
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
			var gap, afterLast *OrderedEpisode
			for _, e := range shows[item.TitleID] {
				if e.FirstAired == nil || e.FirstAired.After(now) {
					continue
				}
				if item.SeasonNumber == nil && e.Season == 0 || item.SeasonNumber != nil && e.SeasonNumber != *item.SeasonNumber {
					continue
				}
				p.Total++
				at, ok := episodes[e.ID]
				switch {
				case ok && (p.LastWatchedAt == nil || !at.Before(*p.LastWatchedAt)):
					afterLast = nil
				case !ok:
					gap = cmp.Or(gap, &e)
					if afterLast == nil && p.LastWatchedAt != nil {
						afterLast = &e
					}
				}
				count(at, ok)
			}
			p.Next = cmp.Or(afterLast, gap)
		}
		out[keyOf(item)] = p
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
		id, err := createList(ctx, q, profileID, l)
		if err != nil {
			return err
		}
		out, err = getList(ctx, q, profileID, id)
		return err
	})
	return out, err
}

func createList(ctx context.Context, q *store.Queries, profileID int64, l store.List) (int64, error) {
	if err := checkPrivacy(ctx, q, profileID, l.Privacy); err != nil {
		return 0, err
	}
	n, err := q.CountLists(ctx, profileID)
	if err != nil {
		return 0, err
	}
	if n >= maxLists {
		return 0, ErrTooManyLists
	}
	slug, err := listSlug(ctx, q, profileID, 0, l.Name)
	if err != nil {
		return 0, err
	}
	created, err := q.CreateList(ctx, store.CreateListParams{
		ProfileID:   profileID,
		Name:        l.Name,
		Slug:        slug,
		Description: l.Description,
		Privacy:     l.Privacy,
		SortBy:      l.SortBy,
		SortHow:     l.SortHow,
		Rule:        l.Rule,
	})
	if err != nil {
		return 0, err
	}
	return created.ID, markLists(ctx, q, profileID)
}

func UpdateList(ctx context.Context, db *pgxpool.Pool, profileID, id int64, c ListChanges) (List, error) {
	err := changeList(ctx, db, profileID, id, func(q *store.Queries, l store.List) error {
		name := l.Name
		if c.Rule != nil && l.Rule == nil || c.Cover != nil && l.Rule != nil {
			return ErrRuleList
		}
		if c.Rule != nil && c.Rule.LeftOut == nil {
			stored, err := readRule(l.Rule)
			if err != nil {
				return err
			}
			kept := *c.Rule
			kept.LeftOut = stored.LeftOut
			c.Rule = &kept
		}
		if err := c.apply(&l); err != nil {
			return err
		}
		if err := checkPrivacy(ctx, q, profileID, l.Privacy); err != nil {
			return err
		}
		if c.Cover != nil {
			l.CoverItemID = nil
			if *c.Cover != 0 {
				ok, err := q.OnList(ctx, store.OnListParams{ID: *c.Cover, ListID: l.ID})
				if err != nil {
					return err
				}
				if !ok {
					return ErrListCover
				}
				l.CoverItemID = c.Cover
			}
		}
		if l.Name != name {
			slug, err := listSlug(ctx, q, profileID, l.ID, l.Name)
			if err != nil {
				return err
			}
			if slug != l.Slug {
				if err := q.KeepOldListSlug(ctx, store.KeepOldListSlugParams{ProfileID: l.ProfileID, Slug: l.Slug, ListID: l.ID}); err != nil {
					return err
				}
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
			CoverItemID: l.CoverItemID,
			Rule:        l.Rule,
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
	err := changeItems(ctx, db, profileID, id, func(q *store.Queries, list store.List) error {
		if list.Rule != nil {
			return ErrRuleList
		}
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
				ListID:    list.ID,
				TitleID:   r.title.ID,
				SeasonID:  r.seasonID(),
				EpisodeID: r.episodeID(),
				Notes:     optional(strings.TrimSpace(l.Notes)),
				ListedAt:  l.At,
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
	return changeItems(ctx, db, profileID, listID, func(q *store.Queries, l store.List) error {
		if l.Rule != nil {
			n, err := leaveOut(ctx, q, profileID, l, []int64{itemID})
			if err == nil && n == 0 {
				return ErrNotFound
			}
			return err
		}
		n, err := q.RemoveListEntry(ctx, store.RemoveListEntryParams{ID: itemID, ListID: l.ID})
		if err == nil && n == 0 {
			return ErrNotFound
		}
		return err
	})
}

func RemoveManyFromList(ctx context.Context, db *pgxpool.Pool, profileID, listID int64, itemIDs []int64) (int64, error) {
	var removed int64
	err := changeItems(ctx, db, profileID, listID, func(q *store.Queries, l store.List) error {
		var err error
		if l.Rule != nil {
			removed, err = leaveOut(ctx, q, profileID, l, itemIDs)
			return err
		}
		removed, err = q.RemoveListEntries(ctx, store.RemoveListEntriesParams{ListID: l.ID, Ids: itemIDs})
		return err
	})
	return removed, err
}

func SetListNotes(ctx context.Context, db *pgxpool.Pool, profileID, listID, itemID int64, notes string) error {
	notes = strings.TrimSpace(notes)
	if utf8.RuneCountInString(notes) > 500 {
		return ErrListNotes
	}
	return changeItems(ctx, db, profileID, listID, func(q *store.Queries, l store.List) error {
		if l.Rule != nil {
			return ErrRuleList
		}
		n, err := q.SetListItemNotes(ctx, store.SetListItemNotesParams{ID: itemID, ListID: l.ID, Notes: optional(notes)})
		if err == nil && n == 0 {
			return ErrNotFound
		}
		return err
	})
}

func ReorderList(ctx context.Context, db *pgxpool.Pool, profileID, listID int64, order []int64) error {
	return changeItems(ctx, db, profileID, listID, func(q *store.Queries, l store.List) error {
		if l.Rule != nil {
			return ErrRuleList
		}
		rows, err := q.ListEntryIDs(ctx, l.ID)
		if err != nil {
			return err
		}
		kids, err := isKids(ctx, q, profileID)
		if err != nil {
			return err
		}
		var ids []int64
		for _, r := range rows {
			if !kids || !r.Flagged {
				ids = append(ids, r.ID)
			}
		}
		sorted := slices.Clone(order)
		slices.Sort(sorted)
		slices.Sort(ids)
		if !slices.Equal(sorted, ids) {
			return ErrListOrder
		}
		full := make([]int64, 0, len(rows))
		next := 0
		for _, r := range rows {
			if kids && r.Flagged {
				full = append(full, r.ID)
				continue
			}
			full = append(full, order[next])
			next++
		}
		return q.RankListItems(ctx, store.RankListItemsParams{Ids: full, ListID: l.ID})
	})
}

func canEditItems(access, privacy, viewerKind string) bool {
	switch {
	case access == "mine":
		return true
	case access != "household" || privacy == "private":
		return false
	}
	return privacy == "household" || viewerKind != "kids" && viewerKind != "guest"
}

func changeList(ctx context.Context, db *pgxpool.Pool, profileID, id int64, change func(*store.Queries, store.List) error) error {
	return editList(ctx, db, profileID, func(q *store.Queries) (store.List, error) {
		l, err := q.OwnedList(ctx, store.OwnedListParams{ID: id, ProfileID: profileID})
		return l, notFound(err)
	}, change)
}

func changeItems(ctx context.Context, db *pgxpool.Pool, profileID, id int64, change func(*store.Queries, store.List) error) error {
	return editList(ctx, db, profileID, func(q *store.Queries) (store.List, error) {
		l, err := q.LockList(ctx, id)
		if err != nil {
			return l, notFound(err)
		}
		rows, err := q.VisibleLists(ctx, store.VisibleListsParams{Viewer: profileID, ListID: &id})
		if err != nil || len(rows) == 0 {
			return l, cmp.Or(err, ErrNotFound)
		}
		editor, err := q.ProfileByID(ctx, profileID)
		if err != nil {
			return l, err
		}
		if !canEditItems(rows[0].Access, l.Privacy, editor.Kind) {
			return l, ErrListReadOnly
		}
		return l, nil
	}, change)
}

func editList(ctx context.Context, db *pgxpool.Pool, profileID int64, lock func(*store.Queries) (store.List, error), change func(*store.Queries, store.List) error) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		l, err := lock(q)
		if err != nil {
			return err
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
	if c.Rule != nil {
		if err := c.Rule.check(); err != nil {
			return err
		}
		raw, err := ruleJSON(c.Rule)
		if err != nil {
			return err
		}
		l.Rule = raw
	}
	if l.Rule != nil && l.SortBy == "added" {
		return ErrRuleSort
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

func isKids(ctx context.Context, q *store.Queries, profileID int64) (bool, error) {
	if profileID == 0 {
		return false, nil
	}
	p, err := q.ProfileByID(ctx, profileID)
	if err != nil {
		return false, err
	}
	return p.Kind == "kids", nil
}
