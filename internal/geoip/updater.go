package geoip

import (
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	dbipDownloadURL = "https://download.db-ip.com/free/dbip-city-lite-%d-%02d.mmdb.gz"
)

type Updater struct {
	dbPath string
	logger *slog.Logger
}

func NewUpdater(dbPath string, logger *slog.Logger) *Updater {
	return &Updater{
		dbPath: dbPath,
		logger: logger,
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
	resp, err := http.Head(url)
	if err != nil {
		return 0, 0, err
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return now.Year(), int(now.Month()), nil
	}

	prev := now.AddDate(0, -1, 0)
	url = fmt.Sprintf(dbipDownloadURL, prev.Year(), int(prev.Month()))
	resp, err = http.Head(url)
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

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		prev := now.AddDate(0, -1, 0)
		url = fmt.Sprintf(dbipDownloadURL, prev.Year(), int(prev.Month()))
		resp, err = http.Get(url)
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

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to save download: %w", err)
	}
	tmpFile.Close()

	if err := u.extractGzip(tmpPath); err != nil {
		return fmt.Errorf("failed to extract database: %w", err)
	}

	u.logger.Info("GeoIP database updated successfully", "path", u.dbPath)
	return nil
}

func (u *Updater) extractGzip(gzPath string) error {
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

	out, err := os.Create(u.dbPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, gzr)
	return err
}
