package geoip

import (
	"fmt"
	"net"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

type Location struct {
	Country string
	City    string
}

type Resolver struct {
	db *maxminddb.Reader
}

type geoRecord struct {
	Country struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
}

func NewResolver(dbPath string) (*Resolver, error) {
	db, err := maxminddb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &Resolver{db: db}, nil
}

func (r *Resolver) Lookup(ipStr string) (*Location, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return &Location{}, nil
	}

	var record geoRecord
	if err := r.db.Lookup(ip, &record); err != nil {
		return nil, err
	}

	return &Location{
		Country: record.Country.Names["en"],
		City:    record.City.Names["en"],
	}, nil
}

// LookupRaw returns the full decoded record for an IP, for troubleshooting.
func (r *Resolver) LookupRaw(ipStr string) (map[string]any, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %q", ipStr)
	}
	var raw map[string]any
	if err := r.db.Lookup(ip, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// DatabaseType is the mmdb database type (e.g. "DBIP-City-Lite").
func (r *Resolver) DatabaseType() string {
	return r.db.Metadata.DatabaseType
}

// BuildTime is when the database was built.
func (r *Resolver) BuildTime() time.Time {
	return time.Unix(int64(r.db.Metadata.BuildEpoch), 0)
}

func (r *Resolver) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}
