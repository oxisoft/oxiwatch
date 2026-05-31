package geoip

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestUpdater(dbPath string) *Updater {
	return NewUpdater(dbPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestDatabaseExists(t *testing.T) {
	t.Run("missing path returns false", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist.mmdb")
		u := newTestUpdater(missing)
		if u.DatabaseExists() {
			t.Errorf("DatabaseExists() = true, want false for missing path %q", missing)
		}
	})

	t.Run("existing file returns true", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "city.mmdb")
		if err := os.WriteFile(path, []byte("fake mmdb contents"), 0o644); err != nil {
			t.Fatalf("failed to create temp db file: %v", err)
		}
		u := newTestUpdater(path)
		if !u.DatabaseExists() {
			t.Errorf("DatabaseExists() = false, want true for existing file %q", path)
		}
	})
}

func TestGetDatabaseInfo(t *testing.T) {
	t.Run("returns modtime and size for existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "city.mmdb")
		contents := []byte("0123456789") // 10 bytes
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			t.Fatalf("failed to create temp db file: %v", err)
		}

		want := time.Date(2023, time.March, 15, 12, 0, 0, 0, time.UTC)
		if err := os.Chtimes(path, want, want); err != nil {
			t.Fatalf("failed to set mtime: %v", err)
		}

		u := newTestUpdater(path)
		modTime, size, err := u.GetDatabaseInfo()
		if err != nil {
			t.Fatalf("GetDatabaseInfo() error = %v, want nil", err)
		}
		if !modTime.Equal(want) {
			t.Errorf("modTime = %v, want %v", modTime, want)
		}
		if size != int64(len(contents)) {
			t.Errorf("size = %d, want %d", size, len(contents))
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing.mmdb")
		u := newTestUpdater(missing)
		modTime, size, err := u.GetDatabaseInfo()
		if err == nil {
			t.Fatalf("GetDatabaseInfo() error = nil, want error for missing file")
		}
		if !modTime.IsZero() {
			t.Errorf("modTime = %v, want zero value on error", modTime)
		}
		if size != 0 {
			t.Errorf("size = %d, want 0 on error", size)
		}
	})
}

func TestGetLocalVersion(t *testing.T) {
	tests := []struct {
		name      string
		modTime   time.Time
		wantYear  int
		wantMonth int
	}{
		{
			name:      "march 2023",
			modTime:   time.Date(2023, time.March, 15, 0, 0, 0, 0, time.UTC),
			wantYear:  2023,
			wantMonth: 3,
		},
		{
			name:      "december 2024",
			modTime:   time.Date(2024, time.December, 1, 23, 59, 59, 0, time.UTC),
			wantYear:  2024,
			wantMonth: 12,
		},
		{
			name:      "january 2020",
			modTime:   time.Date(2020, time.January, 31, 6, 30, 0, 0, time.UTC),
			wantYear:  2020,
			wantMonth: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "city.mmdb")
			if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
				t.Fatalf("failed to create temp db file: %v", err)
			}
			if err := os.Chtimes(path, tt.modTime, tt.modTime); err != nil {
				t.Fatalf("failed to set mtime: %v", err)
			}

			u := newTestUpdater(path)
			year, month, err := u.GetLocalVersion()
			if err != nil {
				t.Fatalf("GetLocalVersion() error = %v, want nil", err)
			}
			if year != tt.wantYear {
				t.Errorf("year = %d, want %d", year, tt.wantYear)
			}
			if month != tt.wantMonth {
				t.Errorf("month = %d, want %d", month, tt.wantMonth)
			}
		})
	}

	t.Run("returns error for missing file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing.mmdb")
		u := newTestUpdater(missing)
		year, month, err := u.GetLocalVersion()
		if err == nil {
			t.Fatalf("GetLocalVersion() error = nil, want error for missing file")
		}
		if year != 0 || month != 0 {
			t.Errorf("year, month = %d, %d, want 0, 0 on error", year, month)
		}
	})
}

func TestNeedsUpdate_NoDatabase(t *testing.T) {
	// When no database is present, NeedsUpdate must return (true, nil)
	// without consulting the network (the missing-db branch short-circuits).
	missing := filepath.Join(t.TempDir(), "missing.mmdb")
	u := newTestUpdater(missing)

	needs, err := u.NeedsUpdate()
	if err != nil {
		t.Fatalf("NeedsUpdate() error = %v, want nil", err)
	}
	if !needs {
		t.Errorf("NeedsUpdate() = false, want true when no database present")
	}
}
