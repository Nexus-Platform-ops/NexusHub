// Prometheus metrics for the API. Every Server owns its own registry so
// tests can build many servers in one process without duplicate-registration
// panics; production scrapes the registry through Metrics.Handler() on the
// admin port (METRICS_ADDR).
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics bundles every Nexus metric. Label values are drawn from fixed
// vocabularies (route patterns, severities) so cardinality stays bounded.
type Metrics struct {
	reg *prometheus.Registry

	httpDuration         *prometheus.HistogramVec
	feedDuration         prometheus.Histogram
	radarPushDuration    prometheus.Histogram
	rankDuration         prometheus.Histogram
	whisperWipeDuration  prometheus.Histogram
	safetyReportDuration *prometheus.HistogramVec

	// SafetyHighOpenSeconds is the age in seconds of the oldest still-open
	// HIGH-severity abuse report; 0 when the queue is clear. Backed by the
	// RunSafetyGauge poller (api.Server), not by request handling.
	SafetyHighOpenSeconds prometheus.Gauge
}

// NewMetrics builds a registry with the Go/process collectors plus the
// Nexus histograms and gauges.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	m := &Metrics{
		reg: reg,
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "nexus", Subsystem: "http",
			Name: "request_duration_seconds",
			Help: "HTTP request latency by route pattern, method and status code.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "method", "code"}),
		feedDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nexus",
			Name:      "feed_duration_seconds",
			Help:      "Latency of the social feed listing handler.",
			Buckets:   prometheus.DefBuckets,
		}),
		radarPushDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nexus",
			Name:      "radar_push_duration_seconds",
			Help: "Latency of the full radar response assembly (presence upsert, " +
				"friend/block suppression, proximity ranking and k-anonymity gate). " +
				"The monolith has no separate push transport; this is the radar hot path.",
			Buckets: prometheus.DefBuckets,
		}),
		rankDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nexus",
			Name:      "rank_duration_seconds",
			Help:      "Latency of the radar proximity-ranking query (ST_DWithin + nearest-neighbour ordering).",
			Buckets:   prometheus.DefBuckets,
		}),
		whisperWipeDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "nexus",
			Name:      "whisper_wipe_duration_seconds",
			Help:      "Latency of the whisper read-and-wipe region (fetch, recipient check, Redis delete).",
			Buckets:   prometheus.DefBuckets,
		}),
		safetyReportDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "nexus",
			Name:      "safety_report_duration_seconds",
			Help:      "Latency of abuse-report intake, labelled by mapped severity.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"severity"}),
		SafetyHighOpenSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "nexus",
			Name:      "safety_high_open_seconds",
			Help:      "Age of the oldest open HIGH-severity abuse report in seconds (0 = queue clear).",
		}),
	}
	reg.MustRegister(m.httpDuration, m.feedDuration, m.radarPushDuration,
		m.rankDuration, m.whisperWipeDuration, m.safetyReportDuration,
		m.SafetyHighOpenSeconds)
	return m
}

// Handler exposes the registry for Prometheus scraping.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// ObserveHTTP records one request. route must be the ServeMux pattern (or
// "unmatched"); code is the response status.
func (m *Metrics) ObserveHTTP(route, method string, d time.Duration, code int) {
	m.httpDuration.WithLabelValues(route, method, strconv.Itoa(code)).Observe(d.Seconds())
}

func (m *Metrics) ObserveFeed(d time.Duration)              { m.feedDuration.Observe(d.Seconds()) }
func (m *Metrics) ObserveRadarPush(d time.Duration)         { m.radarPushDuration.Observe(d.Seconds()) }
func (m *Metrics) ObserveRank(d time.Duration)              { m.rankDuration.Observe(d.Seconds()) }
func (m *Metrics) ObserveWhisperWipe(d time.Duration)      { m.whisperWipeDuration.Observe(d.Seconds()) }
func (m *Metrics) ObserveSafetyReport(severity string, d time.Duration) {
	m.safetyReportDuration.WithLabelValues(severity).Observe(d.Seconds())
}
