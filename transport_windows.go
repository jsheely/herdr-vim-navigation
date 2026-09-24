//go:build windows

package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
)

const errorPipeBusy syscall.Errno = 231 // ERROR_PIPE_BUSY

// dialHerdr opens Herdr's named pipe. Herdr serves it at `\\.\pipe\` followed
// by the socket path it exports in HERDR_SOCKET_PATH. Pipe handles opened this
// way have no read deadline; main's watchdog bounds a stuck server instead.
func dialHerdr(socketPath string) (io.ReadWriteCloser, error) {
	name := socketPath
	if !strings.HasPrefix(name, `\\.\pipe\`) {
		name = `\\.\pipe\` + name
	}
	for attempt := 0; ; attempt++ {
		f, err := os.OpenFile(name, os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
		// Every pipe instance is momentarily taken; retry briefly.
		if !errors.Is(err, errorPipeBusy) || attempt >= 20 {
			return nil, err
		}
		time.Sleep(5 * time.Millisecond)
	}
}
