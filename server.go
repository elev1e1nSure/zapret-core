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
	server           *http.Server
	mu               sync.Mutex
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
