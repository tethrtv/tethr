package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallsInFlightAreCapped(t *testing.T) {
	var now, most atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := now.Add(1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		now.Add(-1)
		_, _ = w.Write([]byte(`{"title":"Inception"}`))
	}))
	defer fake.Close()
	c := &Client{BaseURL: fake.URL}
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if _, err := c.Movie(context.Background(), 27205); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if most.Load() > inFlight {
		t.Errorf("%d calls to TMDB at once, want at most %d", most.Load(), inFlight)
	}
}
