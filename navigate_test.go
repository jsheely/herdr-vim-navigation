package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeHerdr records calls in the same "method args" shape the reference
// shell tests log from their mock herdr.
type fakeHerdr struct {
	procs   []string // foreground process names reported by process_info
	infoErr error
	calls   []string
}

func (f *fakeHerdr) ProcessInfo(paneID string) (ProcessInfo, error) {
	f.calls = append(f.calls, "process-info "+paneID)
	var info ProcessInfo
	if f.infoErr != nil {
		return info, f.infoErr
	}
	info.PaneID, info.ShellPID = paneID, 4242
	for i, n := range f.procs {
		info.Foreground = append(info.Foreground, struct {
			PID   int     `json:"pid"`
			Name  string  `json:"name"`
			Argv0 *string `json:"argv0"`
		}{PID: 100 + i, Name: n})
	}
	return info, nil
}

func (f *fakeHerdr) FocusDirection(dir Direction, paneID string) error {
	f.calls = append(f.calls, strings.TrimSpace("focus "+string(dir)+" "+paneID))
	return nil
}

func (f *fakeHerdr) SendKeys(paneID string, keys ...string) error {
	f.calls = append(f.calls, "send-keys "+paneID+" "+strings.Join(keys, " "))
	return nil
}

func (f *fakeHerdr) last() string {
	if len(f.calls) == 0 {
		return ""
	}
	return f.calls[len(f.calls)-1]
}

// stubPlatform replaces the Windows console/process-tree lookup for a test.
func stubPlatform(t *testing.T, names ...string) {
	t.Helper()
	orig := platformNames
	platformNames = func(ProcessInfo) []string { return names }
	t.Cleanup(func() { platformNames = orig })
}

func TestNavigate(t *testing.T) {
	passthrough := defaultConfig()
	passthrough.Passthrough = []string{"lazygit"}
	custom := defaultConfig()
	custom.Keys = [4]string{"alt+h", "alt+j", "alt+k", "alt+l"}

	cases := []struct {
		name     string
		cfg      Config
		procs    []string
		platform []string
		infoErr  error
		dir      Direction
		pane     string
		want     string
	}{
		{name: "nvim forwards", procs: []string{"nvim"}, dir: "left", pane: "w1:p1", want: "send-keys w1:p1 ctrl+h"},
		{name: "vim forwards", procs: []string{"vim"}, dir: "right", pane: "w1:p1", want: "send-keys w1:p1 ctrl+l"},
		{name: "shell moves focus", procs: []string{"zsh"}, dir: "down", pane: "w1:p1", want: "focus down w1:p1"},
		{name: "passthrough forwards", cfg: passthrough, procs: []string{"lazygit"}, dir: "up", pane: "w1:p1", want: "send-keys w1:p1 ctrl+k"},
		{name: "no passthrough moves focus", procs: []string{"lazygit"}, dir: "up", pane: "w1:p1", want: "focus up w1:p1"},
		{name: "custom keys", cfg: custom, procs: []string{"nvim"}, dir: "down", pane: "w1:p1", want: "send-keys w1:p1 alt+j"},
		{name: "no pane uses focused pane", procs: []string{"nvim"}, dir: "right", pane: "", want: "focus right"},
		{name: "process info error moves focus", infoErr: errors.New("boom"), dir: "left", pane: "w1:p1", want: "focus left w1:p1"},
		{
			// Windows: Herdr reports only the shell; Vim is found beside it.
			name: "windows console process forwards", procs: []string{"pwsh.exe"},
			platform: []string{`C:\Program Files\PowerShell\7\pwsh.exe`, `C:\Program Files\Neovim\bin\nvim.exe`},
			dir:      "left", pane: "w4:p1R", want: "send-keys w4:p1R ctrl+h",
		},
		{
			name: "windows shell only moves focus", procs: []string{"pwsh.exe"},
			platform: []string{`C:\Program Files\PowerShell\7\pwsh.exe`},
			dir:      "left", pane: "w4:p1R", want: "focus left w4:p1R",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubPlatform(t, tc.platform...)
			cfg := tc.cfg
			if cfg.VimPattern == nil {
				cfg = defaultConfig()
			}
			f := &fakeHerdr{procs: tc.procs, infoErr: tc.infoErr}
			if err := navigate(cfg, f, tc.dir, tc.pane); err != nil {
				t.Fatal(err)
			}
			if got := f.last(); got != tc.want {
				t.Fatalf("last call %q, want %q (all: %q)", got, tc.want, f.calls)
			}
		})
	}
}

