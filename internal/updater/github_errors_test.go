package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	syncatomic "sync/atomic"
	"testing"
	"time"
)

func TestGitHubResponseErrorsDistinguishLimitsFromAccess(t *testing.T) {
	now := time.Date(2026, 10, 26, 11, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		status     int
		headers    http.Header
		body, want string
		wait       time.Duration
	}{
		{"primary", 403, http.Header{"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {fmt.Sprint(now.Add(20 * time.Minute).Unix())}}, `{"message":"API rate limit exceeded for private-address"}`, "11:20:00 UTC", 20 * time.Minute},
		{"secondary", 403, http.Header{"Retry-After": {"120"}}, `{"message":"secondary rate limit"}`, "Abfragelimit", 2 * time.Minute},
		{"retry date", 429, http.Header{"Retry-After": {now.Add(3 * time.Minute).Format(http.TimeFormat)}}, "", "11:03:00 UTC", 3 * time.Minute},
		{"later reset", 429, http.Header{"Retry-After": {"60"}, "X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {fmt.Sprint(now.Add(10 * time.Minute).Unix())}}, "", "11:10:00 UTC", 10 * time.Minute},
		{"secondary fallback", 403, nil, `{"message":"You have exceeded a secondary rate limit. private-address fixture-token"}`, "Abfragelimit", time.Minute},
		{"malformed limit headers", 429, http.Header{"Retry-After": {"99999999999999999999"}, "X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {"invalid"}}, "", "Abfragelimit", time.Minute},
		{"access", 403, nil, `{"message":"private-address fixture-token"}`, "Repository-Berechtigung", 0},
		{"bad token", 401, nil, `{"message":"fixture-token"}`, "Zugangsdaten ungültig", 0},
		{"not found", 404, nil, "", "Repository oder Release nicht gefunden", 0},
		{"server error", 502, nil, "", "HTTP 502", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.status, Header: tc.headers, Body: io.NopCloser(strings.NewReader(tc.body))}
			err, retry := githubResponseError(resp, now)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(err.Error(), "private-address") || strings.Contains(err.Error(), "fixture-token") {
				t.Fatal("raw GitHub diagnostics leaked")
			}
			if tc.wait == 0 {
				if !retry.IsZero() {
					t.Fatal("access error incorrectly cached as a rate limit")
				}
			} else if !retry.Equal(now.Add(tc.wait)) {
				t.Fatalf("retry %v, expected %v", retry, now.Add(tc.wait))
			}
		})
	}
}

func TestGitHubRateLimitCooldownAvoidsRepeatedRequests(t *testing.T) {
	var calls syncatomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	}))
	defer server.Close()
	g := NewGitHub(Config{Repository: "example/anker"}, "")
	g.base = server.URL
	g.HTTP = server.Client()
	_, first := g.fetch(context.Background(), "/releases/latest", "application/json", 1024)
	_, second := g.fetch(context.Background(), "/releases/latest", "application/json", 1024)
	if first == nil || second == nil || first.Error() != second.Error() || calls.Load() != 1 {
		t.Fatalf("cooldown failed: calls=%d first=%v second=%v", calls.Load(), first, second)
	}
	g.rateMu.Lock()
	g.retryAt = time.Now().Add(-time.Second)
	g.rateMu.Unlock()
	_, err := g.fetch(context.Background(), "/releases/latest", "application/json", 1024)
	if err == nil || calls.Load() != 2 {
		t.Fatalf("request not resumed after cooldown: %v", err)
	}
}
