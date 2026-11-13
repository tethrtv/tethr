package wikidata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecentChanges(t *testing.T) {
	var asked []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		asked = append(asked, q.Get("rccontinue"))
		if q.Get("list") != "recentchanges" || q.Get("rcnamespace") != "0" || q.Get("rcdir") != "newer" ||
			q.Get("rcstart") != "2026-11-10T16:50:45Z" || q.Get("rcend") != "2026-11-10T17:00:00Z" || !strings.Contains(q.Get("rcprop"), "user") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if q.Get("rccontinue") == "" {
			_, _ = w.Write([]byte(`{"continue":{"rccontinue":"20261110165200|123"},"query":{"recentchanges":[
				{"type":"edit","title":"Q83495","timestamp":"2026-11-10T16:51:00Z","user":"Someone","comment":"/* wbsetlabel-set:1|en */ The Matrix"},
				{"type":"edit","title":"Q83495","timestamp":"2026-11-10T16:51:30Z","user":"~2026-59816-69","temp":"","comment":"/* wbsetdescription-add:1|fr */ film"}
			]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"query":{"recentchanges":[
			{"type":"log","title":"Q1","timestamp":"2026-11-10T16:52:00Z","user":"192.0.2.1","anon":"","logtype":"delete","comment":""},
			{"type":"edit","title":"Property:P31","timestamp":"2026-11-10T16:53:00Z","comment":""}
		]}}`))
	}))
	defer fake.Close()

	c := &Client{APIURL: fake.URL, UserAgent: "Tethr/test"}
	from := time.Date(2026, 11, 10, 16, 50, 45, 0, time.UTC)
	changes, next, err := c.RecentChanges(context.Background(), from, from.Add(9*time.Minute+15*time.Second), "")
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{QID: "Q83495", At: time.Date(2026, 11, 10, 16, 51, 0, 0, time.UTC), Relevant: true},
		{QID: "Q83495", At: time.Date(2026, 11, 10, 16, 51, 30, 0, time.UTC), LoggedOut: true},
	}
	if len(changes) != len(want) || changes[0] != want[0] || changes[1] != want[1] || next != "20261110165200|123" {
		t.Fatalf("first page = %+v, %q", changes, next)
	}
	changes, next, err = c.RecentChanges(context.Background(), from, from.Add(9*time.Minute+15*time.Second), next)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].QID != "Q1" || !changes[0].Relevant || !changes[0].LoggedOut || next != "" {
		t.Fatalf("second page = %+v, %q, want the deletion and nothing outside items", changes, next)
	}
}

func TestRelevantEdit(t *testing.T) {
	for _, tc := range []struct {
		comment string
		want    bool
	}{
		{"/* wbsetlabel-set:1|en */ The Matrix", true},
		{"/* wbsetlabel-add:1|ja */ マトリックス", true},
		{"/* wbsetlabeldescriptionaliases:2|de */ Matrix, Film", true},
		{"/* wbsetdescription-add:1|fr */ film de 1999", false},
		{"/* wbsetaliases-add:1|en */ Matrix", false},
		{"/* wbsetsitelink-add:1|frwiki */ Matrix (film)", false},
		{"/* clientsitelink-update:0|uk|ukwiki:Old|ukwiki:New */", false},
		{"/* wbsetreference-add:2| */ [[Property:P577]]: 31 March 1999", false},
		{"/* wbsetqualifier-add:1| */ [[Property:P291]]: [[Q30]]", false},
		{"/* wbcreateclaim-create:1| */ [[Property:P13459]]: [[Q133830758]], [[:toollabs:quickstatements/#/batch/294650|batch #294650]]", false},
		{"/* wbcreateclaim-create:1| */ [[Property:P4947]]: 603", true},
		{"/* wbsetclaim-update:2||1 */ [[Property:P31]]: [[Q11424]]", true},
		{"/* wbremoveclaims-remove:1| */ [[Property:P345]]: tt0133093", true},
		{"/* wbsetclaim-create:2||1 */ [[Property:P279]]: [[Q11424]]", true},
		{"/* wbsetclaim-create:2||1 */ [[Property:P297]]: GB", true},
		{"/* wbeditentity-update:0| */", true},
		{"/* wbeditentity-update-languages-short:0||de, fr */", true},
		{"/* wbmergeitems-from:0||Q123 */", true},
		{"/* wbcreateredirect:0||Q1|Q2 */", true},
		{"Undid revision 2567633370 by [[Special:Contributions/Someone|Someone]]", true},
		{"", true},
	} {
		if got := relevantEdit(tc.comment); got != tc.want {
			t.Errorf("relevantEdit(%q) = %v, want %v", tc.comment, got, tc.want)
		}
	}
}
