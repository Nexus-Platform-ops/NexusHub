package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryOp(t *testing.T) {
	cases := map[string]string{
		"SELECT 1":              "select",
		"  INSERT INTO x":       "insert",
		"\nUPDATE\nfoo":         "update",
		"DELETE FROM social.y":  "delete",
		"":                      "query",
		"WITH c AS (SELECT 1)":  "with",
		"select":                "select",
	}
	for in, want := range cases {
		if got := queryOp(in); got != want {
			t.Errorf("queryOp(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstTable(t *testing.T) {
	cases := map[string]string{
		"SELECT 1 FROM social.users WHERE id=$1":  "social.users",
		"INSERT INTO dating.swipes (a) VALUES ($1)": "dating.swipes",
		"UPDATE social.reports SET status=$1":       "social.reports",
		"SELECT * FROM a JOIN dating.matches m ON m.x=a.y": "a",
		"SELECT 1":                                "",
	}
	for in, want := range cases {
		if got := firstTable(in); got != want {
			t.Errorf("firstTable(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMiddlewareLabelsByPatternAndCode(t *testing.T) {
	m := NewMetrics()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/posts/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	srv := httptest.NewServer(m.Middleware(mux))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/posts/42") //nolint:noctx // test
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	body := scrape(t, m)
	if !strings.Contains(body, `nexus_http_request_duration_seconds_count{code="418",method="GET",route="GET /api/v1/posts/{id}"} 1`) {
		t.Errorf("missing route-labelled count:\n%s", body)
	}
}

func TestMiddlewareUnmatchedRoute(t *testing.T) {
	m := NewMetrics()
	srv := httptest.NewServer(m.Middleware(http.NewServeMux()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nope") //nolint:noctx // test
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()

	body := scrape(t, m)
	if !strings.Contains(body, `route="unmatched"`) {
		t.Errorf("missing unmatched label:\n%s", body)
	}
}

func TestDomainHistogramsAndGauge(t *testing.T) {
	m := NewMetrics()
	m.ObserveFeed(120 * time.Millisecond)
	m.ObserveRadarPush(80 * time.Millisecond)
	m.ObserveRank(40 * time.Millisecond)
	m.ObserveWhisperWipe(5 * time.Millisecond)
	m.ObserveSafetyReport("high", 30*time.Millisecond)
	m.ObserveSafetyReport("normal", 10*time.Millisecond)
	m.SafetyHighOpenSeconds.Set(900)

	body := scrape(t, m)
	for _, want := range []string{
		"nexus_feed_duration_seconds_count 1",
		"nexus_radar_push_duration_seconds_count 1",
		"nexus_rank_duration_seconds_count 1",
		"nexus_whisper_wipe_duration_seconds_count 1",
		`nexus_safety_report_duration_seconds_count{severity="high"} 1`,
		`nexus_safety_report_duration_seconds_count{severity="normal"} 1`,
		"nexus_safety_high_open_seconds 900",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q:\n%s", want, body)
		}
	}
}

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/metrics") //nolint:noctx // test
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}
