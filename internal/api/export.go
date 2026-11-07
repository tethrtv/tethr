package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/mail"
)

func (s *Server) ExportData(ctx context.Context, _ ExportDataRequestObject) (ExportDataResponseObject, error) {
	if site, _ := ctx.Value(fetchSiteKey).(string); site != "same-origin" && site != "same-site" {
		return ExportData403JSONResponse(problem("own_site_only", "Download your data from the account page on Tethr.")), nil
	}
	accountID, token, ok, err := s.signedIn(ctx)
	if !ok {
		return ExportData401JSONResponse{notSignedIn}, err
	}
	if _, busy := s.exporting.LoadOrStore(accountID, true); busy {
		return ExportData429JSONResponse(problem("export_running", "Your last download is still being put together. Give it a moment.")), nil
	}
	defer s.exporting.Delete(accountID)

	start, err := core.StartExport(ctx, s.DB, token)
	switch {
	case errors.Is(err, core.ErrNoSession):
		return ExportData401JSONResponse{notSignedIn}, nil
	case errors.Is(err, core.ErrSignInAgain):
		return ExportData403JSONResponse(problem("sign_in_again", "To keep your data safe, sign in again before downloading it.")), nil
	case errors.Is(err, core.ErrTooManyExports):
		return ExportData429JSONResponse(problem("too_many_exports", "You can download your data twice a day. Try again tomorrow.")), nil
	case err != nil:
		return nil, err
	}
	notice, err := exportEmail(s.AppURL, start)
	if err != nil {
		return nil, err
	}
	if err := s.Mail.Send(ctx, notice); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	household, err := core.Export(ctx, s.DB, accountID)
	if err != nil {
		return nil, err
	}
	file, err := s.exportZip(household, start.At)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf(`attachment; filename="tethr-%s.zip"`, start.At.UTC().Format("2006-01-02"))
	return ExportData200ApplicationzipResponse{Body: bytes.NewReader(file), ContentLength: int64(len(file)), Headers: ExportData200ResponseHeaders{ContentDisposition: &name}}, nil
}

