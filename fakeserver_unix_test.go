//go:build !windows

package main

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// startFakeServer serves Herdr's newline-delimited JSON protocol on a Unix
// socket, one request per connection, and returns the socket path.
func startFakeServer(t *testing.T, handle func(line []byte) string) string {
	t.Helper()
	// Keep the path short: macOS limits socket paths to 104 bytes.
	dir, err := os.MkdirTemp("", "hvn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "herdr.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadBytes('\n')
				if err != nil {
					return
				}
				conn.Write([]byte(handle(line) + "\n"))
			}()
		}
	}()
	return path
}
