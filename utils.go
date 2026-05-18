//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// hideConsoleWindow hides the console window using WinAPI.
// Called before any log output in daemon modes (--server, --watch) to prevent window flash.
func hideConsoleWindow() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, 0) // SW_HIDE = 0
	}
}

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

// exePath returns the full path of the running executable.
// Falls back to "zapret-core.exe" on error.
func exePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "zapret-core.exe"
	}
	return exe
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
