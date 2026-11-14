package wikidata

import (
	"net/url"
	"regexp"
	"strings"
)

func site(domain, path string) *regexp.Regexp {
	return regexp.MustCompile(`^(?:[a-z0-9-]+\.)*` + domain + path)
}

var links = map[string][]*regexp.Regexp{
	"imdb":           {site(`imdb\.com`, `/title/(tt[0-9]+)`)},
	"tvmaze":         {site(`tvmaze\.com`, `/shows/([0-9]+)`)},
	"letterboxd":     {site(`letterboxd\.com`, `/film/([a-z0-9-]+)`)},
	"mal":            {site(`myanimelist\.net`, `/anime/([0-9]+)`)},
	"anidb":          {site(`anidb\.net`, `/anime/([0-9]+)`)},
	"anilist":        {site(`anilist\.co`, `/anime/([0-9]+)`)},
	"rottentomatoes": {site(`rottentomatoes\.com`, `/((?:m|tv)/[a-z0-9_-]+)`)},
	"metacritic":     {site(`metacritic\.com`, `/((?:movie|tv)/[^/]+)`)},
	"appletv":        {site(`tv\.apple\.com`, `/.*(umc\.cmc\.[a-z0-9]+)`)},
	"netflix":        {site(`netflix\.com`, `/(?:.*/)?title/([0-9]+)`)},
	"primevideo": {
		site(`primevideo\.com`, `/(?:.*/)?detail/(?:[^/]+/)?([A-Z0-9]{26}|[A-Z0-9]{10}|amzn1\.dv\.gti\.[0-9a-f-]+)(?:/|$)`),
		site(`amazon\.[a-z.]+`, `/(?:gp/video/detail|dp)/([A-Z0-9]{10})`),
	},
	"disneyplus": {site(`disneyplus\.com`, `/(?:.*/)?(?:movies|series|video)/(?:[^/]+/)?((?:entity-)?[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}|[A-Za-z0-9]{12,13})`)},
}

var tmdbLinks = map[string]*regexp.Regexp{
	"movie": site(`themoviedb\.org`, `/movie/([0-9]+)`),
	"show":  site(`themoviedb\.org`, `/tv/([0-9]+)`),
}

func OutsideID(kind, provider, in string) (string, bool) {
	prop, valid := property(kind, provider)
	if prop == "" {
		return "", false
	}
	in = strings.TrimSpace(in)
	if valid.MatchString(in) {
		return in, true
	}
	pages := links[provider]
	if provider == "tmdb" {
		pages = []*regexp.Regexp{tmdbLinks[kind]}
	}
	if !strings.Contains(in, "://") {
		in = "https://" + in
	}
	u, err := url.Parse(in)
	if err != nil || u.Host == "" {
		return "", false
	}
	where := strings.ToLower(u.Hostname()) + u.Path
	for _, p := range pages {
		if m := p.FindStringSubmatch(where); m != nil && valid.MatchString(m[1]) {
			return m[1], true
		}
	}
	return "", false
}

func Lists(kind, provider string) bool {
	prop, _ := property(kind, provider)
	return prop != ""
}
