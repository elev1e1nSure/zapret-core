package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	cleanupOldExe()

	if err := initLogger(); err != nil {
		fmt.Printf("\033[31m[!] Logger initialization error: %v\033[0m\n", err)
		os.Exit(1)
	}
	defer closeLogger()

	if err := LoadConfig(); err != nil {
		logError("Config loading error: %v", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		printBanner()
		runBest()
		return
	}

	switch os.Args[1] {
	case "--find":
		printBanner()
		runFind()
	case "--status":
		printBanner()
		runStatus()
	case "--stop":
		printBanner()
		runStop()
	case "--watch":
		printBanner()
		runWatch()
	case "--server":
		printBanner()
		runServer()
	case "--updatelists":
		printBanner()
		runUpdate()
	case "--update":
		printBanner()
		runSelfUpdate()
	case "--reset":
		printBanner()
		runReset()
	case "--export":
		printBanner()
		exportPath := ""
		if len(os.Args) > 2 {
			exportPath = os.Args[2]
		}
		runExport(exportPath)
	case "--import":
		printBanner()
		importPath := ""
		if len(os.Args) > 2 {
			importPath = os.Args[2]
		}
		runImport(importPath)
	default:
		printBanner()
		logInfo("Usage:")
		logInfo("  zapret-core           — run best known strategy")
		logInfo("  zapret-core --find    — find working strategy")
		logInfo("  zapret-core --status  — show status")
		logInfo("  zapret-core --stop    — stop")
		logInfo("  zapret-core --watch   — monitoring + auto-recovery on failure")
		logInfo("  zapret-core --server  — start HTTP API server on :7432")
		logInfo("  zapret-core --updatelists  — update lists from GitHub")
		logInfo("  zapret-core --update  — self-update to latest release")
		logInfo("  zapret-core --reset   — clear strategies for current ASN")
		logInfo("  zapret-core --export  — export strategies to file")
		logInfo("  zapret-core --import  — import strategies from file")
	}
}

// runBest loads and runs the best known strategy for the current provider
func runBest() {
	logInfo("Detecting provider...")
	provider := GetProvider()
	logInfo("Provider: %s (%s)", provider.ASN, provider.Org)

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Knowledge loading error: %v", err)
		os.Exit(1)
	}

	vectors := kb.BestForASN(provider.ASN, 1)
	if len(vectors) == 0 {
		logWarn("No known strategies for this provider. Run --find")
		os.Exit(1)
	}

	strategy := VectorToStrategy(vectors[0], 0)

	logInfo("Starting strategy: %s", strategy.Name)
	err = StartWinws(strategy)
	if err != nil {
		logError("Startup error: %v", err)
		os.Exit(1)
	}
	logSuccess("Running. Press Ctrl+C to stop.")

	// Handle Ctrl+C to stop winws before exiting
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh

	logInfo("Stopping...")
	StopWinws()
	logInfo("Stopped.")
}

// runFind iterates through strategies to find a working one
func runFind() {
	logInfo("Detecting provider...")
	provider := GetProvider()
	logInfo("Provider: %s (%s)", provider.ASN, provider.Org)

	logInfo("Checking conflicts...")
	conflicts := CheckConflicts()
	if len(conflicts) > 0 {
		logWarn("Conflicts detected:")
		for _, c := range conflicts {
			logWarn("    - %s", c)
		}
		logError("Resolve conflicts and run again.")
		os.Exit(1)
	}
	logSuccess("No conflicts.")

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Knowledge loading error: %v", err)
		os.Exit(1)
	}

	opt := NewOptimizer(provider.ASN, kb)

	logInfo("Starting strategy search...")
	result, vector := opt.Run()

	if result == nil {
		logError("Working strategy not found.")
		os.Exit(1)
	}

	logSuccess("Working strategy found: %s", result.Name)
	kb.Record(provider.ASN, vector, 1.0)
	logSuccess("Strategy saved to knowledge.")
}

// runStatus shows the current state
func runStatus() {
	running := IsWinwsRunning()
	if running {
		logInfo("winws running")
	} else {
		logWarn("winws not running")
	}

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Knowledge loading error: %v", err)
		return
	}

	provider := GetProvider()
	vectors := kb.BestForASN(provider.ASN, 1)
	if len(vectors) > 0 {
		strategy := VectorToStrategy(vectors[0], 0)
		logInfo("Best known strategy for %s: %s", provider.ASN, strategy.Name)
	}
}

// runStop stops winws
func runStop() {
	logInfo("Stopping winws...")
	err := StopWinws()
	if err != nil {
		logError("Error: %v", err)
		os.Exit(1)
	}
	logSuccess("Stopped.")
}

// runWatch starts watchdog with auto-recovery on failure
func runWatch() {
	logInfo("Starting watchdog...")
	provider := GetProvider()
	logInfo("Provider: %s (%s)", provider.ASN, provider.Org)

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Knowledge loading error: %v", err)
		os.Exit(1)
	}

	StartWatchdog(provider.ASN, kb)
}

// runServer starts the HTTP API server
func runServer() {
	provider := GetProvider()
	logInfo("Provider: %s (%s)", provider.ASN, provider.Org)

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Knowledge loading error: %v", err)
		os.Exit(1)
	}

	srv := NewAPIServer(kb, provider)
	logInfo("Starting API server on 127.0.0.1:7432")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		logInfo("Shutting down server...")
		srv.Stop()
	}()

	if err := srv.Start("127.0.0.1:7432"); err != nil {
		logError("Server error: %v", err)
		os.Exit(1)
	}
}

