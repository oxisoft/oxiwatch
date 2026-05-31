// Package metrics exposes OxiWatch runtime data as Prometheus metrics over an
// HTTP endpoint. Every oxiwatch_* series carries a constant "server" label so a
// single Prometheus instance can scrape many hosts and tell them apart.
package metrics

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the collectors and the exporter HTTP server.
type Metrics struct {
	logger   *slog.Logger
	server   *http.Server
	registry *prometheus.Registry

	loginAttempts   *prometheus.CounterVec // labels: result, method
	invalidUser     prometheus.Counter
	countryAttempts *prometheus.CounterVec // labels: country
	buildInfo       *prometheus.GaugeVec   // labels: version
	startTime       prometheus.Gauge
}

// New registers the collectors on a private registry (so it is safe to call
// more than once, e.g. in tests). serverName is attached as a constant
// "server" label to every oxiwatch_* metric.
func New(serverName string, logger *slog.Logger) *Metrics {
	cl := prometheus.Labels{"server": serverName}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	promauto := promauto.With(reg)

	return &Metrics{
		logger:   logger,
		registry: reg,
		loginAttempts: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "oxiwatch_ssh_login_attempts_total",
			Help:        "Total SSH authentication attempts observed, by result and method.",
			ConstLabels: cl,
		}, []string{"result", "method"}),
		invalidUser: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "oxiwatch_ssh_invalid_user_attempts_total",
			Help:        "Total SSH attempts for non-existent (invalid) users.",
			ConstLabels: cl,
		}),
		countryAttempts: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "oxiwatch_ssh_attempts_by_country_total",
			Help:        "Total SSH attempts by source country (GeoIP); \"unknown\" when not resolved.",
			ConstLabels: cl,
		}, []string{"country"}),
		buildInfo: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "oxiwatch_build_info",
			Help:        "Build information; the value is always 1.",
			ConstLabels: cl,
		}, []string{"version"}),
		startTime: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "oxiwatch_start_time_seconds",
			Help:        "Unix timestamp of when the OxiWatch daemon started.",
			ConstLabels: cl,
		}),
	}
}

// SetBuildInfo records the running version.
func (m *Metrics) SetBuildInfo(version string) {
	m.buildInfo.WithLabelValues(version).Set(1)
}

// MarkStart records the daemon start time.
func (m *Metrics) MarkStart(t time.Time) {
	m.startTime.Set(float64(t.Unix()))
}

// RecordAttempt records a single observed SSH authentication attempt.
func (m *Metrics) RecordAttempt(success bool, method, country string, invalidUser bool) {
	result := "failure"
	if success {
		result = "success"
	}
	if method == "" {
		method = "unknown"
	}
	m.loginAttempts.WithLabelValues(result, method).Inc()

	if country == "" {
		country = "unknown"
	}
	m.countryAttempts.WithLabelValues(country).Inc()

	if invalidUser {
		m.invalidUser.Inc()
	}
}

// Start binds the exporter to addr and serves /metrics. The listen socket is
// opened synchronously so a bind failure is returned to the caller; requests
// are then served in the background until ctx is cancelled.
func (m *Metrics) Start(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("OxiWatch Prometheus exporter\nMetrics: /metrics\n"))
	})
	m.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		if err := m.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			m.logger.Error("metrics server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.server.Shutdown(shutdownCtx)
	}()

	return nil
}
