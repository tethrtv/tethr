package limit

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	start := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	l := New(Rule{Name: "AUTHED_API_POST_LIMIT", Limit: 1, Period: time.Second, Burst: 3})

	for i := range 3 {
		if _, ok := l.Allow("a", start); !ok {
			t.Fatalf("call %d refused within the burst", i+1)
		}
	}
	wait, ok := l.Allow("a", start)
	if ok || wait != time.Second {
		t.Fatalf("fourth call = %v, %v; want refused for 1s", wait, ok)
	}
	if _, ok := l.Allow("b", start); !ok {
		t.Error("another key was refused")
	}
	if _, ok := l.Allow("a", start.Add(500*time.Millisecond)); ok {
		t.Error("allowed before a call had refilled")
	}
	if _, ok := l.Allow("a", start.Add(time.Second)); !ok {
		t.Error("refused after a call had refilled")
	}
	for i := range 3 {
		if _, ok := l.Allow("a", start.Add(time.Hour)); !ok {
			t.Fatalf("call %d refused after an idle hour", i+1)
		}
	}
}

func TestLimiterSpreadsTheLimitOverThePeriod(t *testing.T) {
	start := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	l := New(Rule{Name: "AUTHED_API_GET_LIMIT", Limit: 1000, Period: 5 * time.Minute, Burst: 1000})
	for range 1000 {
		l.Allow("a", start)
	}
	wait, ok := l.Allow("a", start)
	if ok || wait != 300*time.Millisecond {
		t.Fatalf("over the limit = %v, %v; want refused for 300ms", wait, ok)
	}
}

func TestLimiterForgetsIdleKeys(t *testing.T) {
	start := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	l := New(Rule{Name: "SIGN_IN_LIMIT", Limit: 20, Period: time.Minute, Burst: 20})
	l.Allow("a", start)
	l.Allow("b", start.Add(10*time.Minute))
	if _, kept := l.buckets["a"]; kept {
		t.Error("an idle key was kept")
	}
	if _, kept := l.buckets["b"]; !kept {
		t.Error("a busy key was dropped")
	}
}

func TestRefuse(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	rec := httptest.NewRecorder()
	Refuse(rec, Rule{Name: "AUTHED_API_GET_LIMIT", Limit: 1000, Period: 5 * time.Minute}, 300*time.Millisecond, now)
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q", got)
	}
	want := `{"name":"AUTHED_API_GET_LIMIT","period":300,"limit":1000,"remaining":0,"until":"2026-11-05T12:00:01Z"}`
	if got := rec.Header().Get("X-Ratelimit"); got != want {
		t.Errorf("X-Ratelimit = %s", got)
	}

	rec = httptest.NewRecorder()
	Refuse(rec, Rule{Name: "AUTHED_API_POST_LIMIT", Limit: 1, Period: time.Second}, 600*time.Millisecond, now.Add(700*time.Millisecond))
	if got := rec.Header().Get("X-Ratelimit"); !strings.HasSuffix(got, `"until":"2026-11-05T12:00:02Z"}`) {
		t.Errorf("X-Ratelimit = %s", got)
	}
}

func TestVisitor(t *testing.T) {
	for _, c := range []struct {
		header, remote, sent, want string
	}{
		{"", "203.0.113.9:5123", "198.51.100.1", "203.0.113.9"},
		{"CF-Connecting-IP", "127.0.0.1:5123", "198.51.100.1", "198.51.100.1"},
		{"CF-Connecting-IP", "127.0.0.1:5123", "", "127.0.0.1"},
		{"CF-Connecting-IP", "127.0.0.1:5123", "2001:db8:1:2:3:4:5:6", "2001:db8:1:2::/64"},
		{"", "[2001:db8:1:2:3:4:5:6]:5123", "", "2001:db8:1:2::/64"},
	} {
		r := httptest.NewRequest("GET", "/v1/me", nil)
		r.RemoteAddr = c.remote
		if c.sent != "" {
			r.Header.Set("CF-Connecting-IP", c.sent)
		}
		if got := Visitor(r, c.header); got != c.want {
			t.Errorf("Visitor(%q, %q, %q) = %q, want %q", c.header, c.remote, c.sent, got, c.want)
		}
	}
}

func TestLimits(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	l := Standard("")
	call := func(method string, accountID int64, remote string) (string, bool) {
		r := httptest.NewRequest(method, "/v1/history", nil)
		r.RemoteAddr = remote
		rule, _, ok := l.Check(r, accountID, now)
		return rule.Name, ok
	}

	for range 10 {
		call("POST", 1, "203.0.113.9:1")
	}
	if name, ok := call("POST", 1, "198.51.100.1:1"); ok || name != "AUTHED_API_POST_LIMIT" {
		t.Errorf("eleventh write = %s, %v", name, ok)
	}
	if _, ok := call("GET", 1, "203.0.113.9:1"); !ok {
		t.Error("a read was refused after the writes ran out")
	}
	if _, ok := call("POST", 2, "203.0.113.9:1"); !ok {
		t.Error("another account was refused")
	}

	for range 20 {
		call("POST", 0, "203.0.113.9:1")
	}
	if name, ok := call("POST", 0, "203.0.113.9:1"); ok || name != "UNAUTHED_API_POST_LIMIT" {
		t.Errorf("twenty-first sign-in = %s, %v", name, ok)
	}
	if _, ok := call("POST", 0, "198.51.100.1:1"); !ok {
		t.Error("another address was refused")
	}
	if _, ok := call("GET", 0, "203.0.113.9:1"); !ok {
		t.Error("a read without sign-in was refused")
	}
}
