package version

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{"equal simple", "1.2.3", "1.2.3", 0},
		{"equal zero", "0.0.0", "0.0.0", 0},
		{"greater patch", "1.2.4", "1.2.3", 1},
		{"less patch", "1.2.3", "1.2.4", -1},
		{"greater minor", "1.3.0", "1.2.9", 1},
		{"less minor", "1.2.0", "1.3.0", -1},
		{"greater major", "2.0.0", "1.9.9", 1},
		{"less major", "1.9.9", "2.0.0", -1},
		{"different lengths equal", "1.2", "1.2.0", 0},
		{"different lengths equal reverse", "1.2.0", "1.2", 0},
		{"different lengths greater", "1.2.1", "1.2", 1},
		{"different lengths less", "1.2", "1.2.1", -1},
		{"multi-digit greater", "1.10", "1.9", 1},
		{"multi-digit less", "1.9", "1.10", -1},
		{"multi-digit patch", "1.0.10", "1.0.9", 1},
		{"single component greater", "2", "1", 1},
		{"single component equal", "5", "5", 0},
		{"empty vs empty", "", "", 0},
		{"empty vs version", "", "1.0.0", -1},
		{"version vs empty", "1.0.0", "", 1},
		{"long equal", "1.2.3.4", "1.2.3.4", 0},
		{"long vs short greater", "1.2.3.4", "1.2.3", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestGetAssetURL(t *testing.T) {
	c := NewChecker("1.0.0")
	expectedName := fmt.Sprintf("oxiwatch-%s-%s", runtime.GOOS, runtime.GOARCH)
	wantURL := "https://example.com/download/" + expectedName

	t.Run("asset present", func(t *testing.T) {
		release := &Release{
			TagName: "v1.0.0",
			Assets: []Asset{
				{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
				{Name: expectedName, BrowserDownloadURL: wantURL},
			},
		}
		got, err := c.GetAssetURL(release)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != wantURL {
			t.Errorf("GetAssetURL() = %q, want %q", got, wantURL)
		}
	})

	t.Run("asset absent", func(t *testing.T) {
		release := &Release{
			TagName: "v1.0.0",
			Assets: []Asset{
				{Name: "oxiwatch-other-platform", BrowserDownloadURL: "https://example.com/other"},
			},
		}
		got, err := c.GetAssetURL(release)
		if err == nil {
			t.Fatalf("expected error, got nil with url %q", got)
		}
		if got != "" {
			t.Errorf("expected empty url on error, got %q", got)
		}
	})

	t.Run("empty assets", func(t *testing.T) {
		release := &Release{TagName: "v1.0.0", Assets: nil}
		if _, err := c.GetAssetURL(release); err == nil {
			t.Fatal("expected error for empty assets, got nil")
		}
	})
}

func TestGetChecksumURL(t *testing.T) {
	c := NewChecker("1.0.0")
	wantURL := "https://example.com/checksums.txt"

	t.Run("checksums present", func(t *testing.T) {
		release := &Release{
			TagName: "v1.0.0",
			Assets: []Asset{
				{Name: "oxiwatch-linux-amd64", BrowserDownloadURL: "https://example.com/bin"},
				{Name: "checksums.txt", BrowserDownloadURL: wantURL},
			},
		}
		got, err := c.GetChecksumURL(release)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != wantURL {
			t.Errorf("GetChecksumURL() = %q, want %q", got, wantURL)
		}
	})

	t.Run("checksums absent", func(t *testing.T) {
		release := &Release{
			TagName: "v1.0.0",
			Assets: []Asset{
				{Name: "oxiwatch-linux-amd64", BrowserDownloadURL: "https://example.com/bin"},
			},
		}
		got, err := c.GetChecksumURL(release)
		if err == nil {
			t.Fatalf("expected error, got nil with url %q", got)
		}
		if got != "" {
			t.Errorf("expected empty url on error, got %q", got)
		}
	})

	t.Run("empty assets", func(t *testing.T) {
		release := &Release{TagName: "v1.0.0", Assets: nil}
		if _, err := c.GetChecksumURL(release); err == nil {
			t.Fatal("expected error for empty assets, got nil")
		}
	})
}

func TestFetchChecksums(t *testing.T) {
	t.Run("well-formed with blanks and whitespace", func(t *testing.T) {
		body := "abc123  oxiwatch-linux-amd64\n" +
			"\n" +
			"   def456   oxiwatch-darwin-arm64   \n" +
			"\t\n" +
			"  789ghi\toxiwatch-windows-amd64.exe\n" +
			"\n"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))
		defer srv.Close()

		c := NewChecker("1.0.0")
		got, err := c.fetchChecksums(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{
			"oxiwatch-linux-amd64":       "abc123",
			"oxiwatch-darwin-arm64":      "def456",
			"oxiwatch-windows-amd64.exe": "789ghi",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("fetchChecksums() = %#v, want %#v", got, want)
		}
	})

	t.Run("empty body yields empty map", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "\n\n   \n\t\n")
		}))
		defer srv.Close()

		c := NewChecker("1.0.0")
		got, err := c.fetchChecksums(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty map, got %#v", got)
		}
	})

	t.Run("malformed lines with single field are skipped", func(t *testing.T) {
		body := "abc123  oxiwatch-linux-amd64\nlonelytoken\n"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))
		defer srv.Close()

		c := NewChecker("1.0.0")
		got, err := c.fetchChecksums(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"oxiwatch-linux-amd64": "abc123"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("fetchChecksums() = %#v, want %#v", got, want)
		}
	})

	t.Run("extra fields keep first two", func(t *testing.T) {
		body := "abc123  oxiwatch-linux-amd64  extra ignored tokens\n"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))
		defer srv.Close()

		c := NewChecker("1.0.0")
		got, err := c.fetchChecksums(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"oxiwatch-linux-amd64": "abc123"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("fetchChecksums() = %#v, want %#v", got, want)
		}
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		c := NewChecker("1.0.0")
		if _, err := c.fetchChecksums(srv.URL); err == nil {
			t.Fatal("expected error on non-200 status, got nil")
		}
	})

	t.Run("unreachable url returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()

		c := NewChecker("1.0.0")
		if _, err := c.fetchChecksums(url); err == nil {
			t.Fatal("expected error on unreachable url, got nil")
		}
	})
}

func TestNewChecker(t *testing.T) {
	c := NewChecker("1.2.3")
	if c == nil {
		t.Fatal("NewChecker returned nil")
	}
	if c.currentVersion != "1.2.3" {
		t.Errorf("currentVersion = %q, want %q", c.currentVersion, "1.2.3")
	}
	if c.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
}
