package geoip

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func dbipPath(t time.Time) string {
	return fmt.Sprintf("/free/dbip-city-lite-%d-%02d.mmdb.gz", t.Year(), int(t.Month()))
}

func TestResolver_SyntheticDB(t *testing.T) {
	r, err := NewResolver(writeTestMMDB(t, "Poland", "Bialystok"))
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	defer r.Close()

	loc, err := r.Lookup("83.6.42.41")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if loc.Country != "Poland" || loc.City != "Bialystok" {
		t.Errorf("Lookup = %+v, want Poland/Bialystok", loc)
	}
	// IPv4-mapped IPv6 must resolve like the plain IPv4 address.
	if loc, _ := r.Lookup("::ffff:83.6.42.41"); loc.Country != "Poland" {
		t.Errorf("mapped Lookup = %+v, want Poland", loc)
	}
	// An unparsable IP is not an error: the alert still goes out, without a location.
	if loc, err := r.Lookup("not-an-ip"); err != nil || *loc != (Location{}) {
		t.Errorf("Lookup(bad) = %+v, %v; want empty, nil", loc, err)
	}

	raw, err := r.LookupRaw("1.2.3.4")
	if err != nil {
		t.Fatalf("LookupRaw: %v", err)
	}
	if _, ok := raw["country"]; !ok {
		t.Errorf("LookupRaw missing country: %v", raw)
	}
	if _, err := r.LookupRaw("nope"); err == nil {
		t.Error("LookupRaw(bad) = nil error, want error")
	}
	if r.DatabaseType() != "OxiWatch-Test" {
		t.Errorf("DatabaseType = %q", r.DatabaseType())
	}
	if r.BuildTime().Unix() != 1700000000 {
		t.Errorf("BuildTime = %v", r.BuildTime())
	}
	if err := (&Resolver{}).Close(); err != nil {
		t.Errorf("Close on empty resolver: %v", err)
	}
}

func TestNewResolver_Invalid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "junk.mmdb")
	os.WriteFile(p, []byte("not a database"), 0o644)
	if _, err := NewResolver(p); err == nil {
		t.Error("NewResolver(junk) = nil error, want error")
	}
}

func TestGetLatestRemoteVersion(t *testing.T) {
	now := time.Now()
	prev := now.AddDate(0, -1, 0)

	t.Run("current month available", func(t *testing.T) {
		u := newTestUpdater(filepath.Join(t.TempDir(), "db.mmdb"))
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodHead {
				t.Errorf("method = %s, want HEAD", r.Method)
			}
			if r.URL.Path != dbipPath(now) {
				w.WriteHeader(http.StatusNotFound)
			}
		})
		y, m, err := u.GetLatestRemoteVersion()
		if err != nil || y != now.Year() || m != int(now.Month()) {
			t.Errorf("got %d-%d, %v", y, m, err)
		}
	})

	t.Run("falls back to previous month", func(t *testing.T) {
		u := newTestUpdater(filepath.Join(t.TempDir(), "db.mmdb"))
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != dbipPath(prev) {
				w.WriteHeader(http.StatusNotFound)
			}
		})
		y, m, err := u.GetLatestRemoteVersion()
		if err != nil || y != prev.Year() || m != int(prev.Month()) {
			t.Errorf("got %d-%d, %v", y, m, err)
		}
	})

	t.Run("nothing available", func(t *testing.T) {
		u := newTestUpdater(filepath.Join(t.TempDir(), "db.mmdb"))
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		if _, _, err := u.GetLatestRemoteVersion(); err == nil {
			t.Error("want error when no month is published")
		}
	})
}