// runUpdate updates list files from GitHub
func runUpdate() {
	logInfo("Updating lists from GitHub...")

	err := UpdateLists(func(current, total int, filename string) {
		logInfo("[%d/%d] Updating %s...", current, total, filename)
	})

	if err != nil {
		logError("Update error: %v", err)
		os.Exit(1)
	}

	logSuccess("Lists updated successfully.")
}

// runReset removes all knowledge entries for the current provider's ASN
func runReset() {
	logInfo("Detecting provider...")
	provider := GetProvider()
	logInfo("Provider: %s (%s)", provider.ASN, provider.Org)

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Knowledge loading error: %v", err)
		os.Exit(1)
	}

	removed := kb.RemoveForASN(provider.ASN)
	if removed == 0 {
		logInfo("No entries found for ASN %s", provider.ASN)
		return
	}

	if err := kb.Save(); err != nil {
		logError("Error saving knowledge: %v", err)
		os.Exit(1)
	}

	logSuccess("Removed %d strategies for ASN %s", removed, provider.ASN)
}

// runExport exports all knowledge entries to a JSON file
func runExport(filepath string) {
	if filepath == "" {
		logError("Usage: zapret-core --export <filepath>")
		os.Exit(1)
	}

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Knowledge loading error: %v", err)
		os.Exit(1)
	}

	// Check if file exists
	if _, err := os.Stat(filepath); err == nil {
		fmt.Print("File already exists. Overwrite? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			logError("Error reading input: %v", err)
			os.Exit(1)
		}
		response = strings.TrimSpace(response)
		if response != "y" && response != "Y" {
			logInfo("Export aborted.")
			return
		}
	}

	export := kb.Export()
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		logError("Error marshaling export: %v", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		logError("Error writing file: %v", err)
		os.Exit(1)
	}

	logSuccess("Exported %d entries to %s", export.EntryCount, filepath)
}

// runImport merges knowledge entries from a JSON file
func runImport(filepath string) {
	if filepath == "" {
		logError("Usage: zapret-core --import <filepath>")
		os.Exit(1)
	}

	data, err := os.ReadFile(filepath)
	if err != nil {
		logError("Error reading file: %v", err)
		os.Exit(1)
	}

	var export ExportFormat
	if err := json.Unmarshal(data, &export); err != nil {
		logError("Error parsing export file: %v", err)
		os.Exit(1)
	}

	if export.Entries == nil {
		logError("Invalid export format: missing entries")
		os.Exit(1)
	}

	if len(export.Entries) == 0 {
		logInfo("No entries to import.")
		return
	}

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Knowledge loading error: %v", err)
		os.Exit(1)
	}

	added, skipped, updated := kb.Import(export)

	if err := kb.Save(); err != nil {
		logError("Error saving knowledge: %v", err)
		os.Exit(1)
	}

	logInfo("Import results:")
	logInfo("  Added: %d", added)
	logInfo("  Skipped: %d", skipped)
	logInfo("  Updated: %d", updated)
	logSuccess("Import completed.")
}

// runSelfUpdate checks for updates and applies them if available
func runSelfUpdate() {
	logInfo("Current version: %s", Version)

	remoteVersion, err := checkForUpdate()
	if err != nil {
		logError("Failed to check for updates: %v", err)
		os.Exit(1)
	}

	if remoteVersion == "" {
		logInfo("Already up to date.")
		return
	}

	logInfo("New version available: %s", remoteVersion)

	// Get release info to find asset URLs
	client := &http.Client{Timeout: 30 * time.Second}
	url := "https://api.github.com/repos/elev1e1nSure/zapret-core/releases/latest"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logError("Failed to create request: %v", err)
		os.Exit(1)
	}
	req.Header.Set("User-Agent", "zapret-core-updater")

	resp, err := client.Do(req)
	if err != nil {
		logError("Failed to fetch release info: %v", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logError("Failed to fetch release info: HTTP %d", resp.StatusCode)
		os.Exit(1)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		logError("Failed to parse release info: %v", err)
		os.Exit(1)
	}

	// Find zip and checksums assets
	var zipURL, checksumURL string
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, "windows-amd64.zip") {
			zipURL = asset.BrowserDownloadURL
		} else if asset.Name == "checksums.txt" {
			checksumURL = asset.BrowserDownloadURL
		}
	}

	if zipURL == "" {
		logError("Windows zip not found in release assets")
		os.Exit(1)
	}

	if checksumURL == "" {
		logError("checksums.txt not found in release assets")
		os.Exit(1)
	}

	logInfo("Downloading update...")

	// Download files
	zipPath, checksumPath, err := downloadRelease(zipURL, checksumURL)
	if err != nil {
		logError("Download failed: %v", err)
		os.Exit(1)
	}
	defer os.Remove(zipPath)
	defer os.Remove(checksumPath)

	// Parse checksum
	zipFilename := filepath.Base(zipPath)
	expectedHash, err := parseChecksum(checksumPath, zipFilename)
	if err != nil {
		logError("Failed to parse checksum: %v", err)
		os.Exit(1)
	}

	// Verify SHA256
	logInfo("Verifying download...")
	if err := verifySHA256(zipPath, expectedHash); err != nil {
		logError("Verification failed: %v", err)
		os.Exit(1)
	}

	// Extract exe
	logInfo("Extracting update...")
	exeDir := getExeDir()
	newExePath := filepath.Join(exeDir, "zapret-core.exe.new")
	if err := extractExe(zipPath, newExePath); err != nil {
		logError("Extraction failed: %v", err)
		os.Exit(1)
	}
	defer os.Remove(newExePath)

	// Apply update
	if err := applyUpdate(newExePath, remoteVersion); err != nil {
		logError("Failed to apply update: %v", err)
		os.Exit(1)
	}
}