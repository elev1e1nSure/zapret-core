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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
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

// UpdateProgressCallback is called during update to report progress
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

// downloadFile downloads from url to destPath with timeout
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
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(file)
	return filepath.Join(root, "..", "lists")
}

// GitHubRelease represents a GitHub release API response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// getExeDir returns the directory of the current executable
func getExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// getExePath returns the full path of the current executable
func getExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "zapret-core.exe"
	}
	return exe
}

// cleanupOldExe silently deletes zapret-core.exe.old if it exists
func cleanupOldExe() {
	oldExe := filepath.Join(getExeDir(), "zapret-core.exe.old")
	_ = os.Remove(oldExe)
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

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
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

	return release.TagName, nil
}

// downloadRelease downloads the zip and checksums.txt files
func downloadRelease(zipURL, checksumURL string) (string, string, error) {
	exeDir := getExeDir()
	zipPath := filepath.Join(exeDir, "zapret-core-update.zip")
	checksumPath := filepath.Join(exeDir, "checksums.txt")

	// Download checksums first
	if err := downloadFile(checksumURL, checksumPath); err != nil {
		return "", "", fmt.Errorf("download checksums: %w", err)
	}

	// Download zip
	if err := downloadFile(zipURL, zipPath); err != nil {
		os.Remove(checksumPath)
		return "", "", fmt.Errorf("download zip: %w", err)
	}

	return zipPath, checksumPath, nil
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
		if f.Name == "zapret-core.exe" {
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

// applyUpdate performs atomic file swap and restarts the process
func applyUpdate(newExePath, remoteVersion string) error {
	exePath := getExePath()
	oldExePath := exePath + ".old"

	// Rename current exe to .old
	if err := os.Rename(exePath, oldExePath); err != nil {
		return fmt.Errorf("rename current exe: %w", err)
	}

	// Rename new exe to current exe
	if err := os.Rename(newExePath, exePath); err != nil {
		// Try to restore old exe on failure
		_ = os.Rename(oldExePath, exePath)
		return fmt.Errorf("rename new exe: %w", err)
	}

	// Restart process
	logSuccess("Updated %s → %s. Restarting...", Version, remoteVersion)

	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new process: %w", err)
	}

	os.Exit(0)
	return nil
}
