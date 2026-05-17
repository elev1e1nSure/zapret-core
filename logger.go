package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

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

func initLogger() error {
	// Ensure stdout is connected to console
	fmt.Print("")
	
	cwd, _ := os.Getwd()
	logPath := filepath.Join(cwd, "data", "zapret.log")
	
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return err
	}
	
	// Check if file exists to determine if we need to write BOM
	_, err := os.Stat(logPath)
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
	logWriter = io.MultiWriter(os.Stdout, f)
	
	return nil
}

func closeLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

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

func logInfo(format string, args ...interface{}) {
	log(INFO, format, args...)
}

func logWarn(format string, args ...interface{}) {
	log(WARN, format, args...)
}

func logError(format string, args ...interface{}) {
	log(ERROR, format, args...)
}

func logSuccess(format string, args ...interface{}) {
	log(SUCCESS, format, args...)
}

func printBanner() {
	bold := "\033[1m"
	cyan := "\033[36m"
	reset := "\033[0m"
	fmt.Fprintf(logWriter, "\n%s%s╔════════════════════════════════════════════════════════════╗%s\n", bold, cyan, reset)
	fmt.Fprintf(logWriter, "%s%s║%s              zapret-core v1.0.1              %s%s║%s\n", bold, cyan, reset, bold, cyan, reset)
	fmt.Fprintf(logWriter, "%s%s╚════════════════════════════════════════════════════════════╝%s\n\n", bold, cyan, reset)
}
