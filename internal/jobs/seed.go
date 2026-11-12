package jobs

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/wikidata"
)

type Seed struct {
	DB  *pgxpool.Pool
	Log *slog.Logger

	Items   []string
	Classes []string
	Labels  string
	Names   string
	Taken   time.Time
}

func (s Seed) Run(ctx context.Context) error {
	if err := s.loadClasses(ctx); err != nil {
		return err
	}
	labels := map[string]map[string]string{}
	if s.Labels != "" {
		var err error
		if labels, err = readGzip(s.Labels, wikidata.ReadSnapshotLabels); err != nil {
			return fmt.Errorf("%s: %w", s.Labels, err)
		}
	}
	held, err := s.readNames()
	if err != nil {
		return err
	}
	owners, err := s.tmdbOwners(labels)
	if err != nil {
		return err
	}

	sd := &seeding{db: s.DB, log: s.Log, counts: map[string]int{}}
	counts := sd.counts
	report := func(msg string) {
		s.Log.Info(msg, "items", counts["items"], "created", counts["created"], "updated", counts["updated"], "skipped", counts["skipped"],
			"left out", counts["left out"], "kind unknown", counts["kind unknown"], "unreadable", counts["unreadable"])
	}
	unreadable, err := s.eachItem(labels, func(it wikidata.Item) error {
		if counts["items"]++; counts["items"]%10000 == 0 {
			report("seeding")
		}
		return sd.item(ctx, it, owners, held(it))
	})
	counts["unreadable"] = unreadable
	if err != nil {
		report("seed stopped")
		return err
	}
	report("seeded")
	return nil
}

func (s Seed) loadClasses(ctx context.Context) error {
	for _, path := range s.Classes {
		var batch []core.WikidataClass
		var unreadable []string
		_, err := readGzip(path, func(r io.Reader) (struct{}, error) {
			return struct{}{}, wikidata.ReadSnapshotClasses(r, func(c wikidata.Class, readable bool) error {
				if readable {
					batch = append(batch, core.WikidataClass(c))
				} else {
					unreadable = append(unreadable, c.QID)
				}
				return nil
			})
		})
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := core.SaveClasses(ctx, s.DB, batch); err != nil {
			return err
		}
		for _, q := range unreadable {
			if err := core.AddMissingClass(ctx, s.DB, q); err != nil {
				return err
			}
			s.Log.Warn("seed class unreadable, kept any row already there", "class", q)
		}
		s.Log.Info("classes loaded", "file", path, "classes", len(batch), "unreadable", len(unreadable))
	}
	return nil
}

func (s Seed) eachItem(labels map[string]map[string]string, each func(wikidata.Item) error) (int, error) {
	unreadable := 0
	for _, path := range s.Items {
		n, err := readGzip(path, func(r io.Reader) (int, error) {
			return wikidata.ReadSnapshot(r, labels, each)
		})
		unreadable += n
		if err != nil {
			return unreadable, fmt.Errorf("%s: %w", path, err)
		}
	}
	return unreadable, nil
}

func tmdbKey(kind string, tmdb int32) string {
	return kind + " " + strconv.Itoa(int(tmdb))
}

func (s Seed) tmdbOwners(labels map[string]map[string]string) (map[string]string, error) {
	type claim struct {
		qid       string
		sitelinks int
	}
	claims := map[string][]claim{}
	_, err := s.eachItem(labels, func(it wikidata.Item) error {
		for _, kind := range it.TMDBKinds() {
			w := it.Title(kind)
			if w.TMDB != 0 {
				key := tmdbKey(kind, w.TMDB)
				claims[key] = append(claims[key], claim{it.QID(), w.Sitelinks})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	owners := map[string]string{}
	shared, unowned := 0, 0
	for key, cs := range claims {
		slices.SortFunc(cs, func(a, b claim) int { return b.sitelinks - a.sitelinks })
		// When items share a TMDB ID, the best known keeps it only if it has at least twice the sitelinks of the next.
		switch {
		case len(cs) == 1 || cs[0].sitelinks >= 2*cs[1].sitelinks && cs[0].sitelinks > 0:
			owners[key] = cs[0].qid
		default:
			owners[key] = ""
			unowned++
		}
		if len(cs) > 1 {
			shared++
		}
	}
	s.Log.Info("tmdb ids", "claimed", len(claims), "shared", shared, "unowned", unowned)
	return owners, nil
}

func (s Seed) readNames() (func(wikidata.Item) bool, error) {
	weekAgo := map[string]string{}
	if s.Names != "" {
		f, err := os.Open(s.Names)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		dec := json.NewDecoder(bufio.NewReader(f))
		for {
			var n struct {
				QID string  `json:"qid"`
				En  *string `json:"en"`
				Mul *string `json:"mul"`
			}
			err := dec.Decode(&n)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", s.Names, err)
			}
			switch {
			case n.En != nil:
				weekAgo[n.QID] = *n.En
			case n.Mul != nil:
				weekAgo[n.QID] = *n.Mul
			default:
				weekAgo[n.QID] = ""
			}
		}
	}
	return func(it wikidata.Item) bool {
		if !it.Modified.IsZero() && it.Modified.Before(s.Taken.AddDate(0, 0, -7)) {
			return true
		}
		was, ok := weekAgo[it.QID()]
		now := it.Label("en")
		if now == "" {
			now = it.Label("mul")
		}
		return ok && now != "" && strings.EqualFold(was, now)
	}, nil
}

func readGzip[T any](path string, read func(io.Reader) (T, error)) (T, error) {
	var zero T
	f, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return zero, err
	}
	zr.Multistream(true)
	return read(zr)
}

type seeding struct {
	db     *pgxpool.Pool
	log    *slog.Logger
	counts map[string]int
}

func (sd *seeding) item(ctx context.Context, it wikidata.Item, owners map[string]string, nameHeld bool) error {
	kinds, complete, err := core.SeedKinds(ctx, sd.db, it.TMDBKinds(), it.InstanceOf())
	if err != nil {
		return err
	}
	if len(kinds) == 0 {
		if !complete {
			sd.counts["kind unknown"]++
			sd.log.Info("left out an item whose classes aren't all known", "item", it.QID(), "instance of", it.InstanceOf())
			return nil
		}
		sd.counts["left out"]++
		sd.log.Info("left out an item that isn't one film or show", "item", it.QID(), "instance of", it.InstanceOf())
		return core.LeaveOutWikidata(ctx, sd.db, it.QID(), "not one film or show")
	}
	for _, kind := range kinds {
		w := it.Title(kind)
		var shared int32
		if owner, ok := owners[tmdbKey(kind, w.TMDB)]; ok && owner != it.QID() {
			shared, w.TMDB = w.TMDB, 0
		}
		res, err := core.SeedTitle(ctx, sd.db, kind, core.WikidataTitle(w), shared, nameHeld)
		if err != nil {
			return fmt.Errorf("%s: %w", it.QID(), err)
		}
		sd.counts[string(res.Outcome)]++
		if res.Outcome == core.SeedSkipped {
			sd.log.Info("skipped an item another title already stands for", "item", it.QID(), "kind", kind, "title", res.Title)
			if err := core.LeaveOutWikidata(ctx, sd.db, it.QID(), "another title stands for it"); err != nil {
				return err
			}
		}
		for _, c := range res.Conflicts {
			sd.log.Warn("wikidata item disagrees with an outside id", "item", it.QID(), "title", res.Title, "provider", c.Provider, "value", c.Value, "other", c.Other, "outcome", c.Outcome)
		}
	}
	return nil
}
