//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var (
	procCreateNamedPipeW  = kernel32.NewProc("CreateNamedPipeW")
	procConnectNamedPipe  = kernel32.NewProc("ConnectNamedPipe")
	procFlushFileBuffers  = kernel32.NewProc("FlushFileBuffers")
	errorPipeConnected    = syscall.Errno(535) // ERROR_PIPE_CONNECTED
	pipeAccessDuplex      = uintptr(3)
	pipeUnlimitedInstance = uintptr(255)
)

// startFakeServer serves Herdr's newline-delimited JSON protocol on a named
// pipe laid out the way Herdr does it (`\\.\pipe\` + socket path), one
// request per connection, and returns the socket path.
func startFakeServer(t *testing.T, handle func(line []byte) string) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), fmt.Sprintf("herdr-%d.sock", time.Now().UnixNano()))
	name, _ := syscall.UTF16PtrFromString(`\\.\pipe\` + sock)

	listen := func() (syscall.Handle, error) {
		h, _, err := procCreateNamedPipeW.Call(uintptr(unsafe.Pointer(name)), pipeAccessDuplex, 0,
			pipeUnlimitedInstance, 4096, 4096, 0, 0)
		if syscall.Handle(h) == syscall.InvalidHandle {
			return 0, err
		}
		return syscall.Handle(h), nil
	}
	// Create the first instance before returning so the client can connect.
	first, err := listen()
	if err != nil {
		t.Fatalf("CreateNamedPipe: %v", err)
	}
	go func() {
		h := first
		for {
			if r, _, err := procConnectNamedPipe.Call(uintptr(h), 0); r == 0 && err != errorPipeConnected {
				syscall.CloseHandle(h)
				return
			}
			next, err := listen()
			conn := os.NewFile(uintptr(h), "pipe")
			if line, rerr := bufio.NewReader(conn).ReadBytes('\n'); rerr == nil {
				conn.Write([]byte(handle(line) + "\n"))
				procFlushFileBuffers.Call(uintptr(h)) // wait for the client to read it
			}
			conn.Close()
			if err != nil {
				return
			}
			h = next
		}
	}()
	return sock
}
