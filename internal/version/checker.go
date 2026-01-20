package version

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubAPIURL = "https://api.github.com/repos/oxisoft/oxiwatch/releases/latest"
)

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type Checker struct {
	currentVersion string
	httpClient     *http.Client
}

func NewChecker(currentVersion string) *Checker {
	return &Checker{
		currentVersion: currentVersion,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Checker) GetLatestRelease() (*Release, error) {
	req, err := http.NewRequest("GET", githubAPIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "oxiwatch/"+c.currentVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func (c *Checker) IsUpdateAvailable() (bool, string, error) {
	release, err := c.GetLatestRelease()
	if err != nil {
		return false, "", err
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	if c.currentVersion == "dev" {
		return true, latestVersion, nil
	}

	currentClean := strings.TrimPrefix(c.currentVersion, "v")
	if compareVersions(latestVersion, currentClean) > 0 {
		return true, latestVersion, nil
	}

	return false, latestVersion, nil
}

func (c *Checker) GetAssetURL(release *Release) (string, error) {
	expectedName := fmt.Sprintf("oxiwatch-%s-%s", runtime.GOOS, runtime.GOARCH)

	for _, asset := range release.Assets {
		if asset.Name == expectedName {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("no binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
}

func (c *Checker) GetChecksumURL(release *Release) (string, error) {
	for _, asset := range release.Assets {
		if asset.Name == "checksums.txt" {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("checksums.txt not found in release")
}

func (c *Checker) fetchChecksums(url string) (map[string]string, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch checksums: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	checksums := make(map[string]string)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			checksums[parts[1]] = parts[0]
		}
	}
	return checksums, nil
}

func (c *Checker) Upgrade(verbose bool) error {
	log := func(format string, args ...any) {
		if verbose {
			fmt.Printf(format+"\n", args...)
		}
	}

	log("Fetching release information...")
	release, err := c.GetLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	log("Latest version: %s", latestVersion)

	if c.currentVersion != "dev" {
		currentClean := strings.TrimPrefix(c.currentVersion, "v")
		if compareVersions(latestVersion, currentClean) <= 0 {
			return fmt.Errorf("already at latest version (%s)", c.currentVersion)
		}
	}

	log("Fetching checksums...")
	checksumURL, err := c.GetChecksumURL(release)
	if err != nil {
		return fmt.Errorf("failed to get checksum URL: %w", err)
	}

	checksums, err := c.fetchChecksums(checksumURL)
	if err != nil {
		return fmt.Errorf("failed to fetch checksums: %w", err)
	}

	assetName := fmt.Sprintf("oxiwatch-%s-%s", runtime.GOOS, runtime.GOARCH)
	expectedChecksum, ok := checksums[assetName]
	if !ok {
		return fmt.Errorf("no checksum found for %s", assetName)
	}
	log("Expected checksum: %s", expectedChecksum)

	assetURL, err := c.GetAssetURL(release)
	if err != nil {
		return err
	}

	log("Downloading binary...")
	resp, err := c.httpClient.Get(assetURL)
	if err != nil {
		return fmt.Errorf("failed to download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	execDir := filepath.Dir(execPath)
	tempPath := filepath.Join(execDir, ".oxiwatch.new")

	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(tempFile, hasher)

	_, err = io.Copy(writer, resp.Body)
	tempFile.Close()
	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to write binary: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	log("Downloaded checksum: %s", actualChecksum)

	log("Verifying checksum...")
	if actualChecksum != expectedChecksum {
		os.Remove(tempPath)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}
	log("Checksum verified successfully")

	log("Replacing binary...")
	if err := os.Rename(tempPath, execPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	log("Upgrade complete")
	return nil
}

func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := max(len(parts1), len(parts2))

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &n1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &n2)
		}

		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}

	return 0
}
