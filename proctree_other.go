//go:build !windows

package main

// On Linux and macOS Herdr reports the pane's real foreground process group,
// which is exact: a Vim suspended with Ctrl+Z is correctly not foreground.
// Looking at other processes would get that case wrong, so we don't.
func platformProcessNames(info ProcessInfo) []string { return nil }
