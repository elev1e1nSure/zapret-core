package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func (s *APIServer) addSubscriber(id string, ch chan string) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	if s.subscribers == nil {
		s.subscribers = make(map[string]chan string)
	}
	// Check if subscriber already exists to prevent duplicates
	if _, exists := s.subscribers[id]; exists {
		return
	}
	s.subscribers[id] = ch
}

func (s *APIServer) removeSubscriber(id string) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	if ch, ok := s.subscribers[id]; ok {
		close(ch)
		delete(s.subscribers, id)
	}
}

func (s *APIServer) emitEvent(event StatusEvent) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	jsonData, err := json.Marshal(event)
	if err != nil {
		return
	}

	droppedCount := 0
	for _, ch := range s.subscribers {
		select {
		case ch <- string(jsonData):
		default:
			droppedCount++
		}
	}
	if droppedCount > 0 {
		logWarn("emitEvent: dropped %d events (channel full)", droppedCount)
	}
}

func (s *APIServer) handleUpdateSelf(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.checkConflict(w, r) {
		return
	}

	s.setOperation("update-self")
	defer s.clearOperation()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	progressChan := make(chan UpdateSelfProgress, 10)

	// Send initial event
	s.sendSSE(w, "message", UpdateSelfProgress{Type: "checking", Message: "Checking for updates..."})
	flusher.Flush()

	// Start update in goroutine
	go func() {
		remoteVersion, err := checkForUpdate()
		if err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Failed to check for updates: %v", err)}
			close(progressChan)
			return
		}

		if remoteVersion == "" {
			progressChan <- UpdateSelfProgress{Type: "up_to_date", Message: fmt.Sprintf("Already up to date (%s)", Version)}
			close(progressChan)
			return
		}

		progressChan <- UpdateSelfProgress{Type: "found", Message: fmt.Sprintf("New version available: %s → %s", Version, remoteVersion)}

		// Get release info
		client := &http.Client{Timeout: 30 * time.Second}
		url := "https://api.github.com/repos/elev1e1nSure/zapret-core/releases/latest"

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Failed to create request: %v", err)}
			close(progressChan)
			return
		}
		req.Header.Set("User-Agent", "zapret-core-updater")

		resp, err := client.Do(req)
		if err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Failed to fetch release info: %v", err)}
			close(progressChan)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Failed to fetch release info: HTTP %d", resp.StatusCode)}
			close(progressChan)
			return
		}

		var release GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Failed to parse release info: %v", err)}
			close(progressChan)
			return
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
			progressChan <- UpdateSelfProgress{Type: "error", Message: "Windows zip not found in release assets"}
			close(progressChan)
			return
		}

		if checksumURL == "" {
			progressChan <- UpdateSelfProgress{Type: "error", Message: "checksums.txt not found in release assets"}
			close(progressChan)
			return
		}

		progressChan <- UpdateSelfProgress{Type: "downloading", Message: fmt.Sprintf("Downloading %s...", filepath.Base(zipURL))}

		// Download files
		zipPath, checksumPath, zipFilename, err := downloadRelease(zipURL, checksumURL)
		if err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Download failed: %v", err)}
			close(progressChan)
			return
		}
		defer os.Remove(zipPath)
		defer os.Remove(checksumPath)

		// Parse checksum
		expectedHash, err := parseChecksum(checksumPath, zipFilename)
		if err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Failed to parse checksum: %v", err)}
			close(progressChan)
			return
		}

		progressChan <- UpdateSelfProgress{Type: "verifying", Message: "Verifying SHA256..."}

		// Verify SHA256
		if err := verifySHA256(zipPath, expectedHash); err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Verification failed: %v", err)}
			close(progressChan)
			return
		}

		progressChan <- UpdateSelfProgress{Type: "applying", Message: "Applying update..."}

		// Extract exe
		exeDir := getExeDir()
		newExePath := filepath.Join(exeDir, "zapret-core.exe.new")
		if err := extractExe(zipPath, newExePath); err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Extraction failed: %v", err)}
			close(progressChan)
			return
		}
		defer os.Remove(newExePath)

		// Apply update
		if err := applyUpdate(newExePath, remoteVersion); err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Failed to apply update: %v", err)}
			close(progressChan)
			return
		}

		progressChan <- UpdateSelfProgress{Type: "success", Message: "Updated successfully. Restarting..."}
		close(progressChan)
	}()

	// Stream progress
	for progress := range progressChan {
		if progress.Type == "error" {
			s.sendSSE(w, "error", ErrorResponse{Error: progress.Message})
			flusher.Flush()
			return
		} else if progress.Type == "success" {
			s.sendSSE(w, "success", SuccessResponse{Status: "updated", Message: progress.Message})
			flusher.Flush()
			// Note: applyUpdate will restart the process, so we don't return here
			return
		} else if progress.Type == "up_to_date" {
			s.sendSSE(w, "success", SuccessResponse{Status: "up_to_date", Message: progress.Message})
			flusher.Flush()
			return
		}

		s.sendSSE(w, "progress", progress)
		flusher.Flush()
	}
}

func (s *APIServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Generate unique subscriber ID
	subscriberID := strconv.FormatUint(atomic.AddUint64(&s.subscriberCount, 1), 10)

	// Create channel for this subscriber
	eventChan := make(chan string, 10)
	s.addSubscriber(subscriberID, eventChan)
	defer s.removeSubscriber(subscriberID)

	// Create context for detecting client disconnect
	ctx := r.Context()

	// Start keep-alive goroutine
	keepAlive := make(chan struct{})
	defer close(keepAlive)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				select {
				case eventChan <- ": ping\n\n":
				default:
				}
			case <-keepAlive:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Send initial status
	s.mu.Lock()
	winwsRunning := IsWinwsRunning()
	watchdogRunning := s.watchdogRunning
	s.mu.Unlock()

	currentStrategy := ""
	if winwsRunning {
		vectors := s.kb.BestForASN(s.provider.ASN, 1)
		if len(vectors) > 0 {
			strategy := VectorToStrategy(vectors[0], 0)
			currentStrategy = strategy.Name
		}
	}

	// Send initial status event
	initialEvent := StatusEvent{
		Type: "status",
	}
	initialEvent.Data.Running = winwsRunning
	initialEvent.Data.Watchdog = watchdogRunning
	initialEvent.Data.Strategy = currentStrategy

	jsonData, err := json.Marshal(initialEvent)
	if err != nil {
		logError("Failed to marshal initial event: %v", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()

	// Stream events
	for {
		select {
		case <-ctx.Done():
			return
		case eventData, ok := <-eventChan:
			if !ok {
				return
			}
			if strings.HasPrefix(eventData, ":") {
				// Keep-alive comment
				fmt.Fprintf(w, "%s\n\n", eventData)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", eventData)
			}
			flusher.Flush()
		}
	}
}
