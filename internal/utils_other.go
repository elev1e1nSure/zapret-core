//go:build !windows

package main

// hideConsoleWindow is a no-op stub for non-Windows platforms.
// Prevents build errors when cross-compiling.
func hideConsoleWindow() {}
