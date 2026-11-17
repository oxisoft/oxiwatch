package geoip

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The helpers below write a minimal but valid MaxMind DB (IPv4, one tree node,
// every address maps to the same record), so the resolver and the updater's
// validate-then-install path are tested without shipping a 60 MB database.

func mmdbUint(typ byte, v uint64) []byte {
	var b []byte
	for v > 0 {
		b = append([]byte{byte(v)}, b...)
		v >>= 8
	}
	return append(mmdbCtrl(typ, len(b)), b...)
}

func mmdbCtrl(typ byte, size int) []byte {
	if size >= 29 {
		panic("mmdb test helper: size too large")
	}
	if typ <= 7 {
		return []byte{typ<<5 | byte(size)}
	}
	return []byte{byte(size), typ - 7}
}

func mmdbString(s string) []byte {
	return append(mmdbCtrl(2, len(s)), s...)
}

func mmdbMap(m map[string][]byte) []byte {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := mmdbCtrl(7, len(m))
	for _, k := range keys {
		out = append(out, mmdbString(k)...)
		out = append(out, m[k]...)
	}
	return out
}

func mmdbNames(en string) []byte {
	return mmdbMap(map[string][]byte{"names": mmdbMap(map[string][]byte{"en": mmdbString(en)})})
}

// buildTestMMDB returns a valid mmdb where every IPv4 address resolves to
// country/city given.
func buildTestMMDB(country, city string) []byte {
	const nodeCount = 1
	var buf bytes.Buffer
	// One node, 24-bit records, both pointing to data offset 0.
	rec := uint32(nodeCount + 16)
	for range 2 {
		buf.Write([]byte{byte(rec >> 16), byte(rec >> 8), byte(rec)})
	}
	buf.Write(make([]byte, 16)) // data section separator
	buf.Write(mmdbMap(map[string][]byte{
		"country": mmdbNames(country),
		"city":    mmdbNames(city),
	}))
	buf.WriteString("\xab\xcd\xefMaxMind.com")
	buf.Write(mmdbMap(map[string][]byte{
		"binary_format_major_version": mmdbUint(5, 2),
		"binary_format_minor_version": mmdbUint(5, 0),
		"build_epoch":                 mmdbUint(9, 1700000000),
		"database_type":               mmdbString("OxiWatch-Test"),
		"description":                 mmdbMap(map[string][]byte{"en": mmdbString("test")}),
		"ip_version":                  mmdbUint(5, 4),
		"languages":                   append(mmdbCtrl(11, 1), mmdbString("en")...),
		"node_count":                  mmdbUint(6, nodeCount),
		"record_size":                 mmdbUint(5, 24),
	}))
	return buf.Bytes()
}

func writeTestMMDB(t *testing.T, country, city string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.mmdb")
	if err := os.WriteFile(p, buildTestMMDB(country, city), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// redirectTransport sends every request to the test server, keeping the path,
// so the hard-coded download.db-ip.com URLs hit the fake.
type redirectTransport struct{ target *url.URL }

func (rt redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme = rt.target.Scheme
	r2.URL.Host = rt.target.Host
	r2.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(r2)
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func fakeDBIP(t *testing.T, u *Updater, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	u.client = &http.Client{Transport: redirectTransport{target}}
}
