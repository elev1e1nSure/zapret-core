package main

import (
	"path/filepath"
	"runtime"
)

// assetsDir returns absolute path to assets/
func assetsDir() string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(file)
	return filepath.Join(root, "assets")
}

// winwsPath returns absolute path to winws.exe
func winwsPath() string {
	return filepath.Join(assetsDir(), "winws.exe")
}

// fake returns absolute path to a file in assets/fake/
func fake(filename string) string {
	return filepath.Join(assetsDir(), "fake", filename)
}

// lists returns absolute path to a file in ../lists/
func lists(filename string) string {
	root := filepath.Dir(winwsPath())
	return filepath.Join(root, "..", "lists", filename)
}

// containsHelper checks if substr exists in s (naive implementation)
func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// contains checks if substr exists in s
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && containsHelper(s, substr))
}

// containsStr checks if substr exists in s (alias for contains)
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}
