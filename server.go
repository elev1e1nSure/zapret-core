package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
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
	server          *http.Server
	mu              sync.Mutex
	subscribersMu   sync.Mutex
	subscribers     map[string]chan string
	subscriberCount uint64
	opInProgress    bool
	opType          string
	watchdogRunning bool
	kb              *Knowledge
	provider        ProviderInfo
	watchdogCancel  context.CancelFunc
}

type StatusResponse struct {
	WinwsRunning        bool         `json:"winws_running"`
	WatchdogRunning     bool         `json:"watchdog_running"`
	CurrentStrategy     string       `json:"current_strategy"`
	Provider            ProviderInfo `json:"provider"`
	OperationInProgress bool         `json:"operation_in_progress"`
	OperationType       string       `json:"operation_type"`
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

// SSEEvent is the unified envelope for all server-sent events.
type SSEEvent struct {
	Type    string      `json:"type"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type HealthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
}

type StatusEvent struct {
	Type string `json:"type"`
	Data struct {
		Running  bool   `json:"running"`
		Watchdog bool   `json:"watchdog"`
		Strategy string `json:"strategy"`
	} `json:"data"`
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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
	mux.HandleFunc("/api/health", srv.handleHealth)
	mux.HandleFunc("/api/update-self", srv.handleUpdateSelf)
	mux.HandleFunc("/api/events", srv.handleEvents)
	mux.HandleFunc("/api/logs", srv.handleLogs)

	srv.server = &http.Server{
		Addr:    ":7432",
		Handler: corsMiddleware(mux),
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

// sendEvent writes a unified SSEEvent envelope to the stream.
func (s *APIServer) sendEvent(w http.ResponseWriter, evType, message string, data interface{}) {
	env := SSEEvent{Type: evType, Message: message, Data: data}
	jsonData, err := json.Marshal(env)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *APIServer) sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
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

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.sendJSON(w, http.StatusOK, HealthResponse{OK: true, Version: Version})
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

const (
	sentinelSuccess = -1
	sentinelError   = -2
)

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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	progressChan := make(chan FindProgress, 10)

	conflicts := CheckConflicts()
	if len(conflicts) > 0 {
		s.sendEvent(w, "error", fmt.Sprintf("conflicts detected: %v", conflicts), nil)
		return
	}

	go func() {
		opt := NewOptimizerWithProgress(s.provider.ASN, s.kb, progressChan)
		result, vector := opt.Run()

		if result != nil {
			s.kb.Record(s.provider.ASN, vector, 1.0)
			progressChan <- FindProgress{
				Current:  sentinelSuccess,
				Strategy: result.Name,
				Score:    1.0,
			}
		} else {
			progressChan <- FindProgress{
				Current:  sentinelError,
				Strategy: "",
				Score:    0.0,
			}
		}
		close(progressChan)
	}()

	for progress := range progressChan {
		if progress.Current == sentinelSuccess {
			best := s.kb.BestForASN(s.provider.ASN, 1)
			s.sendEvent(w, "success", "Strategy found", FindSuccessResponse{
				Strategy: VectorToStrategy(best[0], 0),
				Score:    1.0,
				Vector:   best[0],
			})
			return
		} else if progress.Current == sentinelError {
			s.sendEvent(w, "error", "no working strategy found", nil)
			return
		}
		s.sendEvent(w, "progress", fmt.Sprintf("[%d/%d] Testing: %s", progress.Current, progress.Total, progress.Strategy), progress)
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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	progressChan := make(chan UpdateProgress, 10)

	go func() {
		err := UpdateLists(func(current, total int, filename string) {
			progressChan <- UpdateProgress{
				Current:  current,
				Total:    total,
				Filename: filename,
			}
		})

		if err != nil {
			progressChan <- UpdateProgress{Current: sentinelError, Filename: err.Error()}
		} else {
			progressChan <- UpdateProgress{Current: sentinelSuccess}
		}
		close(progressChan)
	}()

	for progress := range progressChan {
		if progress.Current == sentinelError {
			s.sendEvent(w, "error", progress.Filename, nil)
			return
		} else if progress.Current == sentinelSuccess {
			s.sendEvent(w, "success", "lists updated successfully", nil)
			return
		}
		s.sendEvent(w, "progress", fmt.Sprintf("[%d/%d] Updating %s...", progress.Current, progress.Total, progress.Filename), progress)
	}
}
