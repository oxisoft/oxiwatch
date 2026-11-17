package version

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
)

// redirectTransport sends every request (incl. the hard-coded GitHub API URL)
// to the fake server, keeping the path.
type redirectTransport struct{ target *url.URL }

func (rt redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme = rt.target.Scheme
	r2.URL.Host = rt.target.Host
	r2.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(r2)
}

type fakeGitHub struct {
	tag        string
	status     int // for the API call; 0 = 200
	badJSON    bool
	withSums   bool
	sums       string // checksums.txt body; empty = computed from binary
	sumsStatus int
	binary     []byte
	binStatus  int
	withBinary bool
	gotUA      string
	gotAccept  string
}

func (f *fakeGitHub) start(t *testing.T) *Checker {
	t.Helper()
	asset := fmt.Sprintf("oxiwatch-%s-%s", runtime.GOOS, runtime.GOARCH)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/oxisoft/oxiwatch/releases/latest":
			f.gotUA, f.gotAccept = r.Header.Get("User-Agent"), r.Header.Get("Accept")
			if f.status != 0 {
				w.WriteHeader(f.status)
				return
			}
			if f.badJSON {
				w.Write([]byte("{not json"))
				return
			}
			rel := Release{TagName: f.tag}
			if f.withSums {
				rel.Assets = append(rel.Assets, Asset{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/dl/checksums.txt"})
			}
			if f.withBinary {
				rel.Assets = append(rel.Assets, Asset{Name: asset, BrowserDownloadURL: srv.URL + "/dl/" + asset})
			}
			json.NewEncoder(w).Encode(rel)
		case "/dl/checksums.txt":
			if f.sumsStatus != 0 {
				w.WriteHeader(f.sumsStatus)
				return
			}
			body := f.sums
			if body == "" {
				h := sha256.Sum256(f.binary)
				body = hex.EncodeToString(h[:]) + "  " + asset + "\n"
			}
			w.Write([]byte(body))
		case "/dl/" + asset:
			if f.binStatus != 0 {
				w.WriteHeader(f.binStatus)
				return
			}
			w.Write(f.binary)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	c := NewChecker("0.4.4")
	c.httpClient = &http.Client{Transport: redirectTransport{target}}
	return c
}

func TestGetLatestRelease(t *testing.T) {
	f := &fakeGitHub{tag: "v0.5.0"}
	c := f.start(t)
	rel, err := c.GetLatestRelease()
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if rel.TagName != "v0.5.0" {
		t.Errorf("tag = %q", rel.TagName)
	}
	if f.gotUA != "oxiwatch/0.4.4" || !strings.Contains(f.gotAccept, "github") {
		t.Errorf("headers UA=%q Accept=%q", f.gotUA, f.gotAccept)
	}

	if _, err := (&fakeGitHub{status: http.StatusForbidden}).start(t).GetLatestRelease(); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("rate limited: err = %v, want status 403", err)
	}
	if _, err := (&fakeGitHub{badJSON: true}).start(t).GetLatestRelease(); err == nil {
		t.Error("bad JSON: want error")
	}
	c = NewChecker("1.0.0")
	c.httpClient = &http.Client{Transport: redirectTransport{&url.URL{Scheme: "http", Host: "127.0.0.1:1"}}}
	if _, err := c.GetLatestRelease(); err == nil {
		t.Error("unreachable: want error")
	}
}

func TestIsUpdateAvailable(t *testing.T) {
	cases := []struct {
		current, tag string
		want         bool
	}{
		{"0.4.4", "v0.5.0", true},
		{"v0.4.4", "v0.4.4", false},
		{"0.4.4", "v0.4.3", false},
		{"dev", "v0.1.0", true},
	}
	for _, tc := range cases {
		c := (&fakeGitHub{tag: tc.tag}).start(t)
		c.currentVersion = tc.current
		got, latest, err := c.IsUpdateAvailable()
		if err != nil || got != tc.want || latest != strings.TrimPrefix(tc.tag, "v") {
			t.Errorf("%s vs %s: got %v %q %v, want %v", tc.current, tc.tag, got, latest, err, tc.want)
		}
	}
	if _, _, err := (&fakeGitHub{status: 500}).start(t).IsUpdateAvailable(); err == nil {
		t.Error("API error must propagate")
	}
}

func TestUpgrade_Refusals(t *testing.T) {
	cases := []struct {
		name    string
		f       *fakeGitHub
		current string
		wantErr string
	}{
		{"api down", &fakeGitHub{status: 502}, "0.4.4", "fetch latest release"},
		{"already latest", &fakeGitHub{tag: "v0.4.4"}, "0.4.4", "already at latest"},
		{"no checksums asset", &fakeGitHub{tag: "v0.5.0", withBinary: true}, "0.4.4", "checksum URL"},
		{"checksums 404", &fakeGitHub{tag: "v0.5.0", withSums: true, withBinary: true, sumsStatus: 404}, "0.4.4", "fetch checksums"},
		{"no checksum for this platform", &fakeGitHub{tag: "v0.5.0", withSums: true, withBinary: true, sums: "abc  oxiwatch-plan9-mips\n"}, "0.4.4", "no checksum found"},
		{"no binary for this platform", &fakeGitHub{tag: "v0.5.0", withSums: true, binary: []byte("x")}, "dev", "no binary found"},
		{"download 404", &fakeGitHub{tag: "v0.5.0", withSums: true, withBinary: true, binary: []byte("x"), binStatus: 404}, "0.4.4", "download failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.f.start(t)
			c.currentVersion = tc.current
			err := c.Upgrade(true)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Upgrade err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// A binary whose hash does not match checksums.txt must never replace the
// running executable (the test binary itself here).
func TestUpgrade_ChecksumMismatchKeepsBinary(t *testing.T) {
	asset := fmt.Sprintf("oxiwatch-%s-%s", runtime.GOOS, runtime.GOARCH)
	f := &fakeGitHub{tag: "v9.9.9", withSums: true, withBinary: true,
		binary: []byte("tampered"), sums: strings.Repeat("0", 64) + "  " + asset + "\n"}
	c := f.start(t)
	err := c.Upgrade(false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Upgrade err = %v, want checksum mismatch", err)
	}
}
