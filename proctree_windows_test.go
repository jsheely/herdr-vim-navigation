//go:build windows

package main

import (
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

// startChild runs a short-lived console program (ping) as our child.
func startChild(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("ping", "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start ping: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd
}

func normalized(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = normalizeProcessName(n)
	}
	return out
}

func TestDescendantNames(t *testing.T) {
	child := startChild(t)
	names := normalized(descendantNames(os.Getpid()))
	if !slices.Contains(names, "ping") {
		t.Fatalf("descendants of the test process %v should include ping (pid %d)", names, child.Process.Pid)
	}
}

func TestConsoleProcessNames(t *testing.T) {
	child := startChild(t)
	// A just-started process cannot be attached to until it has finished
	// connecting to its console, so give it a moment.
	var names []string
	var err error
	for i := 0; i < 50; i++ {
		if names, err = consoleProcessNames(child.Process.Pid); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Skipf("no attachable console in this environment: %v", err)
	}
	got := normalized(names)
	if !slices.Contains(got, "ping") {
		t.Fatalf("console of pid %d lists %v, want ping", child.Process.Pid, got)
	}
	// Our own process attaches to read the list and must not report itself.
	self, _ := os.Executable()
	if slices.Contains(names, self) {
		t.Fatalf("console list should exclude this process: %v", names)
	}
	// We reattached afterwards, so logging still works.
	if _, err := os.Stderr.WriteString(""); err != nil {
		t.Fatalf("stderr broken after console round trip: %v", err)
	}
}

func TestPlatformProcessNamesWithoutShell(t *testing.T) {
	if names := platformProcessNames(ProcessInfo{}); names != nil {
		t.Fatalf("no shell pid should yield no names, got %v", names)
	}
}
