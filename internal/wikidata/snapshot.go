package wikidata

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

type Item struct {
	e        entity
	Modified time.Time
}

func (it Item) QID() string              { return it.e.ID }
func (it Item) Title(kind string) Title  { return it.e.title(kind) }
func (it Item) InstanceOf() []string     { return it.e.items("P31") }
func (it Item) Label(lang string) string { return it.e.Labels[lang].Value }

func (it Item) TMDBKinds() []string {
	var kinds []string
	for _, kind := range []string{"movie", "show"} {
		if p, _ := property(kind, "tmdb"); len(it.e.strings(p, digits)) > 0 {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

type snapshotItem struct {
	ID        string                     `json:"id"`
	Labels    map[string]string          `json:"labels"`
	Sitelinks int                        `json:"sitelinks"`
	Modified  time.Time                  `json:"modified"`
	Claims    map[string]json.RawMessage `json:"claims"`
}

func (s snapshotItem) entity(more map[string]string) entity {
	e := entity{ID: s.ID, Claims: s.Claims, sitelinkCount: s.Sitelinks, Labels: map[string]struct {
		Value string `json:"value"`
	}{}}
	for _, labels := range []map[string]string{more, s.Labels} {
		for lang, v := range labels {
			e.Labels[lang] = struct {
				Value string `json:"value"`
			}{v}
		}
	}
	return e
}

func ReadSnapshot(r io.Reader, labels map[string]map[string]string, each func(Item) error) (int, error) {
	dec := json.NewDecoder(bufio.NewReaderSize(r, 1<<20))
	unreadable := 0
	for n := 1; ; n++ {
		var s snapshotItem
		err := dec.Decode(&s)
		if errors.Is(err, io.EOF) {
			return unreadable, nil
		}
		if err != nil {
			return unreadable, fmt.Errorf("snapshot item %d: %w", n, err)
		}
		e := s.entity(labels[s.ID])
		if !IsQID(e.ID) || !e.readable() {
			unreadable++
			continue
		}
		if err := each(Item{e: e, Modified: s.Modified}); err != nil {
			return unreadable, err
		}
	}
}

func ReadSnapshotLabels(r io.Reader) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	dec := json.NewDecoder(bufio.NewReaderSize(r, 1<<20))
	for {
		var s struct {
			ID     string            `json:"id"`
			Labels map[string]string `json:"labels"`
		}
		err := dec.Decode(&s)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out[s.ID] = s.Labels
	}
}

func ReadSnapshotClasses(r io.Reader, each func(c Class, readable bool) error) error {
	dec := json.NewDecoder(bufio.NewReaderSize(r, 1<<20))
	for {
		var s snapshotItem
		err := dec.Decode(&s)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		e := s.entity(nil)
		if !IsQID(e.ID) {
			continue
		}
		if !e.readable() {
			if err := each(Class{QID: e.ID}, false); err != nil {
				return err
			}
			continue
		}
		c := Class{QID: e.ID, Label: s.Labels["en"], Country: e.first("P297"), Language: e.first("P305"), Parents: slices.Clone(e.items("P279"))}
		if c.Language == "" {
			c.Language = e.first("P218")
		}
		if err := each(c, true); err != nil {
			return err
		}
	}
}