func TestForwardTarget(t *testing.T) {
	cfg := defaultConfig()
	for _, n := range []string{
		"vi", "vim", "nvim", "view", "gvim", "vimdiff", "nvimdiff", "lvim", "vimx",
		"nvim.exe", "NVIM.EXE", `C:\Program Files\Neovim\bin\nvim.exe`, "/usr/bin/vim",
	} {
		if cfg.forwardTarget([]string{n}) == "" {
			t.Errorf("%q should forward", n)
		}
	}
	for _, n := range []string{
		"zsh", "-zsh", "pwsh.exe", "bash", "vimtutor", "neovide", "nvim-qt", "vim-herdr", "lazygit", "",
	} {
		if got := cfg.forwardTarget([]string{n}); got != "" {
			t.Errorf("%q should not forward, matched %q", n, got)
		}
	}
}

func TestNormalizeProcessName(t *testing.T) {
	cases := map[string]string{
		"nvim":                                 "nvim",
		"NVIM.EXE":                             "nvim",
		`C:\Program Files\Neovim\bin\nvim.exe`: "nvim",
		"/opt/homebrew/bin/nvim":               "nvim",
		"-zsh":                                 "zsh",
		"  lazygit.exe ":                       "lazygit",
		`C:\Users\me\scoop\shims\lazygit.EXE`:  "lazygit",
		"":                                     "",
	}
	for in, want := range cases {
		if got := normalizeProcessName(in); got != want {
			t.Errorf("normalizeProcessName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDirection(t *testing.T) {
	for i, s := range []string{"left", "down", "up", "right"} {
		d, ok := parseDirection(s)
		if !ok || d != directions[i] {
			t.Errorf("parseDirection(%q) = %q, %v", s, d, ok)
		}
		if k := defaultConfig().key(d); k != defaultConfig().Keys[i] {
			t.Errorf("key(%q) = %q", d, k)
		}
	}
	if _, ok := parseDirection("sideways"); ok {
		t.Error("sideways should be rejected")
	}
}

func TestDetect(t *testing.T) {
	stubPlatform(t, `C:\Program Files\Neovim\bin\nvim.exe`)
	var out strings.Builder
	if err := detect(defaultConfig(), &fakeHerdr{procs: []string{"pwsh.exe"}}, "w1:p1", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "forward keys to nvim") {
		t.Fatalf("unexpected detect output:\n%s", out.String())
	}
	if err := detect(defaultConfig(), &fakeHerdr{}, "", &out); err == nil {
		t.Fatal("detect without a pane should fail")
	}
}

func TestTargetPane(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"focused_pane_id":"w1:p9","tab_id":"w1:t1"}`)
	t.Setenv("HERDR_PANE_ID", "w1:p2")
	if got := targetPane(); got != "w1:p9" {
		t.Errorf("context pane: got %q", got)
	}
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", "")
	if got := targetPane(); got != "w1:p2" {
		t.Errorf("env pane: got %q", got)
	}
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_ACTIVE_PANE_ID", "")
	if got := targetPane(); got != "" {
		t.Errorf("no pane: got %q", got)
	}
}

func TestProcessNamesIncludeArgv0(t *testing.T) {
	stubPlatform(t)
	argv0 := "/usr/local/bin/nvim"
	f := &fakeHerdr{}
	info := ProcessInfo{}
	info.Foreground = append(info.Foreground, struct {
		PID   int     `json:"pid"`
		Name  string  `json:"name"`
		Argv0 *string `json:"argv0"`
	}{Name: "nv", Argv0: &argv0})
	names, err := paneProcessNames(fixedInfo{f, info}, "w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"nv", argv0}; !reflect.DeepEqual(names, want) {
		t.Fatalf("got %q, want %q", names, want)
	}
}

type fixedInfo struct {
	*fakeHerdr
	info ProcessInfo
}

func (f fixedInfo) ProcessInfo(string) (ProcessInfo, error) { return f.info, nil }
