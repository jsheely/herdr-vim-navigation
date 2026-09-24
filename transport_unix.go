//go:build !windows

package main

import (
	"io"
	"net"
	"time"
)

// dialHerdr connects to Herdr's Unix domain socket.
func dialHerdr(socketPath string) (io.ReadWriteCloser, error) {
	conn, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	return conn, nil
}
