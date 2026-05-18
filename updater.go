package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	baseURL = "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/"
)

var listFiles = []string{
	"ipset-all.txt",
	"ipset-exclude.txt",
	"list-exclude.txt",
	"list-general.txt",
	"list-google.txt",
}

// UpdateProgressCallback is called during list update to report progress
type UpdateProgressCallback func(current, total int, filename string)

// UpdateLists downloads and updates all list files from GitHub
// Uses atomic updates: download to temp file, then rename
// On error, leaves lists directory unchanged
func UpdateLists(progressCb UpdateProgressCallback) error {
	listsDir := listsDirPath()

	total := len(listFiles)
	for i, filename := range listFiles {
		if progressCb != nil {
			progressCb(i+1, total, filename)
		}

		url := baseURL + filename
		destPath := filepath.Join(listsDir, filename)
		tmpPath := destPath + ".tmp"

		if err := downloadFile(url, tmpPath); err != nil {
			logError("Failed to download %s: %v", filename, err)
			return fmt.Errorf("download %s: %w", filename, err)
		}

		if err := os.Rename(tmpPath, destPath); err != nil {
			logError("Failed to rename %s: %v", filename, err)
			os.Remove(tmpPath)
			return fmt.Errorf("rename %s: %w", filename, err)
		}

		logInfo("Updated %s", filename)
	}

	return nil
}

// downloadFile downloads from url to destPath with 30-second timeout
func downloadFile(url, destPath string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}

	return nil
}

// listsDirPath returns absolute path to lists/ directory
func listsDirPath() string {
	return filepath.Join(exeDir(), "lists")
}

// GitHubRelease represents a GitHub release API response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// cleanupUpdateArtifacts silently deletes leftover .old and .new files from previous updates
func cleanupUpdateArtifacts() {
	_ = os.Remove(exePath() + ".old")
	_ = os.Remove(exePath() + ".new")
}

// checkForUpdate checks GitHub API for latest release and returns the tag name if newer
func checkForUpdate() (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	url := "https://api.github.com/repos/elev1e1nSure/zapret-core/releases/latest"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "zapret-core-updater")

	// Add GitHub token if available
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	localVersion := strings.TrimPrefix(Version, "v")
	remoteVersion := strings.TrimPrefix(release.TagName, "v")

	if localVersion == remoteVersion {
		return "", nil // up to date
	}

	// Compare versions numerically (major.minor.patch)
	localParts := strings.Split(localVersion, ".")
	remoteParts := strings.Split(remoteVersion, ".")

	for i := 0; i < 3; i++ {
		if i >= len(localParts) || i >= len(remoteParts) {
			break
		}
		localNum, _ := strconv.Atoi(localParts[i])
		remoteNum, _ := strconv.Atoi(remoteParts[i])
		if remoteNum > localNum {
			return release.TagName, nil // update available
		} else if remoteNum < localNum {
			return "", nil // local is newer
		}
	}

	return "", nil // versions are equal or local is newer
}

// downloadRelease downloads zip and checksums.txt to exeDir
func downloadRelease(zipURL, checksumURL string) (string, string, string, error) {
	exeDir := exeDir()
	zipFilename := filepath.Base(zipURL)
	zipPath := filepath.Join(exeDir, zipFilename)
	checksumPath := filepath.Join(exeDir, "checksums.txt")

	// Download checksums first
	if err := downloadFile(checksumURL, checksumPath); err != nil {
		return "", "", "", fmt.Errorf("download checksums: %w", err)
	}

	// Download zip
	if err := downloadFile(zipURL, zipPath); err != nil {
		os.Remove(checksumPath)
		return "", "", "", fmt.Errorf("download zip: %w", err)
	}

	return zipPath, checksumPath, zipFilename, nil
}

// verifySHA256 computes SHA256 of file and compares with expected hash
func verifySHA256(filepath, expectedHash string) error {
	f, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return err
	}

	computedHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(computedHash, expectedHash) {
		return fmt.Errorf("SHA256 mismatch: expected %s, got %s", expectedHash, computedHash)
	}

	return nil
}

