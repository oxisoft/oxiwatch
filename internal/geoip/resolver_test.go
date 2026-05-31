package geoip

import (
	"os"
	"testing"
)

// TestResolver_KnownIPs is an integration check against a real DB-IP city-lite
// database. It is skipped unless a database is available, so it never blocks CI
// — but it documents expected lookups and guards against regressions where a
// well-known IP stops resolving (e.g. 83.6.42.41 -> Poland, Bialystok).
//
// Run it against an installed database with:
//
//	GEOIP_TEST_DB=/var/lib/oxiwatch/dbip-city-lite.mmdb go test ./internal/geoip/ -run KnownIPs -v
func TestResolver_KnownIPs(t *testing.T) {
	path := os.Getenv("GEOIP_TEST_DB")
	if path == "" {
		path = "/var/lib/oxiwatch/dbip-city-lite.mmdb"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no GeoIP database available (set GEOIP_TEST_DB): %v", err)
	}

	r, err := NewResolver(path)
	if err != nil {
		t.Fatalf("NewResolver(%s): %v", path, err)
	}
	defer r.Close()

	cases := []struct {
		ip, country, city string // city "" means don't assert city (varies by DB month)
	}{
		{"83.6.42.41", "Poland", "Bialystok"},
		{"8.8.8.8", "United States", ""},
		{"1.1.1.1", "Australia", ""},
	}

	for _, c := range cases {
		loc, err := r.Lookup(c.ip)
		if err != nil {
			t.Errorf("Lookup(%s): %v", c.ip, err)
			continue
		}
		if loc.Country != c.country {
			t.Errorf("Lookup(%s) country = %q, want %q", c.ip, loc.Country, c.country)
		}
		if c.city != "" && loc.City != c.city {
			t.Errorf("Lookup(%s) city = %q, want %q", c.ip, loc.City, c.city)
		}
	}
}

// TestResolver_Introspection exercises the troubleshooting helpers used by the
// `oxiwatch geoip lookup` command (skipped without a database).
func TestResolver_Introspection(t *testing.T) {
	path := os.Getenv("GEOIP_TEST_DB")
	if path == "" {
		path = "/var/lib/oxiwatch/dbip-city-lite.mmdb"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no GeoIP database available (set GEOIP_TEST_DB): %v", err)
	}
	r, err := NewResolver(path)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	defer r.Close()

	if r.DatabaseType() == "" {
		t.Error("DatabaseType() is empty")
	}
	if r.BuildTime().IsZero() {
		t.Error("BuildTime() is zero")
	}
	raw, err := r.LookupRaw("83.6.42.41")
	if err != nil {
		t.Fatalf("LookupRaw: %v", err)
	}
	if len(raw) == 0 {
		t.Error("LookupRaw returned an empty record for a known IP")
	}
	t.Logf("type=%s built=%s raw keys=%d", r.DatabaseType(), r.BuildTime().Format("2006-01-02"), len(raw))
}

// TestResolver_InvalidIP verifies the resolver returns an empty location (no
// error) for unparseable input, matching how the daemon treats it.
func TestResolver_InvalidIP(t *testing.T) {
	path := os.Getenv("GEOIP_TEST_DB")
	if path == "" {
		path = "/var/lib/oxiwatch/dbip-city-lite.mmdb"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no GeoIP database available (set GEOIP_TEST_DB): %v", err)
	}
	r, err := NewResolver(path)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	defer r.Close()

	loc, err := r.Lookup("not-an-ip")
	if err != nil {
		t.Fatalf("Lookup(invalid) returned error: %v", err)
	}
	if loc == nil || loc.Country != "" || loc.City != "" {
		t.Errorf("Lookup(invalid) = %+v, want empty location", loc)
	}
}
