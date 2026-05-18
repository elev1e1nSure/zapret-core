package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type FindProgress struct {
	Current  int     `json:"current"`
	Total    int     `json:"total"`
	Strategy string  `json:"strategy"`
	Score    float64 `json:"score"`
}

type UpdateProgress struct {
	Current  int    `json:"current"`
	Total    int    `json:"total"`
	Filename string `json:"filename"`
}

type APIServer struct {
	server           *http.Server
	mu               sync.Mutex
	subscribersMu    sync.Mutex
	subscribers      map[string]chan string
	subscriberCount  uint64
	opInProgress     bool
	opType           string
	watchdogRunning  bool
	kb               *Knowledge
	provider         ProviderInfo
	watchdogCancel   context.CancelFunc
}

type StatusResponse struct {
	WinwsRunning        bool          `json:"winws_running"`
	WatchdogRunning     bool          `json:"watchdog_running"`
	CurrentStrategy     string        `json:"current_strategy"`
	Provider            ProviderInfo  `json:"provider"`
	OperationInProgress bool          `json:"operation_in_progress"`
	OperationType       string        `json:"operation_type"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type FindSuccessResponse struct {
	Strategy *Strategy      `json:"strategy"`
	Score    float64        `json:"score"`
	Vector   StrategyVector `json:"vector"`
}

type StartResponse struct {
	Status   string `json:"status"`
	Strategy string `json:"strategy,omitempty"`
}

type KnowledgeResponse struct {
	Entries []KnowledgeEntry `json:"entries"`
	Total   int              `json:"total"`
}

type VersionResponse struct {
	Version string `json:"version"`
}

type UpdateSelfProgress struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type StatusEvent struct {
	Type string `json:"type"`
	Data struct {
		Running  bool   `json:"running"`
		Watchdog bool   `json:"watchdog"`
		Strategy string `json:"strategy"`
	} `json:"data"`
}

func NewAPIServer(kb *Knowledge, provider ProviderInfo) *APIServer {
	mux := http.NewServeMux()
	srv := &APIServer{
		kb:       kb,
		provider: provider,
	}

	mux.HandleFunc("/api/find", srv.handleFind)
	mux.HandleFunc("/api/update", srv.handleUpdate)
	mux.HandleFunc("/api/start", srv.handleStart)
	mux.HandleFunc("/api/stop", srv.handleStop)
	mux.HandleFunc("/api/watchdog", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			srv.handleWatchdogStart(w, r)
		case http.MethodDelete:
			srv.handleWatchdogStop(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/status", srv.handleStatus)
	mux.HandleFunc("/api/provider", srv.handleProvider)
	mux.HandleFunc("/api/knowledge", srv.handleKnowledge)
	mux.HandleFunc("/api/version", srv.handleVersion)
	mux.HandleFunc("/api/update-self", srv.handleUpdateSelf)
	mux.HandleFunc("/api/events", srv.handleEvents)

	srv.server = &http.Server{
		Addr:    ":7432",
		Handler: mux,
	}

	return srv
}

func (s *APIServer) Start(addr string) error {
	s.server.Addr = addr
	logInfo("API server listening on %s", addr)
	return s.server.ListenAndServe()
}

func (s *APIServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	s.mu.Lock()
	if s.watchdogCancel != nil {
		s.watchdogCancel()
		s.watchdogCancel = nil
	}
	s.mu.Unlock()
	
	return s.server.Shutdown(ctx)
}

func (s *APIServer) checkConflict(w http.ResponseWriter, r *http.Request) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.opInProgress {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("operation in progress: %s", s.opType)})
		return true
	}
	
	if s.watchdogRunning {
		// Allow status/provider/knowledge even if watchdog running
		return false
	}
	
	return false
}

func (s *APIServer) setOperation(opType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opInProgress = true
	s.opType = opType
}

func (s *APIServer) clearOperation() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opInProgress = false
	s.opType = ""
}

func (s *APIServer) sendSSE(w http.ResponseWriter, event string, data interface{}) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
	w.(http.Flusher).Flush()
	return nil
}

func (s *APIServer) sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

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

func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	winwsRunning := IsWinwsRunning()
	watchdogRunning := s.watchdogRunning
	opInProgress := s.opInProgress
	opType := s.opType
	s.mu.Unlock()

	currentStrategy := ""
	if winwsRunning {
		vectors := s.kb.BestForASN(s.provider.ASN, 1)
		if len(vectors) > 0 {
			strategy := VectorToStrategy(vectors[0], 0)
			currentStrategy = strategy.Name
		}
	}

	resp := StatusResponse{
		WinwsRunning:        winwsRunning,
		WatchdogRunning:     watchdogRunning,
		CurrentStrategy:     currentStrategy,
		Provider:            s.provider,
		OperationInProgress: opInProgress,
		OperationType:       opType,
	}

	s.sendJSON(w, http.StatusOK, resp)
}

func (s *APIServer) handleProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.sendJSON(w, http.StatusOK, s.provider)
}

func (s *APIServer) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vectors := s.kb.BestForASN(s.provider.ASN, 999)
	entries := []KnowledgeEntry{}
	for _, v := range vectors {
		for _, e := range s.kb.Entries {
			if e.ASN == s.provider.ASN && vectorsEqual(e.Vector, v) {
				entries = append(entries, e)
				break
			}
		}
	}

	resp := KnowledgeResponse{
		Entries: entries,
		Total:   len(entries),
	}

	s.sendJSON(w, http.StatusOK, resp)
}

func (s *APIServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := VersionResponse{
		Version: Version,
	}
	s.sendJSON(w, http.StatusOK, resp)
}

func (s *APIServer) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.checkConflict(w, r) {
		return
	}

	s.setOperation("start")
	defer s.clearOperation()

	vectors := s.kb.BestForASN(s.provider.ASN, 1)
	if len(vectors) == 0 {
		s.sendJSON(w, http.StatusNotFound, ErrorResponse{Error: "no known strategies for this ASN"})
		return
	}

	strategy := VectorToStrategy(vectors[0], 0)
	wasRunning := IsWinwsRunning()
	err := StartWinws(strategy)
	if err != nil {
		s.sendJSON(w, http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed to start winws: %v", err)})
		return
	}

	resp := StartResponse{
		Status:   "started",
		Strategy: strategy.Name,
	}
	s.sendJSON(w, http.StatusOK, resp)

	// Emit event for status change (only if state changed)
	if !wasRunning {
		event := StatusEvent{Type: "status"}
		event.Data.Running = true
		event.Data.Watchdog = s.watchdogRunning
		event.Data.Strategy = strategy.Name
		s.emitEvent(event)
	}
}

func (s *APIServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.checkConflict(w, r) {
		return
	}

	s.setOperation("stop")
	defer s.clearOperation()

	err := StopWinws()
	if err != nil {
		s.sendJSON(w, http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed to stop winws: %v", err)})
		return
	}

	s.sendJSON(w, http.StatusOK, SuccessResponse{Status: "stopped"})

	// Emit event for status change
	event := StatusEvent{Type: "status"}
	event.Data.Running = false
	event.Data.Watchdog = s.watchdogRunning
	event.Data.Strategy = ""
	s.emitEvent(event)
}

func (s *APIServer) handleWatchdogStart(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.watchdogRunning {
		s.mu.Unlock()
		s.sendJSON(w, http.StatusConflict, ErrorResponse{Error: "watchdog already running"})
		return
	}
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.watchdogRunning = true
	s.watchdogCancel = cancel
	s.mu.Unlock()

	go func() {
		StartWatchdogBackground(s.provider.ASN, s.kb, ctx)
	}()

	s.sendJSON(w, http.StatusOK, SuccessResponse{Status: "started", Message: "watchdog running in background"})

	// Emit event for status change
	s.mu.Lock()
	winwsRunning := IsWinwsRunning()
	s.mu.Unlock()

	currentStrategy := ""
	if winwsRunning {
		vectors := s.kb.BestForASN(s.provider.ASN, 1)
		if len(vectors) > 0 {
			strategy := VectorToStrategy(vectors[0], 0)
			currentStrategy = strategy.Name
		}
	}

	event := StatusEvent{Type: "status"}
	event.Data.Running = winwsRunning
	event.Data.Watchdog = true
	event.Data.Strategy = currentStrategy
	s.emitEvent(event)
}

func (s *APIServer) handleWatchdogStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if !s.watchdogRunning {
		s.mu.Unlock()
		s.sendJSON(w, http.StatusConflict, ErrorResponse{Error: "watchdog not running"})
		return
	}
	
	if s.watchdogCancel != nil {
		s.watchdogCancel()
		s.watchdogCancel = nil
	}
	s.watchdogRunning = false
	s.mu.Unlock()

	StopWinws()
	s.sendJSON(w, http.StatusOK, SuccessResponse{Status: "stopped"})

	// Emit event for status change
	event := StatusEvent{Type: "status"}
	event.Data.Running = false
	event.Data.Watchdog = false
	event.Data.Strategy = ""
	s.emitEvent(event)
}

func (s *APIServer) handleFind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.checkConflict(w, r) {
		return
	}

	s.setOperation("find")
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

	progressChan := make(chan FindProgress, 10)

	// Check conflicts first
	conflicts := CheckConflicts()
	if len(conflicts) > 0 {
		s.sendSSE(w, "error", ErrorResponse{Error: fmt.Sprintf("conflicts detected: %v", conflicts)})
		flusher.Flush()
		return
	}

	// Start optimizer in goroutine
	go func() {
		opt := NewOptimizerWithProgress(s.provider.ASN, s.kb, progressChan)
		result, vector := opt.Run()

		if result != nil {
			s.kb.Record(s.provider.ASN, vector, 1.0)
			progressChan <- FindProgress{
				Current:  -1, // sentinel for success
				Strategy: result.Name,
				Score:    1.0,
			}
		} else {
			progressChan <- FindProgress{
				Current: -2, // sentinel for error
				Strategy: "",
				Score: 0.0,
			}
		}
		close(progressChan)
	}()

	// Stream progress
	for progress := range progressChan {
		if progress.Current == -1 {
			// Success
			s.sendSSE(w, "success", FindSuccessResponse{
				Strategy: VectorToStrategy(s.kb.BestForASN(s.provider.ASN, 1)[0], 0),
				Score:    1.0,
				Vector:   s.kb.BestForASN(s.provider.ASN, 1)[0],
			})
			flusher.Flush()
			return
		} else if progress.Current == -2 {
			// Error
			s.sendSSE(w, "error", ErrorResponse{Error: "no working strategy found"})
			flusher.Flush()
			return
		}

		s.sendSSE(w, "progress", progress)
		flusher.Flush()
	}
}

func (s *APIServer) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.checkConflict(w, r) {
		return
	}

	s.setOperation("update")
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

	progressChan := make(chan UpdateProgress, 10)

	// Start update in goroutine
	go func() {
		err := UpdateLists(func(current, total int, filename string) {
			progressChan <- UpdateProgress{
				Current:  current,
				Total:    total,
				Filename: filename,
			}
		})

		if err != nil {
			progressChan <- UpdateProgress{
				Current:  -1, // sentinel for error
				Total:    0,
				Filename: err.Error(),
			}
		} else {
			progressChan <- UpdateProgress{
				Current:  -2, // sentinel for success
				Total:    0,
				Filename: "",
			}
		}
		close(progressChan)
	}()

	// Stream progress
	for progress := range progressChan {
		if progress.Current == -1 {
			// Error
			s.sendSSE(w, "error", ErrorResponse{Error: progress.Filename})
			flusher.Flush()
			return
		} else if progress.Current == -2 {
			// Success
			s.sendSSE(w, "success", SuccessResponse{Status: "updated", Message: "lists updated successfully"})
			flusher.Flush()
			return
		}

		s.sendSSE(w, "progress", progress)
		flusher.Flush()
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
		zipPath, checksumPath, err := downloadRelease(zipURL, checksumURL)
		if err != nil {
			progressChan <- UpdateSelfProgress{Type: "error", Message: fmt.Sprintf("Download failed: %v", err)}
			close(progressChan)
			return
		}
		defer os.Remove(zipPath)
		defer os.Remove(checksumPath)

		// Parse checksum
		zipFilename := filepath.Base(zipPath)
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
