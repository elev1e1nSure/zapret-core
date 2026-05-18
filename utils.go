package main

import (
	"os"
	"path/filepath"
	"strings"
)

// exeDir returns the directory of the running executable.
// Falls back to the current working directory on error.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		cwd, _ := os.Getwd()
		return cwd
	}
	return filepath.Dir(exe)
}

// assetsDir returns absolute path to assets/
func assetsDir() string {
	return filepath.Join(exeDir(), "assets")
}

// winwsPath returns absolute path to winws.exe
func winwsPath() string {
	return filepath.Join(assetsDir(), "winws.exe")
}

// fake returns absolute path to a file in assets/fake/
func fake(filename string) string {
	return filepath.Join(assetsDir(), "fake", filename)
}

// lists returns absolute path to a file in lists/
func lists(filename string) string {
	return filepath.Join(exeDir(), "lists", filename)
}

// contains reports whether substr is within s.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// containsStr reports whether substr is within s.
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}