// parseChecksum extracts SHA256 hash for a given filename from checksums.txt
func parseChecksum(checksumPath, zipFilename string) (string, error) {
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, zipFilename) {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				return parts[0], nil
			}
		}
	}

	return "", fmt.Errorf("checksum not found for %s", zipFilename)
}

// extractExe extracts zapret-core.exe from zip to destination path
func extractExe(zipPath, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Look for zapret-core.exe at root or in any subfolder
		if filepath.Base(f.Name) == "zapret-core.exe" && !f.FileInfo().IsDir() {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			destFile, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer destFile.Close()

			if _, err := io.Copy(destFile, rc); err != nil {
				return err
			}

			return nil
		}
	}

	return fmt.Errorf("zapret-core.exe not found in zip")
}

// SelfUpdateProgressCallback receives stage and human-readable message during performSelfUpdate
type SelfUpdateProgressCallback func(stage, message string)

// performSelfUpdate runs the full self-update pipeline:
// check → fetch release info → find assets → download → verify SHA256 → extract → atomic swap.
// progressCb is called at each stage; pass nil to suppress progress reporting.
// Returns ("", nil) when already up to date.
// Returns (newVersion, nil) on successful swap; caller must exit/restart.
func performSelfUpdate(progressCb SelfUpdateProgressCallback) (string, error) {
	progress := func(stage, msg string) {
		if progressCb != nil {
			progressCb(stage, msg)
		}
	}

	progress("checking", "Checking for updates...")
	remoteVersion, err := checkForUpdate()
	if err != nil {
		return "", fmt.Errorf("check for update: %w", err)
	}
	if remoteVersion == "" {
		return "", nil
	}

	progress("found", fmt.Sprintf("New version available: %s → %s", Version, remoteVersion))

	client := &http.Client{Timeout: 30 * time.Second}
	url := "https://api.github.com/repos/elev1e1nSure/zapret-core/releases/latest"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "zapret-core-updater")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetch release info: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parse release info: %w", err)
	}

	var zipURL, checksumURL string
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, "windows-amd64.zip") {
			zipURL = asset.BrowserDownloadURL
		} else if asset.Name == "checksums.txt" {
			checksumURL = asset.BrowserDownloadURL
		}
	}

	if zipURL == "" {
		return "", fmt.Errorf("windows zip not found in release assets")
	}
	if checksumURL == "" {
		return "", fmt.Errorf("checksums.txt not found in release assets")
	}

	progress("downloading", fmt.Sprintf("Downloading %s...", filepath.Base(zipURL)))

	zipPath, checksumPath, zipFilename, err := downloadRelease(zipURL, checksumURL)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer os.Remove(zipPath)
	defer os.Remove(checksumPath)

	expectedHash, err := parseChecksum(checksumPath, zipFilename)
	if err != nil {
		return "", fmt.Errorf("parse checksum: %w", err)
	}

	progress("verifying", "Verifying SHA256...")
	if err := verifySHA256(zipPath, expectedHash); err != nil {
		return "", fmt.Errorf("verification failed: %w", err)
	}

	progress("applying", "Applying update...")
	newExePath := filepath.Join(exeDir(), "zapret-core.exe.new")
	if err := extractExe(zipPath, newExePath); err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}

	if err := applyUpdate(newExePath, remoteVersion); err != nil {
		_ = os.Remove(newExePath)
		return "", fmt.Errorf("apply: %w", err)
	}

	return remoteVersion, nil
}

// applyUpdate performs atomic file swap
func applyUpdate(newExePath, remoteVersion string) error {
	current := exePath()
	oldExe := current + ".old"

	// Rename current exe to .old
	if err := os.Rename(current, oldExe); err != nil {
		return fmt.Errorf("rename current exe: %w", err)
	}

	// Rename new exe to current exe
	if err := os.Rename(newExePath, current); err != nil {
		// Try to restore old exe on failure
		_ = os.Rename(oldExe, current)
		return fmt.Errorf("rename new exe: %w", err)
	}

	return nil
}
