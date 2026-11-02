package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	Name   string
	Year   int
	Poster string
}

type SeasonEpisode struct {
	Number  int32
	Name    string
	AirDate string
}

func (c *Client) Movie(ctx context.Context, id int32) (Title, error) {
	var m struct {
		Title   string `json:"title"`
		Release string `json:"release_date"`
		Poster  string `json:"poster_path"`
	}
	err := c.get(ctx, fmt.Sprintf("/3/movie/%d", id), &m)
	return Title{Name: m.Title, Year: year(m.Release), Poster: m.Poster}, err
}

func (c *Client) Show(ctx context.Context, id int32) (Title, error) {
	var s struct {
		Name     string `json:"name"`
		FirstAir string `json:"first_air_date"`
		Poster   string `json:"poster_path"`
	}
	err := c.get(ctx, fmt.Sprintf("/3/tv/%d", id), &s)
	return Title{Name: s.Name, Year: year(s.FirstAir), Poster: s.Poster}, err
}

func (c *Client) Season(ctx context.Context, showID, season int32) ([]SeasonEpisode, error) {
	var s struct {
		Episodes []struct {
			Number  int32  `json:"episode_number"`
			Name    string `json:"name"`
			AirDate string `json:"air_date"`
		} `json:"episodes"`
	}
	if err := c.get(ctx, fmt.Sprintf("/3/tv/%d/season/%d", showID, season), &s); err != nil {
		return nil, err
	}
	out := make([]SeasonEpisode, 0, len(s.Episodes))
	for _, e := range s.Episodes {
		out = append(out, SeasonEpisode{Number: e.Number, Name: e.Name, AirDate: e.AirDate})
	}
	return out, nil
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
