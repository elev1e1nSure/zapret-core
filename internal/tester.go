package main

import (
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// TestTarget represents a URL to check after winws starts
type TestTarget struct {
	Name string
	URL  string
}

var defaultTargets = []TestTarget{
	{"YouTube", "https://www.youtube.com/generate_204"},
	{"Discord", "https://discord.com/api/v10/gateway"},
	{"Google", "https://www.google.com/generate_204"},
}

var winwsStartUnix atomic.Int64

// Strategy represents a DPI bypass strategy with its winws command-line arguments
type Strategy struct {
	Name string
	Args []string
}

// TestResult contains the success score and per-target details for a strategy test
type TestResult struct {
	Score   float64
	Details map[string]bool
}

// hiddenCmd creates an exec.Command that runs without showing a console window
func hiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// StopWinws kills all running winws.exe processes
func StopWinws() error {
	cmd := hiddenCmd("taskkill", "/F", "/IM", "winws.exe")
	if err := cmd.Run(); err != nil {
		// exit code 128 = process not found — not an error
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 128 {
			return fmt.Errorf("taskkill winws.exe: %w", err)
		}
	}
	time.Sleep(1 * time.Second)
	winwsStartUnix.Store(0)
	return nil
}

// IsWinwsRunning checks if winws.exe process is currently running
func IsWinwsRunning() bool {
	cmd := hiddenCmd("tasklist", "/FI", "IMAGENAME eq winws.exe")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(output) > 0
}

// StartWinws launches winws.exe with the given strategy arguments
func StartWinws(s *Strategy) error {
	path := winwsPath()
	cmd := hiddenCmd(path, s.Args...)
	cmd.Dir = assetsDir()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start winws: %w", err)
	}
	winwsStartUnix.Store(time.Now().Unix())
	return nil
}

// TestStrategy tests a strategy by running winws and checking connectivity to test targets
func TestStrategy(s *Strategy) TestResult {
	StopWinws()

	if err := StartWinws(s); err != nil {
		logError("Failed to start winws: %v", err)
		return TestResult{Score: 0, Details: map[string]bool{}}
	}

	time.Sleep(Cfg.InitDelayDuration())

	totalScore := 0.0
	lastDetails := map[string]bool{}

	for i := 0; i < Cfg.TestRuns; i++ {
		score, details := checkTargets()
		totalScore += score
		lastDetails = details
		if i < Cfg.TestRuns-1 {
			time.Sleep(2 * time.Second)
		}
	}

	finalScore := totalScore / float64(Cfg.TestRuns)
	printTestResult(s.Name, finalScore, lastDetails)

	return TestResult{Score: finalScore, Details: lastDetails}
}

// checkTargets checks all default targets in parallel and returns score + per-target results
func checkTargets() (float64, map[string]bool) {
	client := &http.Client{
		Timeout: Cfg.TestTimeoutDuration(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // follow redirects
		},
	}

	details := make(map[string]bool, len(defaultTargets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var okCount int64

	for _, t := range defaultTargets {
		wg.Add(1)
		go func(t TestTarget) {
			defer wg.Done()
			resp, err := client.Get(t.URL)
			ok := err == nil && resp.StatusCode < 500
			if ok {
				resp.Body.Close()
				atomic.AddInt64(&okCount, 1)
			}
			mu.Lock()
			details[t.Name] = ok
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	score := float64(okCount) / float64(len(defaultTargets))
	return score, details
}

// printTestResult logs a human-readable summary of test results
func printTestResult(name string, score float64, details map[string]bool) {
	logInfo("[%s] score=%.2f", name, score)
	for _, t := range defaultTargets {
		if details[t.Name] {
			logInfo("%s:OK", t.Name)
		} else {
			logWarn("%s:FAIL", t.Name)
		}
	}
}

// CheckConflicts checks for known software that breaks WinDivert
func CheckConflicts() []string {
	conflicts := []string{}

	// Service-based conflicts
	services := []string{
		"GoodbyeDPI",
		"discordfix_zapret",
		"winws1",
		"winws2",
		"TracSrvWrapper", // Check Point
		"EPWD",           // Check Point
	}

	for _, svc := range services {
		cmd := hiddenCmd("sc", "query", svc)
		out, _ := cmd.Output()
		if contains(string(out), "RUNNING") {
			conflicts = append(conflicts, svc)
		}
	}

	// AdguardSvc process check
	cmd := hiddenCmd("tasklist", "/FI", "IMAGENAME eq AdguardSvc.exe")
	out, _ := cmd.Output()
	if contains(string(out), "AdguardSvc.exe") {
		conflicts = append(conflicts, "AdguardSvc")
	}

	// Killer NIC service check
	cmd = hiddenCmd("sc", "query")
	out, _ = cmd.Output()
	if contains(string(out), "Killer") {
		conflicts = append(conflicts, "Killer NIC")
	}

	// Intel Connectivity Network Service check
	cmd = hiddenCmd("sc", "query")
	out, _ = cmd.Output()
	if contains(string(out), "Intel") && contains(string(out), "Connectivity") && contains(string(out), "Network") {
		conflicts = append(conflicts, "Intel Connectivity Network Service")
	}

	// SmartByte service check
	cmd = hiddenCmd("sc", "query")
	out, _ = cmd.Output()
	if contains(string(out), "SmartByte") {
		conflicts = append(conflicts, "SmartByte")
	}

	return conflicts
}
