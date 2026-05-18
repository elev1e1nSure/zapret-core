package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	type result struct {
		newVersion string
		err        error
	}
	done := make(chan result, 1)

	go func() {
		newVersion, err := performSelfUpdate(func(stage, msg string) {
			s.sendEvent(w, stage, msg, nil)
		})
		done <- result{newVersion, err}
	}()

	res := <-done

	if res.err != nil {
		s.sendEvent(w, "error", res.err.Error(), nil)
		return
	}

	if res.newVersion == "" {
		s.sendEvent(w, "up_to_date", fmt.Sprintf("Already up to date (%s)", Version), nil)
		return
	}

	s.sendEvent(w, "success", fmt.Sprintf("Update installed (%s → %s). Please restart the server.", Version, res.newVersion), nil)
	s.sendEvent(w, "shutdown", "Server is shutting down for update. Restart to apply.", nil)

	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()
}

func (s *APIServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

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

// handleLogs streams the log file as SSE.
// Query param: ?lines=N (default 100) — how many recent lines to send first.
// Query param: ?download=true — returns entire log file as download instead of SSE.
// After the backlog, new lines are pushed as they are written.
func (s *APIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cwd, _ := os.Getwd()
	logPath := filepath.Join(cwd, "data", "zapret.log")

	// Download mode
	if r.URL.Query().Get("download") == "true" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\"zapret.log\"")
		http.ServeFile(w, r, logPath)
		return
	}

	tail := 100
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			tail = n
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	sendLine := func(line string) {
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return
		}
		s.sendEvent(w, "log", line, nil)
		flusher.Flush()
	}

	// Read backlog: last `tail` lines.
	if lines, err := readLastLines(logPath, tail); err == nil {
		for _, l := range lines {
			sendLine(l)
		}
	}

	// Tail: open file, seek to end, poll for new data every 250ms.
	f, err := os.Open(logPath)
	if err != nil {
		s.sendEvent(w, "error", fmt.Sprintf("cannot open log: %v", err), nil)
		return
	}
	defer f.Close()

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return
	}

	reader := bufio.NewReader(f)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				line, err := reader.ReadString('\n')
				if len(line) > 0 {
					sendLine(line)
				}
				if err != nil {
					break
				}
			}
		}
	}
}

// readLastLines returns up to n last lines from a file efficiently.
func readLastLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}
