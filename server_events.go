package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
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
			s.sendSSE(w, "progress", UpdateSelfProgress{Type: stage, Message: msg})
			flusher.Flush()
		})
		done <- result{newVersion, err}
	}()

	res := <-done

	if res.err != nil {
		s.sendSSE(w, "error", ErrorResponse{Error: res.err.Error()})
		flusher.Flush()
		return
	}

	if res.newVersion == "" {
		s.sendSSE(w, "success", SuccessResponse{
			Status:  "up_to_date",
			Message: fmt.Sprintf("Already up to date (%s)", Version),
		})
		flusher.Flush()
		return
	}

	s.sendSSE(w, "success", SuccessResponse{
		Status:  "updated",
		Message: fmt.Sprintf("Update installed (%s → %s). Please restart the server.", Version, res.newVersion),
	})
	flusher.Flush()

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
