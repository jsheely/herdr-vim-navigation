// herdr-vim-navigation is a Herdr plugin binary. Herdr invokes it with one of:
//
//	left|down|up|right   [[actions]] hook: forward the key into Vim, or move pane focus
//	detect [PANE_ID]     troubleshooting: print what the navigator sees in a pane
package main

import (
	"fmt"
	"os"
	"time"
)

// watchdog bounds a keypress if the Herdr server stops answering; named pipe
// reads on Windows have no deadline of their own.
const watchdog = 5 * time.Second

func main() {
	time.AfterFunc(watchdog, func() {
		logf("timed out after %s waiting for Herdr", watchdog)
		os.Exit(1)
	})
	if len(os.Args) < 2 {
		usage()
	}
	cfg := loadConfig()
	debugEnabled = cfg.Debug
	h := newClient()

	var err error
	if os.Args[1] == "detect" {
		pane := targetPane()
		if len(os.Args) > 2 {
			pane = os.Args[2]
		}
		err = detect(cfg, h, pane, os.Stdout)
	} else if dir, ok := parseDirection(os.Args[1]); ok {
		start := time.Now()
		err = navigate(cfg, h, dir, targetPane())
		debugf("%s: done in %s at_unix_ms=%d", dir, time.Since(start).Round(10*time.Microsecond), time.Now().UnixMilli())
	} else {
		usage()
	}
	if err != nil {
		logf("%v", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: herdr-vim-navigation left|down|up|right|detect [PANE_ID]")
	os.Exit(2)
}
