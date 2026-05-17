package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