func (s *Server) exportZip(h core.HouseholdExport, at time.Time) ([]byte, error) {
	doc := Export{
		Format:       Tethr,
		Version:      1,
		ExportedAt:   at.UTC(),
		Account:      Account{Id: h.Account.ID, Email: h.Account.Email, Name: h.Account.Name, CreatedAt: h.Account.CreatedAt},
		Passkeys:     make([]Passkey, 0, len(h.Passkeys)),
		Apps:         make([]DeveloperApp, 0, len(h.Apps)),
		Connections:  make([]Connection, 0, len(h.Connections)),
		MediaServers: make([]MediaServer, 0, len(h.MediaServers)),
		Profiles:     make([]ProfileExport, 0, len(h.Profiles)),
	}
	for _, p := range h.Passkeys {
		doc.Passkeys = append(doc.Passkeys, toPasskey(p))
	}
	for _, a := range h.Apps {
		doc.Apps = append(doc.Apps, DeveloperApp{Id: a.ID, Name: a.Name, ClientId: a.ClientID, RedirectUris: a.RedirectUris, Connections: a.Connections, CreatedAt: a.CreatedAt})
	}
	for _, g := range h.Connections {
		doc.Connections = append(doc.Connections, Connection{Id: g.ID, AppName: g.AppName, Official: g.Official, ProfileId: g.ProfileID, ProfileName: g.ProfileName, Label: g.Label, UserAgent: g.UserAgent, CreatedAt: g.CreatedAt, LastUsedAt: g.LastUsedAt})
	}

	for _, m := range h.MediaServers {
		doc.MediaServers = append(doc.MediaServers, toMediaServer(m))
	}

	var rows [][]string
	for _, p := range h.Profiles {
		out := ProfileExport{
			Profile:   toProfile(p.Profile),
			History:   make([]Watch, 0, len(p.History)),
			Playback:  make([]ResumePoint, 0, len(p.Playback)),
			Ratings:   make([]Rating, 0, len(p.Ratings)),
			Watchlist: make([]Listing, 0, len(p.Watchlist)),
			Lists:     make([]ListExport, 0, len(p.Lists)),
			Hidden:    make([]HiddenItem, 0, len(p.Hidden)),
			Imports:   make([]Import, 0, len(p.Imports)),
		}
		for _, e := range p.History {
			out.History = append(out.History, Watch{Id: e.ID, WatchedAt: e.At, Action: WatchAction(e.Action), Item: toItem(e.Item)})
			rows = append(rows, historyRow(p.Profile.Name, e))
		}
		for _, e := range p.Playback {
			out.Playback = append(out.Playback, ResumePoint{Id: e.ID, Progress: e.Progress, PausedAt: e.At, Item: toItem(e.Item)})
		}
		for _, e := range p.Ratings {
			out.Ratings = append(out.Ratings, Rating{Id: e.ID, Rating: int(e.Rating), RatedAt: e.At, Item: toItem(e.Item)})
		}
		for _, e := range p.Watchlist {
			out.Watchlist = append(out.Watchlist, Listing{Id: e.ID, Rank: int(e.Rank), ListedAt: e.At, Notes: e.Notes, Item: toItem(e.Item)})
		}
		for _, l := range p.Lists {
			items := make([]Listing, 0, len(l.Entries))
			for _, e := range l.Entries {
				items = append(items, Listing{Id: e.ID, Rank: int(e.Rank), ListedAt: e.At, Notes: e.Notes, Item: toItem(e.Item)})
			}
			out.Lists = append(out.Lists, ListExport{List: s.toList(l.List), Items: items})
		}
		for _, e := range p.Hidden {
			out.Hidden = append(out.Hidden, HiddenItem{Section: e.Section, HiddenAt: e.At, Item: toItem(e.Item)})
		}
		for _, i := range p.Imports {
			out.Imports = append(out.Imports, toImport(i))
		}
		doc.Profiles = append(doc.Profiles, out)
	}

	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.Create("tethr.json")
	if err != nil {
		return nil, err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	w, err = z.Create("history.csv")
	if err != nil {
		return nil, err
	}
	c := csv.NewWriter(w)
	if err := c.Write([]string{"watched_at", "profile", "type", "name", "year", "season", "episode", "tmdb", "imdb", "tvdb", "how"}); err != nil {
		return nil, err
	}
	if err := c.WriteAll(rows); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func historyRow(profile string, e core.Entry) []string {
	t := e.Item.Title
	row := []string{e.At.UTC().Format(time.RFC3339), profile, e.Item.Kind(), "", "", "", "", "", t.Imdb, t.Tvdb, e.Action}
	if t.Name != nil {
		row[3] = *t.Name
	}
	if t.Year != nil {
		row[4] = strconv.Itoa(int(*t.Year))
	}
	if ep := e.Item.Episode; ep != nil {
		row[5] = strconv.Itoa(int(ep.SeasonNumber))
		row[6] = strconv.Itoa(int(ep.EpisodeNumber))
	}
	if t.TmdbID != nil {
		row[7] = strconv.Itoa(int(*t.TmdbID))
	}
	for i, cell := range row {
		row[i] = safeCell(cell)
	}
	return row
}

// Spreadsheets run cells starting with these as formulas, and names come from anyone who edits Wikidata.
func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func exportEmail(appURL string, start core.ExportStart) (mail.Message, error) {
	device := describeDevice(start.Device)
	when := start.At.UTC().Format("2 January 2006 at 15:04 UTC")
	data := struct{ Title, Preview, Lead, Button, Link, Note, Footer, AppURL, Image, ImageAlt string }{
		Title:    "Your Tethr data was downloaded",
		Preview:  "A copy of everything in your household was downloaded.",
		Lead:     fmt.Sprintf("A copy of everything in your household was downloaded from %s on %s.", device, when),
		Button:   "Check your sessions",
		Link:     appURL + "/account",
		Note:     "If this wasn’t you, sign out everywhere from your account page, then check that nobody else can get into your email.",
		Footer:   "We send this every time your data is downloaded.",
		AppURL:   appURL,
		Image:    "email-download.png",
		ImageAlt: "An old telly showing “Downloaded”",
	}
	var html bytes.Buffer
	if err := emails.ExecuteTemplate(&html, "notice.html", data); err != nil {
		return mail.Message{}, err
	}
	text := fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s\n", data.Title, data.Lead, data.Note, data.Link)
	return mail.Message{To: start.Email, Subject: data.Title, Text: text, HTML: html.String()}, nil
}

func describeDevice(userAgent string) string {
	pick := func(names [][2]string) string {
		for _, n := range names {
			if strings.Contains(userAgent, n[0]) {
				return n[1]
			}
		}
		return ""
	}
	browser := pick([][2]string{{"Edg/", "Edge"}, {"Firefox/", "Firefox"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}})
	system := pick([][2]string{{"iPhone", "an iPhone"}, {"iPad", "an iPad"}, {"Android", "Android"}, {"Macintosh", "a Mac"}, {"Mac OS X", "a Mac"}, {"Windows", "Windows"}, {"Linux", "Linux"}})
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	case system != "":
		return system
	}
	return "an unknown device"
}
