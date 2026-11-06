package tmdb

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

var ErrNotFound = errors.New("not on tmdb")

type Client struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
}

type Episode struct {
	ID      int32
	Season  int32
	Number  int32
	AirDate *time.Time
}

func (c *Client) Outline(ctx context.Context, showID int32) ([]Episode, error) {
	var show struct {
		Seasons []struct {
			Number int32 `json:"season_number"`
		} `json:"seasons"`
	}
	if err := c.get(ctx, fmt.Sprintf("/3/tv/%d", showID), &show); err != nil {
		return nil, err
	}
	var episodes []Episode
	for start := 0; start < len(show.Seasons); start += 20 {
		var keys []string
		for _, s := range show.Seasons[start:min(start+20, len(show.Seasons))] {
			keys = append(keys, fmt.Sprintf("season/%d", s.Number))
		}
		var seasons map[string]json.RawMessage
		if err := c.get(ctx, fmt.Sprintf("/3/tv/%d?append_to_response=%s", showID, strings.Join(keys, ",")), &seasons); err != nil {
			return nil, err
		}
		for _, key := range keys {
			var season struct {
				Episodes []struct {
					ID      int32  `json:"id"`
					Season  int32  `json:"season_number"`
					Number  int32  `json:"episode_number"`
					AirDate string `json:"air_date"`
				} `json:"episodes"`
			}
			if raw, ok := seasons[key]; ok {
				if err := json.Unmarshal(raw, &season); err != nil {
					return nil, err
				}
			}
			for _, e := range season.Episodes {
				ep := Episode{ID: e.ID, Season: e.Season, Number: e.Number}
				if t, err := time.Parse(time.DateOnly, e.AirDate); err == nil {
					ep.AirDate = &t
				}
				episodes = append(episodes, ep)
			}
		}
	}
	return episodes, nil
}

type Title struct {
	Name        string
	Year        int
	Poster      string
	Overview    string
	Tagline     string
	Genres      []string
	Runtime     int
	Certificate Certificate
	Score       float64
	Votes       int
	Creators    []string
	Cast        []string
	Networks    []string
	Status      string
	Next        *NextEpisode
	Seasons     []SeasonCount
}

type SeasonCount struct {
	Number   int32
	Episodes int
}

type Result struct {
	Kind   string
	ID     int32
	Name   string
	Year   int
	Poster string
}

type Certificate struct {
	Country string
	Rating  string
}

type NextEpisode struct {
	Season  int32
	Number  int32
	Name    string
	AirDate string
}

type Season struct {
	Overview string
	Episodes []SeasonEpisode
}

type SeasonEpisode struct {
	Number   int32
	Name     string
	AirDate  string
	Overview string
	Runtime  int
}

type named []struct {
	Name string `json:"name"`
}

func (n named) names() []string {
	var out []string
	for _, x := range n {
		out = append(out, x.Name)
	}
	return out
}

var certificateCountries = []string{"GB", "US"}

func (c *Client) Movie(ctx context.Context, id int32) (Title, error) {
	var m struct {
		Title    string  `json:"title"`
		Release  string  `json:"release_date"`
		Poster   string  `json:"poster_path"`
		Overview string  `json:"overview"`
		Tagline  string  `json:"tagline"`
		Genres   named   `json:"genres"`
		Runtime  int     `json:"runtime"`
		Score    float64 `json:"vote_average"`
		Votes    int     `json:"vote_count"`
		Credits  struct {
			Cast named `json:"cast"`
			Crew []struct {
				Name string `json:"name"`
				Job  string `json:"job"`
			} `json:"crew"`
		} `json:"credits"`
		ReleaseDates struct {
			Results []struct {
				Country string `json:"iso_3166_1"`
				Dates   []struct {
					Certification string `json:"certification"`
				} `json:"release_dates"`
			} `json:"results"`
		} `json:"release_dates"`
	}
	if err := c.get(ctx, fmt.Sprintf("/3/movie/%d?language=en-GB&append_to_response=credits,release_dates", id), &m); err != nil {
		return Title{}, err
	}
	t := Title{Name: m.Title, Year: year(m.Release), Poster: m.Poster, Overview: m.Overview, Tagline: m.Tagline, Genres: m.Genres.names(), Runtime: m.Runtime, Score: m.Score, Votes: m.Votes, Cast: m.Credits.Cast[:min(5, len(m.Credits.Cast))].names()}
	for _, p := range m.Credits.Crew {
		if p.Job == "Director" {
			t.Creators = append(t.Creators, p.Name)
		}
	}
	ratings := map[string]string{}
	for _, r := range m.ReleaseDates.Results {
		for _, d := range r.Dates {
			if d.Certification != "" && ratings[r.Country] == "" {
				ratings[r.Country] = d.Certification
			}
		}
	}
	t.Certificate = certificate(ratings)
	return t, nil
}