func TestNeedsUpdate_Remote(t *testing.T) {
	now := time.Now()
	db := writeTestMMDB(t, "X", "Y")

	t.Run("old local database", func(t *testing.T) {
		old := now.AddDate(0, -3, 0)
		os.Chtimes(db, old, old)
		u := newTestUpdater(db)
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {})
		if needs, err := u.NeedsUpdate(); err != nil || !needs {
			t.Errorf("NeedsUpdate = %v, %v; want true", needs, err)
		}
	})

	t.Run("old local database, last year", func(t *testing.T) {
		old := now.AddDate(-1, 0, 0)
		os.Chtimes(db, old, old)
		u := newTestUpdater(db)
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {})
		if needs, err := u.NeedsUpdate(); err != nil || !needs {
			t.Errorf("NeedsUpdate = %v, %v; want true", needs, err)
		}
	})

	t.Run("current local database", func(t *testing.T) {
		os.Chtimes(db, now, now)
		u := newTestUpdater(db)
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {})
		if needs, err := u.NeedsUpdate(); err != nil || needs {
			t.Errorf("NeedsUpdate = %v, %v; want false", needs, err)
		}
	})

	t.Run("remote unreachable keeps the local copy", func(t *testing.T) {
		u := newTestUpdater(db)
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		if needs, err := u.NeedsUpdate(); err == nil || needs {
			t.Errorf("NeedsUpdate = %v, %v; want false + error", needs, err)
		}
	})
}

func TestUpdate(t *testing.T) {
	now := time.Now()
	prev := now.AddDate(0, -1, 0)
	good := gzipBytes(t, buildTestMMDB("Poland", "Bialystok"))

	t.Run("installs a valid database", func(t *testing.T) {
		db := filepath.Join(t.TempDir(), "sub", "dbip.mmdb")
		u := newTestUpdater(db)
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != dbipPath(now) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write(good)
		})
		if err := u.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		r, err := NewResolver(db)
		if err != nil {
			t.Fatalf("installed db does not open: %v", err)
		}
		defer r.Close()
		if loc, _ := r.Lookup("1.1.1.1"); loc.City != "Bialystok" {
			t.Errorf("installed db lookup = %+v", loc)
		}
		left, _ := filepath.Glob(filepath.Join(filepath.Dir(db), "*.gz"))
		left2, _ := filepath.Glob(filepath.Join(filepath.Dir(db), "*.new"))
		if len(left)+len(left2) != 0 {
			t.Errorf("temp files left behind: %v %v", left, left2)
		}
	})

	t.Run("falls back to previous month on 404", func(t *testing.T) {
		db := filepath.Join(t.TempDir(), "dbip.mmdb")
		u := newTestUpdater(db)
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != dbipPath(prev) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write(good)
		})
		if err := u.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if !u.DatabaseExists() {
			t.Error("database not installed")
		}
	})

	failCases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{"server error", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}, "status"},
		{"both months missing", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}, "status"},
		{"not gzip", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("plain text, not gzip"))
		}, "install"},
		{"truncated gzip", func(w http.ResponseWriter, r *http.Request) {
			w.Write(good[:len(good)/2])
		}, "install"},
		{"gzip of a non-mmdb", func(w http.ResponseWriter, r *http.Request) {
			w.Write(gzipBytes(t, []byte("hello")))
		}, "invalid"},
	}
	for _, tc := range failCases {
		t.Run(tc.name+" keeps the old database", func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "dbip.mmdb")
			os.WriteFile(db, []byte("OLD"), 0o644)
			u := newTestUpdater(db)
			fakeDBIP(t, u, tc.handler)
			err := u.Update()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Update err = %v, want it to contain %q", err, tc.wantErr)
			}
			if got, _ := os.ReadFile(db); string(got) != "OLD" {
				t.Errorf("old database replaced: %q", got)
			}
		})
	}

	t.Run("network error", func(t *testing.T) {
		u := newTestUpdater(filepath.Join(t.TempDir(), "dbip.mmdb"))
		fakeDBIP(t, u, func(w http.ResponseWriter, r *http.Request) {})
		u.client.Transport = redirectTransport{target: mustURL("http://127.0.0.1:1")}
		if err := u.Update(); err == nil {
			t.Error("want error")
		}
		if _, _, err := u.GetLatestRemoteVersion(); err == nil {
			t.Error("want error from HEAD")
		}
	})
}
