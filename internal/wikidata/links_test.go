package wikidata

import "testing"

func TestOutsideID(t *testing.T) {
	for _, c := range []struct {
		kind, provider, in, want string
	}{
		{"movie", "imdb", "tt0094625", "tt0094625"},
		{"movie", "imdb", " https://www.imdb.com/title/tt0094625/?ref_=nv_sr_srsg_0 ", "tt0094625"},
		{"movie", "imdb", "m.imdb.com/title/tt0094625/reviews", "tt0094625"},
		{"movie", "tmdb", "https://www.themoviedb.org/movie/149-akira", "149"},
		{"show", "tmdb", "https://www.themoviedb.org/tv/1396-breaking-bad/season/1", "1396"},
		{"movie", "tmdb", "https://www.themoviedb.org/tv/1396-breaking-bad", ""},
		{"show", "tvmaze", "https://www.tvmaze.com/shows/169/breaking-bad", "169"},
		{"show", "tvdb", "81189", "81189"},
		{"show", "tvdb", "https://thetvdb.com/series/breaking-bad", ""},
		{"movie", "letterboxd", "https://letterboxd.com/film/akira/reviews/", "akira"},
		{"show", "letterboxd", "https://letterboxd.com/film/akira/", ""},
		{"movie", "letterboxd", "https://www.imdb.com/title/tt0094625/", ""},
		{"movie", "rottentomatoes", "https://www.rottentomatoes.com/m/akira", "m/akira"},
		{"show", "metacritic", "https://www.metacritic.com/tv/breaking-bad/", "tv/breaking-bad"},
		{"movie", "netflix", "https://www.netflix.com/gb/title/70101234?s=i", "70101234"},
		{"show", "mal", "https://myanimelist.net/anime/30/Neon_Genesis_Evangelion", "30"},
		{"show", "anilist", "https://anilist.co/anime/30/Neon-Genesis-Evangelion/", "30"},
		{"show", "anidb", "https://anidb.net/anime/22", "22"},
		{"movie", "appletv", "https://tv.apple.com/gb/movie/akira/umc.cmc.4lx2g2b7flcjlc5ux9xvsxmqy", "umc.cmc.4lx2g2b7flcjlc5ux9xvsxmqy"},
		{"movie", "primevideo", "https://www.primevideo.com/detail/0QSZ4XJWBV8RZF2Z1OOJ9TQ3VJ/ref=atv_sr", "0QSZ4XJWBV8RZF2Z1OOJ9TQ3VJ"},
		{"movie", "primevideo", "https://www.amazon.co.uk/gp/video/detail/B07FKWJ1RZ/", "B07FKWJ1RZ"},
		{"show", "disneyplus", "https://www.disneyplus.com/series/the-simpsons/3ZoBZ52QHb4x", "3ZoBZ52QHb4x"},
		{"movie", "plex", "5d7768265af944001f1f6a4c", "5d7768265af944001f1f6a4c"},
		{"movie", "imdb", "akira", ""},
		{"movie", "imdb", "https://evil.example/title/tt0094625/", ""},
		{"movie", "nowhere", "tt0094625", ""},
	} {
		got, ok := OutsideID(c.kind, c.provider, c.in)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("OutsideID(%s, %s, %q) = %q, %v, want %q", c.kind, c.provider, c.in, got, ok, c.want)
		}
	}
}
