package metrics

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	return New("srv", slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// readValue extracts the current scalar value of a single counter or gauge
// metric via its protobuf representation. This avoids the testutil package,
// whose transitive godebug import is not recorded in go.mod and would force a
// go.mod update at test time.
func readValue(t *testing.T, m prometheus.Metric) float64 {
	t.Helper()
	var pb dto.Metric
	if err := m.Write(&pb); err != nil {
		t.Fatalf("Write metric: %v", err)
	}
	switch {
	case pb.Counter != nil:
		return pb.Counter.GetValue()
	case pb.Gauge != nil:
		return pb.Gauge.GetValue()
	default:
		t.Fatalf("metric has neither counter nor gauge value: %+v", &pb)
		return 0
	}
}

func TestNewIsRepeatable(t *testing.T) {
	// New() uses a private registry, so calling it multiple times must not
	// panic with a duplicate-registration error.
	for i := 0; i < 3; i++ {
		if m := newTestMetrics(t); m == nil {
			t.Fatalf("New returned nil on iteration %d", i)
		}
	}
}

func TestRecordAttempt(t *testing.T) {
	tests := []struct {
		name        string
		success     bool
		method      string
		country     string
		invalidUser bool

		wantResult       string // label on loginAttempts
		wantMethod       string
		wantCountry      string
		wantLoginCount   float64
		wantCountryCount float64
		wantInvalidCount float64
	}{
		{
			name:             "success with method and country",
			success:          true,
			method:           "publickey",
			country:          "US",
			invalidUser:      false,
			wantResult:       "success",
			wantMethod:       "publickey",
			wantCountry:      "US",
			wantLoginCount:   1,
			wantCountryCount: 1,
			wantInvalidCount: 0,
		},
		{
			name:             "failure with password",
			success:          false,
			method:           "password",
			country:          "DE",
			invalidUser:      false,
			wantResult:       "failure",
			wantMethod:       "password",
			wantCountry:      "DE",
			wantLoginCount:   1,
			wantCountryCount: 1,
			wantInvalidCount: 0,
		},
		{
			name:             "empty method defaults to unknown",
			success:          true,
			method:           "",
			country:          "FR",
			invalidUser:      false,
			wantResult:       "success",
			wantMethod:       "unknown",
			wantCountry:      "FR",
			wantLoginCount:   1,
			wantCountryCount: 1,
			wantInvalidCount: 0,
		},
		{
			name:             "empty country defaults to unknown",
			success:          false,
			method:           "password",
			country:          "",
			invalidUser:      false,
			wantResult:       "failure",
			wantMethod:       "password",
			wantCountry:      "unknown",
			wantLoginCount:   1,
			wantCountryCount: 1,
			wantInvalidCount: 0,
		},
		{
			name:             "invalid user increments invalid counter",
			success:          false,
			method:           "password",
			country:          "RU",
			invalidUser:      true,
			wantResult:       "failure",
			wantMethod:       "password",
			wantCountry:      "RU",
			wantLoginCount:   1,
			wantCountryCount: 1,
			wantInvalidCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestMetrics(t)
			m.RecordAttempt(tt.success, tt.method, tt.country, tt.invalidUser)

			if got := readValue(t, m.loginAttempts.WithLabelValues(tt.wantResult, tt.wantMethod)); got != tt.wantLoginCount {
				t.Errorf("loginAttempts{result=%q,method=%q} = %v, want %v",
					tt.wantResult, tt.wantMethod, got, tt.wantLoginCount)
			}
			if got := readValue(t, m.countryAttempts.WithLabelValues(tt.wantCountry)); got != tt.wantCountryCount {
				t.Errorf("countryAttempts{country=%q} = %v, want %v",
					tt.wantCountry, got, tt.wantCountryCount)
			}
			if got := readValue(t, m.invalidUser); got != tt.wantInvalidCount {
				t.Errorf("invalidUser = %v, want %v", got, tt.wantInvalidCount)
			}
		})
	}
}

func TestRecordAttemptAccumulates(t *testing.T) {
	m := newTestMetrics(t)

	m.RecordAttempt(true, "publickey", "US", false)
	m.RecordAttempt(true, "publickey", "US", false)
	m.RecordAttempt(false, "password", "US", true)

	if got := readValue(t, m.loginAttempts.WithLabelValues("success", "publickey")); got != 2 {
		t.Errorf("success/publickey = %v, want 2", got)
	}
	if got := readValue(t, m.loginAttempts.WithLabelValues("failure", "password")); got != 1 {
		t.Errorf("failure/password = %v, want 1", got)
	}
	if got := readValue(t, m.countryAttempts.WithLabelValues("US")); got != 3 {
		t.Errorf("country US = %v, want 3", got)
	}
	if got := readValue(t, m.invalidUser); got != 1 {
		t.Errorf("invalidUser = %v, want 1", got)
	}
}

func TestSetBuildInfo(t *testing.T) {
	m := newTestMetrics(t)
	m.SetBuildInfo("v1.2.3")

	if got := readValue(t, m.buildInfo.WithLabelValues("v1.2.3")); got != 1 {
		t.Errorf("buildInfo{version=v1.2.3} = %v, want 1", got)
	}
}

func TestMarkStart(t *testing.T) {
	m := newTestMetrics(t)
	now := time.Now()
	m.MarkStart(now)

	got := readValue(t, m.startTime)
	if got <= 0 {
		t.Errorf("startTime = %v, want > 0", got)
	}
	if want := float64(now.Unix()); got != want {
		t.Errorf("startTime = %v, want %v", got, want)
	}
}

func TestMetricsHandlerOutput(t *testing.T) {
	m := newTestMetrics(t)
	m.RecordAttempt(true, "publickey", "US", false)

	handler := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)

	if !strings.Contains(text, "oxiwatch_ssh_login_attempts_total") {
		t.Errorf("body missing oxiwatch_ssh_login_attempts_total\n%s", text)
	}
	if !strings.Contains(text, `server="srv"`) {
		t.Errorf("body missing server=\"srv\" const label\n%s", text)
	}
}

func TestStartBindError(t *testing.T) {
	m := newTestMetrics(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// An obviously invalid address should produce a bind error synchronously.
	if err := m.Start(ctx, "256.256.256.256:0"); err == nil {
		t.Fatal("Start: expected bind error for invalid address, got nil")
	}
}

func TestStartSucceedsAndShutsDown(t *testing.T) {
	m := newTestMetrics(t)

	ctx, cancel := context.WithCancel(context.Background())

	// Bind to an ephemeral loopback port; Start opens the socket synchronously.
	if err := m.Start(ctx, "127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if m.server == nil {
		t.Fatal("Start: server not set")
	}

	// Cancelling the context triggers a background graceful shutdown.
	cancel()

	// Give the shutdown goroutine a moment to run.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := m.server.Shutdown(context.Background()); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
