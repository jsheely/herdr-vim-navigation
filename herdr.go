package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Herdr is the subset of the Herdr API the navigator needs. It is an
// interface so tests can record calls without a running server.
type Herdr interface {
	ProcessInfo(paneID string) (ProcessInfo, error)
	FocusDirection(dir Direction, paneID string) (FocusResult, error)
	SendKeys(paneID string, keys ...string) error
}

// FocusResult is the part of pane.focus_direction's result the navigator
// uses. Herdr versions that predate these fields decode to the zero value,
// which reads as "focus moved".
type FocusResult struct {
	// "no_neighbor" when there is no pane in that direction.
	Reason string `json:"reason"`
	// The pane focus was moving from.
	SourcePaneID string `json:"source_pane_id"`
}

const reasonNoNeighbor = "no_neighbor"

// ProcessInfo mirrors the result of pane.process_info.
type ProcessInfo struct {
	PaneID     string `json:"pane_id"`
	ShellPID   int    `json:"shell_pid"`
	Foreground []struct {
		PID   int     `json:"pid"`
		Name  string  `json:"name"`
		Argv0 *string `json:"argv0"`
	} `json:"foreground_processes"`
}

// client talks to Herdr over its raw socket (a Unix socket, or a named pipe on
// Windows) and falls back to the `herdr` CLI when the socket cannot be opened.
// Going direct matters here: every navigation keypress makes two calls, and
// spawning the CLI costs ~30ms each on Windows versus well under 1ms on the
// socket.
type client struct {
	socketPath string
	bin        string
	seq        int
	// Type Ctrl+letter keys as win32-input-mode sequences (see keys.go).
	win32Input bool
}

func newClient() *client {
	bin := strings.TrimSpace(os.Getenv("HERDR_BIN_PATH"))
	if bin == "" {
		bin = "herdr"
	}
	return &client{
		socketPath: strings.TrimSpace(os.Getenv("HERDR_SOCKET_PATH")),
		bin:        bin,
		win32Input: runtime.GOOS == "windows",
	}
}

func (c *client) ProcessInfo(paneID string) (ProcessInfo, error) {
	var res struct {
		ProcessInfo ProcessInfo `json:"process_info"`
	}
	err := c.call("pane.process_info",
		map[string]any{"pane_id": paneID},
		[]string{"pane", "process-info", "--pane", paneID},
		&res)
	return res.ProcessInfo, err
}

func (c *client) FocusDirection(dir Direction, paneID string) (FocusResult, error) {
	params := map[string]any{"direction": string(dir)}
	args := []string{"pane", "focus", "--direction", string(dir)}
	if paneID != "" {
		params["pane_id"] = paneID
		args = append(args, "--pane", paneID)
	} else {
		args = append(args, "--current")
	}
	var res struct {
		Focus FocusResult `json:"focus"`
	}
	err := c.call("pane.focus_direction", params, args, &res)
	return res.Focus, err
}

func (c *client) SendKeys(paneID string, keys ...string) error {
	if text, ok := win32InputText(keys); ok && c.win32Input {
		return c.call("pane.send_text",
			map[string]any{"pane_id": paneID, "text": text},
			[]string{"pane", "send-text", paneID, text},
			nil)
	}
	return c.call("pane.send_keys",
		map[string]any{"pane_id": paneID, "keys": keys},
		append([]string{"pane", "send-keys", paneID}, keys...),
		nil)
}

// call sends one request and decodes its `result` into out (when non-nil).
// The CLI is used only if the socket could not be opened: once a request has
// been written it may have been applied, so retrying it through the CLI could
// move focus twice.
func (c *client) call(method string, params any, cliArgs []string, out any) error {
	if c.socketPath != "" {
		conn, err := dialHerdr(c.socketPath)
		if err == nil {
			defer conn.Close()
			c.seq++
			line, err := json.Marshal(map[string]any{
				"id":     fmt.Sprintf("vim-nav:%d", c.seq),
				"method": method,
				"params": params,
			})
			if err != nil {
				return err
			}
			if _, err := conn.Write(append(line, '\n')); err != nil {
				return fmt.Errorf("%s: write: %w", method, err)
			}
			resp, err := bufio.NewReader(conn).ReadBytes('\n')
			if err != nil && len(resp) == 0 {
				return fmt.Errorf("%s: read: %w", method, err)
			}
			return decodeEnvelope(method, resp, out)
		}
		debugf("socket %s unavailable (%v); using %s", c.socketPath, err, c.bin)
	}
	return c.cli(cliArgs, out)
}

// cli runs a Herdr CLI command. Its output uses the same JSON envelope as the
// socket; server errors arrive as JSON on stderr with exit status 1.
func (c *client) cli(args []string, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	label := "herdr " + strings.Join(args, " ")
	for _, b := range [][]byte{stdout.Bytes(), stderr.Bytes()} {
		if b = bytes.TrimSpace(b); len(b) > 0 && b[0] == '{' {
			return decodeEnvelope(label, b, out)
		}
	}
	if runErr != nil {
		return fmt.Errorf("%s: %w: %s", label, runErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func decodeEnvelope(label string, raw []byte, out any) error {
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &env); err != nil {
		return fmt.Errorf("%s: invalid response: %w", label, err)
	}
	if env.Error != nil {
		return fmt.Errorf("%s: %s: %s", label, env.Error.Code, env.Error.Message)
	}
	if out == nil {
		return nil
	}
	if env.Result == nil {
		return errors.New(label + ": response has no result")
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("%s: parse result: %w", label, err)
	}
	return nil
}

// PluginContext is the subset of HERDR_PLUGIN_CONTEXT_JSON we use.
type PluginContext struct {
	FocusedPaneID string `json:"focused_pane_id"`
}

// targetPane returns the pane the keypress was aimed at: the focused pane
// from the invocation context, else the pane ids Herdr exports as env vars.
// Empty means "let Herdr use its focused pane".
func targetPane() string {
	if raw := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); raw != "" {
		var c PluginContext
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			logf("invalid HERDR_PLUGIN_CONTEXT_JSON: %v", err)
		} else if c.FocusedPaneID != "" {
			return c.FocusedPaneID
		}
	}
	for _, k := range []string{"HERDR_PANE_ID", "HERDR_ACTIVE_PANE_ID"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

var debugEnabled bool

func debugf(format string, args ...any) {
	if debugEnabled {
		logf(format, args...)
	}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
