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
)

var (
	logFile   *os.File
	logWriter io.Writer
	logMu     sync.Mutex
)

func initLogger() error {
	cwd, _ := os.Getwd()
	logPath := filepath.Join(cwd, "data", "zapret.log")
	
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return err
	}
	
	// Open log file in append mode
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
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
	
	prefix := "[INFO] "
	switch level {
	case WARN:
		prefix = "[WARN] "
	case ERROR:
		prefix = "[ERROR] "
	}
	
	msg := fmt.Sprintf(prefix+format, args...)
	fmt.Fprintln(logWriter, msg)
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
