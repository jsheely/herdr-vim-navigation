package main

import (
	"fmt"
	"io"
	"strings"
)

// Direction is a Herdr pane direction.
type Direction string

// directions is ordered to match Config.Keys.
var directions = [4]Direction{"left", "down", "up", "right"}

func parseDirection(s string) (Direction, bool) {
	for _, d := range directions {
		if string(d) == s {
			return d, true
		}
	}
	return "", false
}

// key returns the key forwarded into Vim for dir.
func (c Config) key(dir Direction) string {
	for i, d := range directions {
		if d == dir {
			return c.Keys[i]
		}
	}
	return ""
}

// forwardsAtEdge reports whether dir's key goes into the pane when Herdr has
// no pane in that direction.
func (c Config) forwardsAtEdge(dir Direction) bool {
	for _, d := range c.ForwardAtEdge {
		if d == dir {
			return true
		}
	}
	return false
}

// platformNames is swapped out by tests so they never inspect real processes.
var platformNames = platformProcessNames

// navigate handles one keypress: forward the key when Vim (or a passthrough
// program) controls the pane, otherwise move Herdr's focus.
func navigate(cfg Config, h Herdr, dir Direction, paneID string) error {
	if paneID == "" {
		// No pane in the invocation context: let Herdr use its focused pane.
		debugf("%s: no target pane; moving focus", dir)
		return moveFocus(cfg, h, dir, "")
	}
	names, err := paneProcessNames(h, paneID)
	if err != nil {
		// Moving focus is the safe default: it never types into the pane.
		logf("%s: process info for %s: %v", dir, paneID, err)
	}
	if target := cfg.forwardTarget(names); target != "" {
		debugf("%s: %s is running %s; sending %s", dir, paneID, target, cfg.key(dir))
		return h.SendKeys(paneID, cfg.key(dir))
	}
	debugf("%s: %s is running %v; moving focus", dir, paneID, names)
	return moveFocus(cfg, h, dir, paneID)
}

// moveFocus moves Herdr's focus. If there is no pane in that direction and
// dir is listed in forward_at_edge, the key goes to the pane instead, so a
// lone shell still gets Ctrl+L (clear screen). Asking Herdr to move and
// reading its answer keeps the common case to one request, and there is no
// window where the layout can change between checking and moving.
func moveFocus(cfg Config, h Herdr, dir Direction, paneID string) error {
	res, err := h.FocusDirection(dir, paneID)
	if err != nil || res.Reason != reasonNoNeighbor || !cfg.forwardsAtEdge(dir) {
		return err
	}
	pane := res.SourcePaneID
	if pane == "" {
		pane = paneID
	}
	if pane == "" {
		return nil
	}
	debugf("%s: no pane that way; sending %s to %s", dir, cfg.key(dir), pane)
	return h.SendKeys(pane, cfg.key(dir))
}

// paneProcessNames lists the programs that may be in control of the pane:
// Herdr's foreground processes, plus on Windows the programs sharing the
// pane's console (see proctree_windows.go).
func paneProcessNames(h Herdr, paneID string) ([]string, error) {
	info, err := h.ProcessInfo(paneID)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, p := range info.Foreground {
		names = append(names, p.Name)
		if p.Argv0 != nil {
			names = append(names, *p.Argv0)
		}
	}
	return append(names, platformNames(info)...), nil
}

// forwardTarget returns the normalized name of the first process that should
// receive the key, or "" when Herdr should move focus instead.
func (c Config) forwardTarget(names []string) string {
	for _, raw := range names {
		n := normalizeProcessName(raw)
		if n == "" {
			continue
		}
		if c.VimPattern.MatchString(n) {
			return n
		}
		for _, p := range c.Passthrough {
			if n == p {
				return n
			}
		}
	}
	return ""
}

// normalizeProcessName reduces a process name or path to a bare lower-cased
// program name: "C:\Program Files\Neovim\bin\nvim.exe" -> "nvim".
func normalizeProcessName(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	p = strings.TrimSuffix(strings.ToLower(p), ".exe")
	return strings.TrimPrefix(p, "-") // login shells appear as "-zsh"
}

// detect prints what the navigator sees in a pane, for troubleshooting.
func detect(cfg Config, h Herdr, paneID string, w io.Writer) error {
	if paneID == "" {
		return fmt.Errorf("detect: no pane id (pass one, or run inside a Herdr pane)")
	}
	names, err := paneProcessNames(h, paneID)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "pane:      %s\n", paneID)
	fmt.Fprintf(w, "processes: %s\n", strings.Join(names, ", "))
	if target := cfg.forwardTarget(names); target != "" {
		fmt.Fprintf(w, "decision:  forward keys to %s (%s)\n", target, strings.Join(cfg.Keys[:], " "))
	} else {
		fmt.Fprintln(w, "decision:  move Herdr pane focus")
	}
	return nil
}