func (c *Client) Show(ctx context.Context, id int32) (Title, error) {
	var s struct {
		Name      string  `json:"name"`
		FirstAir  string  `json:"first_air_date"`
		Poster    string  `json:"poster_path"`
		Overview  string  `json:"overview"`
		Tagline   string  `json:"tagline"`
		Genres    named   `json:"genres"`
		RunTimes  []int   `json:"episode_run_time"`
		CreatedBy named   `json:"created_by"`
		Networks  named   `json:"networks"`
		Status    string  `json:"status"`
		Score     float64 `json:"vote_average"`
		Votes     int     `json:"vote_count"`
		Seasons   []struct {
			Number   int32 `json:"season_number"`
			Episodes int   `json:"episode_count"`
		} `json:"seasons"`
		LastEpisode *struct {
			Runtime int `json:"runtime"`
		} `json:"last_episode_to_air"`
		NextEpisode *struct {
			Season  int32  `json:"season_number"`
			Number  int32  `json:"episode_number"`
			Name    string `json:"name"`
			AirDate string `json:"air_date"`
		} `json:"next_episode_to_air"`
		Credits struct {
			Cast named `json:"cast"`
		} `json:"aggregate_credits"`
		Ratings struct {
			Results []struct {
				Country string `json:"iso_3166_1"`
				Rating  string `json:"rating"`
			} `json:"results"`
		} `json:"content_ratings"`
	}
	if err := c.get(ctx, fmt.Sprintf("/3/tv/%d?language=en-GB&append_to_response=aggregate_credits,content_ratings", id), &s); err != nil {
		return Title{}, err
	}
	t := Title{Name: s.Name, Year: year(s.FirstAir), Poster: s.Poster, Overview: s.Overview, Tagline: s.Tagline, Genres: s.Genres.names(), Creators: s.CreatedBy.names(), Cast: s.Credits.Cast[:min(5, len(s.Credits.Cast))].names(), Networks: s.Networks.names(), Status: s.Status, Score: s.Score, Votes: s.Votes}
	switch {
	case len(s.RunTimes) > 0:
		t.Runtime = s.RunTimes[0]
	case s.LastEpisode != nil:
		t.Runtime = s.LastEpisode.Runtime
	}
	for _, season := range s.Seasons {
		t.Seasons = append(t.Seasons, SeasonCount{Number: season.Number, Episodes: season.Episodes})
	}
	if n := s.NextEpisode; n != nil {
		t.Next = &NextEpisode{Season: n.Season, Number: n.Number, Name: n.Name, AirDate: n.AirDate}
	}
	ratings := map[string]string{}
	for _, r := range s.Ratings.Results {
		ratings[r.Country] = r.Rating
	}
	t.Certificate = certificate(ratings)
	return t, nil
}

func certificate(ratings map[string]string) Certificate {
	for _, country := range certificateCountries {
		if r := ratings[country]; r != "" {
			return Certificate{Country: country, Rating: r}
		}
	}
	return Certificate{}
}

func (c *Client) Season(ctx context.Context, showID, season int32) (Season, error) {
	var s struct {
		Overview string `json:"overview"`
		Episodes []struct {
			Number   int32  `json:"episode_number"`
			Name     string `json:"name"`
			AirDate  string `json:"air_date"`
			Overview string `json:"overview"`
			Runtime  int    `json:"runtime"`
		} `json:"episodes"`
	}
	if err := c.get(ctx, fmt.Sprintf("/3/tv/%d/season/%d?language=en-GB", showID, season), &s); err != nil {
		return Season{}, err
	}
	out := Season{Overview: s.Overview, Episodes: make([]SeasonEpisode, 0, len(s.Episodes))}
	for _, e := range s.Episodes {
		out.Episodes = append(out.Episodes, SeasonEpisode{Number: e.Number, Name: e.Name, AirDate: e.AirDate, Overview: e.Overview, Runtime: e.Runtime})
	}
	return out, nil
}

