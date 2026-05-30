package handler

import (
	"net/http"
	"net/url"
	"testing"
)

func TestNormalizeVercelPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/api/dev", "/dev"},
		{"/api/dev/leaderboard", "/dev/leaderboard"},
		{"/api/health", "/health"},
		{"/api/api/events", "/api/events"},
		{"/api/api/dev/leaderboard", "/api/dev/leaderboard"},
		{"/api/embed/0xabc", "/embed/0xabc"},
	}
	for _, c := range cases {
		r := &http.Request{URL: mustParse(c.in)}
		normalizeVercelPath(r)
		if r.URL.Path != c.want {
			t.Fatalf("%q -> %q, want %q", c.in, r.URL.Path, c.want)
		}
	}
}

func mustParse(path string) *url.URL {
	u, _ := url.Parse("http://example.com" + path)
	return u
}
