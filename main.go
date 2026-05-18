package main

import (
	"fmt"
	"os"
	"os/signal"
)

func main() {
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
	case "--update":
		printBanner()
		runUpdate()
	default:
		printBanner()
		logInfo("Usage:")
		logInfo("  zapret-core           — run best known strategy")
		logInfo("  zapret-core --find    — find working strategy")
		logInfo("  zapret-core --status  — show status")
		logInfo("  zapret-core --stop    — stop")
		logInfo("  zapret-core --watch   — monitoring + auto-recovery on failure")
		logInfo("  zapret-core --server  — start HTTP API server on :7432")
		logInfo("  zapret-core --update  — update lists from GitHub")
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
	select {} // block forever
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
	logInfo("Провайдер: %s (%s)", provider.ASN, provider.Org)

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
	logInfo("Провайдер: %s (%s)", provider.ASN, provider.Org)

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