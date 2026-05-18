package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"
)

// probeTarget is a URL we check to confirm connectivity
type probeTarget struct {
	name string
	url  string
}

var probeTargets = []probeTarget{
	{"youtube", "https://www.youtube.com/generate_204"},
	{"discord", "https://discord.com/api/v10/gateway"},
}

// Watchdog monitors connection and calls onFail when it detects a breakdown
type Watchdog struct {
	mu       sync.Mutex
	failures int
	lastOK   time.Time
	running  bool
	stopCh   chan struct{}
	onFail   func() // called when failure threshold is reached
}

// NewWatchdog creates a watchdog. onFail is called in a separate goroutine.
func NewWatchdog(onFail func()) *Watchdog {
	return &Watchdog{
		stopCh: make(chan struct{}),
		onFail: onFail,
	}
}

// Start launches the background monitoring loop
func (w *Watchdog) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.mu.Unlock()

	logInfo("[watchdog] started")

	go func() {
		ticker := time.NewTicker(Cfg.CheckIntervalDuration())
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				w.probe()
			case <-w.stopCh:
				logInfo("[watchdog] stopped")
				return
			}
		}
	}()
}

// Stop shuts down the monitoring loop
func (w *Watchdog) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		close(w.stopCh)
		w.running = false
	}
}

// probe checks all targets and updates failure counter
func (w *Watchdog) probe() {
	ok, _ := checkProbeTargets()

	w.mu.Lock()
	defer w.mu.Unlock()

	if ok {
		if w.failures > 0 {
			logInfo("[watchdog] recovered after %d failures", w.failures)
		}
		w.failures = 0
		w.lastOK = time.Now()
		return
	}

	w.failures++
	logWarn("[watchdog] failure %d/%d", w.failures, Cfg.FailThreshold)

	if w.failures >= Cfg.FailThreshold {
		w.failures = 0 // reset so we don't spam onFail
		logWarn("[watchdog] threshold reached — triggering optimizer")
		go w.onFail()
	}
}

// checkProbeTargets returns true if at least one target responds OK
// Checks all targets in parallel
func checkProbeTargets() (bool, map[string]bool) {
	type result struct {
		name string
		ok   bool
	}

	results := make(chan result, len(probeTargets))
	client := &http.Client{Timeout: Cfg.TestTimeoutDuration()}
	details := make(map[string]bool)

	for _, t := range probeTargets {
		t := t
		go func() {
			resp, err := client.Get(t.url)
			if err != nil {
				logWarn("[watchdog] %s unreachable: %v", t.name, err)
				results <- result{t.name, false}
				return
			}
			resp.Body.Close()

			ok := resp.StatusCode < 500
			logInfo("[watchdog] %s → %d", t.name, resp.StatusCode)
			results <- result{t.name, ok}
		}()
	}

	anyOK := false
	for range probeTargets {
		r := <-results
		details[r.name] = r.ok
		if r.ok {
			anyOK = true
		}
	}

	return anyOK, details
}

// ProbeOnce runs a single check and returns true if connection is working
// Used by main to verify state before starting watchdog
func ProbeOnce() bool {
	logInfo("[watchdog] running initial probe...")
	ok, _ := checkProbeTargets()
	if ok {
		logInfo("[watchdog] initial probe: OK")
	} else {
		logWarn("[watchdog] initial probe: FAIL")
	}
	return ok
}

// StartWatchdog creates and starts the watchdog with auto-recovery
func StartWatchdog(asn string, kb *Knowledge) {
	wd := NewWatchdog(func() {
		logInfo("[watchdog] Starting optimizer recovery...")
		result, vector := NewOptimizer(asn, kb).Run()
		if result != nil {
			StopWinws()
			StartWinws(result)
			kb.Record(asn, vector, 1.0)
			logInfo("[watchdog] Recovery successful: %s", result.Name)
		} else {
			logError("[watchdog] Recovery failed - no working strategy found")
		}
	})

	wd.Start()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh

	logInfo("Stopping watchdog...")
	wd.Stop()
	StopWinws()
}

// StartWatchdogBackground starts watchdog in background, returns immediately
// Context can be used to cancel the watchdog
func StartWatchdogBackground(asn string, kb *Knowledge, ctx context.Context) {
	wd := NewWatchdog(func() {
		logInfo("[watchdog] Starting optimizer recovery...")
		result, vector := NewOptimizer(asn, kb).Run()
		if result != nil {
			StopWinws()
			StartWinws(result)
			kb.Record(asn, vector, 1.0)
			logInfo("[watchdog] Recovery successful: %s", result.Name)
		} else {
			logError("[watchdog] Recovery failed - no working strategy found")
		}
	})

	wd.Start()

	// Wait for context cancellation
	go func() {
		<-ctx.Done()
		logInfo("[watchdog] Stopping due to context cancellation...")
		wd.Stop()
		StopWinws()
	}()
}