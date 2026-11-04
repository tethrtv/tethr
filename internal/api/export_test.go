package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tethrtv/tethr/internal/dbtest"
)

func TestExportData(t *testing.T) {
	mail := &fakeMail{}
	s := &Server{DB: dbtest.New(t), Mail: mail, AppURL: "https://staging.tethr.tv", AllowedOrigins: []string{"https://staging.tethr.tv"}, Cookie: CookieConfig{Name: "tethr_session"}}
	h := s.Handler()
	cookie := signIn(t, h, mail)
	if rec := do(t, h, "POST", "/v1/history", `{"items":[{"item":{"type":"movie","ids":{"tmdb":27205}},"watched_at":"2024-05-01T20:00:00Z"}]}`, cookie); rec.Code != http.StatusOK {
		t.Fatalf("add history = %d %s", rec.Code, rec.Body)
	}
	if _, err := s.DB.Exec(context.Background(), `UPDATE titles SET name = '=HYPERLINK("x")', year = 2010`); err != nil {
		t.Fatal(err)
	}

	export := func(site string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/v1/export", nil)
		req.Header.Set("Sec-Fetch-Site", site)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := export("cross-site"); rec.Code != http.StatusForbidden {
		t.Errorf("from another site = %d", rec.Code)
	}

	rec := export("same-site")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), `attachment; filename="tethr-`) {
		t.Fatalf("export = %d %s", rec.Code, rec.Header())
	}
	if notice := mail.last(t); notice.subject != "Your Tethr data was downloaded" || notice.to != "nathan@example.com" {
		t.Errorf("notice = %+v", notice)
	}
	z, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		t.Helper()
		f, err := z.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(f)
		return b
	}
	var doc Export
	if err := json.Unmarshal(read("tethr.json"), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Format != Tethr || doc.Account.Email != "nathan@example.com" || len(doc.Profiles) != 1 || len(doc.Profiles[0].History) != 1 {
		t.Errorf("doc = %+v", doc)
	}
	rows, err := csv.NewReader(bytes.NewReader(read("history.csv"))).ReadAll()
	if err != nil || len(rows) != 2 {
		t.Fatalf("csv = %v, %v", rows, err)
	}
	if got := rows[1]; got[1] != "Nathan" || got[2] != "movie" || got[3] != `'=HYPERLINK("x")` || got[4] != "2010" || got[7] != "27205" {
		t.Errorf("csv row = %q", got)
	}

	if rec := export("same-site"); rec.Code != http.StatusOK {
		t.Errorf("second export = %d", rec.Code)
	}
	if rec := export("same-site"); rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "too_many_exports") {
		t.Errorf("third export = %d %s", rec.Code, rec.Body)
	}
	if rec := bearer(t, h, "GET", "/v1/export", "", "token"); rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Errorf("with a token = %d", rec.Code)
	}
}

func TestDescribeDevice(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36":                   "Chrome on a Mac",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 26_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.1 Mobile/15E148 Safari/604.1": "Safari on an iPhone",
		"Zuno/2.3": "an unknown device",
		"":         "an unknown device",
	} {
		if got := describeDevice(ua); got != want {
			t.Errorf("describeDevice(%q) = %q, want %q", ua, got, want)
		}
	}
}
