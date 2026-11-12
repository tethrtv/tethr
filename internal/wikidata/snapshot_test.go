package wikidata

import (
	"fmt"
	"strings"
	"testing"
)

func TestReadSnapshot(t *testing.T) {
	lines := strings.Join([]string{
		`{"id":"Q83495","labels":{"mul":"The Matrix"},"sitelinks":120,"modified":"2026-11-01T10:00:00Z","claims":{"P4947":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"603"}}}],"P31":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q11424"}}}}]}}`,
		`{"id":"Q3","labels":{},"sitelinks":4,"modified":"2026-11-09T10:00:00Z","claims":{"P4983":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"1"}}}],"P4947":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"2"}}}]}}`,
		`{"id":"Q5","labels":{"en":"Broken"},"sitelinks":1,"modified":"2026-11-01T10:00:00Z","claims":{"P31":"not a list"}}`,
	}, "\n")
	var got []string
	unreadable, err := ReadSnapshot(strings.NewReader(lines), map[string]map[string]string{"Q3": {"ja": "千と千尋の神隠し"}}, func(it Item) error {
		w := it.Title(it.TMDBKinds()[0])
		got = append(got, fmt.Sprintf("%s %v %s %d %d %v %s", it.QID(), it.TMDBKinds(), w.Name, w.TMDB, w.Sitelinks, it.InstanceOf(), it.Modified.Format("2006-01-02")))
		return nil
	})
	if err != nil || unreadable != 1 {
		t.Fatalf("unreadable = %d, %v", unreadable, err)
	}
	want := "[Q83495 [movie] The Matrix 603 120 [Q11424] 2026-11-01 Q3 [movie show] 千と千尋の神隠し 2 4 [] 2026-11-09]"
	if fmt.Sprint(got) != want {
		t.Errorf("items = %v\nwant %s", got, want)
	}
}

func TestReadSnapshotClasses(t *testing.T) {
	lines := `{"id":"Q17","labels":{"en":"Japan"},"claims":{"P297":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"JP"}}}]}}
{"id":"Q188473","labels":{"en":"action film"},"claims":{"P279":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q1762165"}}}}]}}
{"id":"Q6","labels":{},"claims":{"P279":"not a list"}}`
	var got []Class
	var broken []string
	err := ReadSnapshotClasses(strings.NewReader(lines), func(c Class, readable bool) error {
		if readable {
			got = append(got, c)
		} else {
			broken = append(broken, c.QID)
		}
		return nil
	})
	if err != nil || fmt.Sprint(broken) != "[Q6]" {
		t.Fatalf("unreadable classes = %v, %v", broken, err)
	}
	if false {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != "[{Q17 Japan JP  []} {Q188473 action film   [Q1762165]}]" {
		t.Errorf("classes = %v", got)
	}
}

func TestReadSnapshotCutShort(t *testing.T) {
	cut := `{"id":"Q1","labels":{},"sitelinks":1,"claims":{}}
{"id":"Q2","labels":{},"sit`
	if _, err := ReadSnapshot(strings.NewReader(cut), nil, func(Item) error { return nil }); err == nil {
		t.Error("a file cut off mid-item should be an error, not a quiet end")
	}
}
