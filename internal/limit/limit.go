package limit

import (
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Rule struct {
	Name   string
	Limit  int
	Period time.Duration
	Burst  int
}

type Limiter struct {
	rule    Rule
	mu      sync.Mutex
	buckets map[string]time.Time
	swept   time.Time
}

func New(rule Rule) *Limiter {
	return &Limiter{rule: rule, buckets: map[string]time.Time{}}
}

// Allow is a GCRA: each key keeps the time its allowance will be full again.
func (l *Limiter) Allow(key string, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.swept) >= time.Minute {
		for k, full := range l.buckets {
			if !full.After(now) {
				delete(l.buckets, k)
			}
		}
		l.swept = now
	}
	interval := l.rule.Period / time.Duration(l.rule.Limit)
	full := l.buckets[key]
	if full.Before(now) {
		full = now
	}
	full = full.Add(interval)
	if allowedAt := full.Add(-interval * time.Duration(l.rule.Burst)); now.Before(allowedAt) {
		return allowedAt.Sub(now), false
	}
	l.buckets[key] = full
	return 0, true
}

func Refuse(w http.ResponseWriter, rule Rule, wait time.Duration, now time.Time) {
	seconds := int((wait + time.Second - 1) / time.Second)
	info, _ := json.Marshal(struct {
		Name      string    `json:"name"`
		Period    int       `json:"period"`
		Limit     int       `json:"limit"`
		Remaining int       `json:"remaining"`
		Until     time.Time `json:"until"`
	}{rule.Name, int(rule.Period / time.Second), rule.Limit, 0, ceil(now.Add(wait)).UTC()})
	w.Header().Set("X-Ratelimit", string(info))
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}

func ceil(t time.Time) time.Time {
	if whole := t.Truncate(time.Second); whole.Before(t) {
		return whole.Add(time.Second)
	}
	return t
}

func Visitor(r *http.Request, header string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get(header)))
	if header == "" || err != nil {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		addr, err = netip.ParseAddr(host)
		if err != nil {
			return r.RemoteAddr
		}
	}
	addr = addr.Unmap()
	if addr.Is6() {
		p, _ := addr.Prefix(64)
		return p.String()
	}
	return addr.String()
}

type Limits struct {
	ipHeader                       string
	reads, writes, visits, signIns *Limiter
}

func Standard(ipHeader string) *Limits {
	return &Limits{
		ipHeader: ipHeader,
		reads:    New(Rule{Name: "AUTHED_API_GET_LIMIT", Limit: 1000, Period: 5 * time.Minute, Burst: 1000}),
		writes:   New(Rule{Name: "AUTHED_API_POST_LIMIT", Limit: 1, Period: time.Second, Burst: 10}),
		visits:   New(Rule{Name: "UNAUTHED_API_GET_LIMIT", Limit: 1000, Period: 5 * time.Minute, Burst: 1000}),
		signIns:  New(Rule{Name: "UNAUTHED_API_POST_LIMIT", Limit: 20, Period: time.Minute, Burst: 20}),
	}
}

func (l *Limits) Check(r *http.Request, accountID int64, now time.Time) (Rule, time.Duration, bool) {
	read := r.Method == http.MethodGet || r.Method == http.MethodHead
	limiter, key := l.visits, Visitor(r, l.ipHeader)
	switch {
	case accountID != 0 && read:
		limiter, key = l.reads, strconv.FormatInt(accountID, 10)
	case accountID != 0:
		limiter, key = l.writes, strconv.FormatInt(accountID, 10)
	case !read:
		limiter = l.signIns
	}
	wait, ok := limiter.Allow(key, now)
	return limiter.rule, wait, ok
}
