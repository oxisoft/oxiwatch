package geoip

import (
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

const (
	dbipDownloadURL = "https://download.db-ip.com/free/dbip-city-lite-%d-%02d.mmdb.gz"
)

type Updater struct {
	dbPath string
	logger *slog.Logger
	client *http.Client
}

func NewUpdater(dbPath string, logger *slog.Logger) *Updater {
	return &Updater{
		dbPath: dbPath,
		logger: logger,
		// The GeoIP database is ~60 MB, so we must NOT cap the whole transfer
		// (a slow link would abort mid-download and leave no database). Instead
		// bound connection setup and response latency, so a dead/hung server
		// can't block daemon startup while a slow-but-progressing download is
		// allowed to finish. A generous overall ceiling is the final backstop.
		client: &http.Client{
			Timeout: 30 * time.Minute,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       60 * time.Second,
			},
		},
	}
}

func (u *Updater) DatabaseExists() bool {
	_, err := os.Stat(u.dbPath)
	return err == nil
}

func (u *Updater) GetDatabaseInfo() (modTime time.Time, size int64, err error) {
	info, err := os.Stat(u.dbPath)
	if err != nil {
		return time.Time{}, 0, err
	}
	return info.ModTime(), info.Size(), nil
}

func (u *Updater) GetLocalVersion() (year int, month int, err error) {
	info, err := os.Stat(u.dbPath)
	if err != nil {
		return 0, 0, err
	}
	modTime := info.ModTime()
	return modTime.Year(), int(modTime.Month()), nil
}

func (u *Updater) GetLatestRemoteVersion() (year int, month int, err error) {
	now := time.Now()

	url := fmt.Sprintf(dbipDownloadURL, now.Year(), int(now.Month()))
	resp, err := u.client.Head(url)
	if err != nil {
		return 0, 0, err
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return now.Year(), int(now.Month()), nil
	}

	prev := now.AddDate(0, -1, 0)
	url = fmt.Sprintf(dbipDownloadURL, prev.Year(), int(prev.Month()))
	resp, err = u.client.Head(url)
	if err != nil {
		return 0, 0, err
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return prev.Year(), int(prev.Month()), nil
	}

	return 0, 0, fmt.Errorf("no remote database found")
}

func (u *Updater) NeedsUpdate() (bool, error) {
	if !u.DatabaseExists() {
		return true, nil
	}

	localYear, localMonth, err := u.GetLocalVersion()
	if err != nil {
		return true, nil
	}

	remoteYear, remoteMonth, err := u.GetLatestRemoteVersion()
	if err != nil {
		return false, err
	}

	if remoteYear > localYear {
		return true, nil
	}
	if remoteYear == localYear && remoteMonth > localMonth {
		return true, nil
	}

	return false, nil
}

func (u *Updater) Update() error {
	u.logger.Info("downloading GeoIP database from DB-IP")

	now := time.Now()
	url := fmt.Sprintf(dbipDownloadURL, now.Year(), int(now.Month()))

	resp, err := u.client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		prev := now.AddDate(0, -1, 0)
		url = fmt.Sprintf(dbipDownloadURL, prev.Year(), int(prev.Month()))
		resp, err = u.client.Get(url)
		if err != nil {
			return fmt.Errorf("failed to download: %w", err)
		}
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	dir := filepath.Dir(u.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "geoip-*.mmdb.gz")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	written, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to save download: %w", err)
	}
	tmpFile.Close()

	// Guard against truncated downloads (a short read that ends in EOF would
	// otherwise be silently accepted and corrupt the database).
	if resp.ContentLength > 0 && written != resp.ContentLength {
		return fmt.Errorf("incomplete download: got %d of %d bytes", written, resp.ContentLength)
	}

	if err := u.extractAndInstall(tmpPath); err != nil {
		return fmt.Errorf("failed to install database: %w", err)
	}

	u.logger.Info("GeoIP database updated successfully", "path", u.dbPath)
	return nil
}

// extractAndInstall decompresses the downloaded gzip into a temporary file in
// the destination directory, verifies it is a readable mmdb, and only then
// atomically renames it into place. This guarantees a partial or corrupt
// download can never replace (or destroy) the working database.
func (u *Updater) extractAndInstall(gzPath string) error {
	f, err := os.Open(gzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	dir := filepath.Dir(u.dbPath)
	out, err := os.CreateTemp(dir, "dbip-*.mmdb.new")
	if err != nil {
		return err
	}
	tmpPath := out.Name()
	defer os.Remove(tmpPath) // no-op once renamed

	if _, err := io.Copy(out, gzr); err != nil {
		out.Close()
		return fmt.Errorf("decompress failed (truncated download?): %w", err)
	}
	if err := out.Close(); err != nil {
		return err
	}

	// Validate the extracted file actually opens as a MaxMind DB before we let
	// it replace the live database.
	db, err := maxminddb.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("extracted database is invalid: %w", err)
	}
	db.Close()

	if err := os.Rename(tmpPath, u.dbPath); err != nil {
		return fmt.Errorf("failed to install database: %w", err)
	}
	return nil
}
