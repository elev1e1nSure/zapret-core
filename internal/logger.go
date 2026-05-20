package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	INFO LogLevel = iota
	WARN
	ERROR
	SUCCESS
)

var (
	logFile   *os.File
	logWriter io.Writer
	logMu     sync.Mutex
)

// initLogger initializes the logging system, creates log file, and enables log rotation.
// In daemon mode (--watch, --server) output goes only to the log file, not to stdout.
func initLogger(daemonMode bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	logPath := filepath.Join(cwd, "data", "zapret.log")

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return err
	}

	// Log rotation: trim to 5MB if file exists and is too large
	const maxSize = 5 * 1024 * 1024 // 5MB
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxSize {
		if err := os.Truncate(logPath, 0); err != nil {
			return fmt.Errorf("failed to truncate log file: %w", err)
		}
	}

	// Check if file exists to determine if we need to write BOM
	_, err = os.Stat(logPath)
	fileExists := !os.IsNotExist(err)

	// Open log file in append mode
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// Write UTF-8 BOM if file is new
	if !fileExists {
		_, err = f.Write([]byte{0xEF, 0xBB, 0xBF})
		if err != nil {
			f.Close()
			return err
		}
	}

	logFile = f
	if daemonMode {
		logWriter = f
	} else {
		logWriter = io.MultiWriter(os.Stdout, f)
	}

	return nil
}

// closeLogger closes the log file if it is open
func closeLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

// log writes a formatted message to both console and log file with color coding
func log(level LogLevel, format string, args ...interface{}) {
	logMu.Lock()
	defer logMu.Unlock()

	var prefix, color, icon string
	reset := "\033[0m"

	switch level {
	case INFO:
		prefix = "INFO"
		color = "\033[36m" // cyan
		icon = "ℹ"
	case WARN:
		prefix = "WARN"
		color = "\033[33m" // yellow
		icon = "⚠"
	case ERROR:
		prefix = "ERROR"
		color = "\033[31m" // red
		icon = "✗"
	case SUCCESS:
		prefix = "OK"
		color = "\033[32m" // green
		icon = "✓"
	}

	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(logWriter, "%s[%s]%s %s %s\n", color, prefix, reset, icon, msg)
}

// logInfo writes an informational message
func logInfo(format string, args ...interface{}) {
	log(INFO, format, args...)
}

// logWarn writes a warning message
func logWarn(format string, args ...interface{}) {
	log(WARN, format, args...)
}

// logError writes an error message
func logError(format string, args ...interface{}) {
	log(ERROR, format, args...)
}

// logSuccess writes a success message
func logSuccess(format string, args ...interface{}) {
	log(SUCCESS, format, args...)
}

// printBanner displays a formatted banner with the application name and version
func printBanner() {
	bold := "\033[1m"
	cyan := "\033[36m"
	reset := "\033[0m"

	width := 62 // видимая ширина между ╔ и ╗
	title := "zapret-core " + Version
	padding := width - len(title)
	left := padding / 2
	right := padding - left

	top := fmt.Sprintf("%s%s╔%s╗%s", bold, cyan, strings.Repeat("═", width), reset)
	mid := fmt.Sprintf("%s%s║%s%s%s%s║%s", bold, cyan, reset, strings.Repeat(" ", left)+title+strings.Repeat(" ", right), bold, cyan, reset)
	bot := fmt.Sprintf("%s%s╚%s╝%s", bold, cyan, strings.Repeat("═", width), reset)

	fmt.Fprintf(logWriter, "\n%s\n%s\n%s\n\n", top, mid, bot)
}