func (c *Client) Search(ctx context.Context, query string) ([]Result, error) {
	var page struct {
		Results []struct {
			MediaType string `json:"media_type"`
			ID        int32  `json:"id"`
			Title     string `json:"title"`
			Name      string `json:"name"`
			Release   string `json:"release_date"`
			FirstAir  string `json:"first_air_date"`
			Poster    string `json:"poster_path"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/3/search/multi?language=en-GB&include_adult=false&query="+url.QueryEscape(query), &page); err != nil {
		return nil, err
	}
	var out []Result
	for _, r := range page.Results {
		switch r.MediaType {
		case "movie":
			out = append(out, Result{Kind: "movie", ID: r.ID, Name: r.Title, Year: year(r.Release), Poster: r.Poster})
		case "tv":
			out = append(out, Result{Kind: "show", ID: r.ID, Name: r.Name, Year: year(r.FirstAir), Poster: r.Poster})
		}
	}
	return out, nil
}

type Group struct {
	ID       string
	Name     string
	Type     int16
	Seasons  int
	Episodes int
}

func (c *Client) Groups(ctx context.Context, showID int32) ([]Group, error) {
	var found struct {
		Results []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Type     int16  `json:"type"`
			Seasons  int    `json:"group_count"`
			Episodes int    `json:"episode_count"`
		} `json:"results"`
	}
	if err := c.get(ctx, fmt.Sprintf("/3/tv/%d/episode_groups", showID), &found); err != nil {
		return nil, err
	}
	out := make([]Group, 0, len(found.Results))
	for _, g := range found.Results {
		out = append(out, Group(g))
	}
	return out, nil
}

type EpisodeGroup struct {
	ID          string
	Name        string
	Description string
	Type        int16
	Seasons     []GroupSeason
}

type GroupSeason struct {
	Number   int32
	Name     string
	Episodes []GroupEpisode
}

type GroupEpisode struct {
	ID       int32
	ShowID   int32
	Season   int32
	Number   int32
	Place    int32
	Name     string
	AirDate  string
	Overview string
	Runtime  int
	Still    string
}

type groupEpisode struct {
	ID       int32  `json:"id"`
	ShowID   int32  `json:"show_id"`
	Season   int32  `json:"season_number"`
	Number   int32  `json:"episode_number"`
	Order    int32  `json:"order"`
	Name     string `json:"name"`
	AirDate  string `json:"air_date"`
	Overview string `json:"overview"`
	Runtime  int    `json:"runtime"`
	Still    string `json:"still_path"`
}

type groupSeason struct {
	Order    int32          `json:"order"`
	Name     string         `json:"name"`
	Episodes []groupEpisode `json:"episodes"`
}

// A group's order is its season number and an episode's place in it is its number, which is how TMDB shows them.
func (c *Client) EpisodeGroup(ctx context.Context, id string) (EpisodeGroup, error) {
	var group struct {
		ID          string        `json:"id"`
		Name        string        `json:"name"`
		Description string        `json:"description"`
		Type        int16         `json:"type"`
		Groups      []groupSeason `json:"groups"`
	}
	if err := c.get(ctx, "/3/tv/episode_group/"+url.PathEscape(id), &group); err != nil {
		return EpisodeGroup{}, err
	}
	out := EpisodeGroup{ID: group.ID, Name: group.Name, Description: group.Description, Type: group.Type}
	slices.SortStableFunc(group.Groups, func(a, b groupSeason) int { return cmp.Compare(a.Order, b.Order) })
	for _, g := range group.Groups {
		slices.SortStableFunc(g.Episodes, func(a, b groupEpisode) int { return cmp.Compare(a.Order, b.Order) })
		season := GroupSeason{Number: g.Order, Name: g.Name}
		for i, e := range g.Episodes {
			season.Episodes = append(season.Episodes, GroupEpisode{
				ID: e.ID, ShowID: e.ShowID, Season: e.Season, Number: e.Number, Place: int32(i + 1),
				Name: e.Name, AirDate: e.AirDate, Overview: e.Overview, Runtime: e.Runtime, Still: e.Still,
			})
		}
		out.Seasons = append(out.Seasons, season)
	}
	return out, nil
}

type FoundEpisode struct {
	ID     int32
	ShowID int32
	Season int32
	Number int32
}

// source is TMDB's name for the other ID, like tvdb_id or imdb_id.
func (c *Client) FindEpisode(ctx context.Context, source, id string) (FoundEpisode, error) {
	var found struct {
		Episodes []struct {
			ID     int32 `json:"id"`
			ShowID int32 `json:"show_id"`
			Season int32 `json:"season_number"`
			Number int32 `json:"episode_number"`
		} `json:"tv_episode_results"`
	}
	if err := c.get(ctx, "/3/find/"+url.PathEscape(id)+"?external_source="+url.QueryEscape(source), &found); err != nil {
		return FoundEpisode{}, err
	}
	if len(found.Episodes) == 0 {
		return FoundEpisode{}, ErrNotFound
	}
	return FoundEpisode(found.Episodes[0]), nil
}

func year(date string) int {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return 0
	}
	return t.Year()
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	base := c.BaseURL
	if base == "" {
		base = "https://api.themoviedb.org"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("tmdb %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
