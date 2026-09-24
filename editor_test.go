package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The editor scripts are tested headlessly when nvim/vim are on PATH.

func editorEnv(t *testing.T) []string {
	t.Helper()
	repo, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(),
		"HERDR_NAV_TEST_REPO="+filepath.ToSlash(repo),
		"HERDR_NAV_TEST_ERRORS="+filepath.ToSlash(filepath.Join(t.TempDir(), "errors.txt")),
		"HERDR_PANE_ID=", "TMUX=")
}

func TestNeovimScript(t *testing.T) {
	nvim, err := exec.LookPath("nvim")
	if err != nil {
		t.Skip("nvim not on PATH")
	}
	cmd := exec.Command(nvim, "--clean", "--headless", "-l", "tests/nvim.lua")
	cmd.Env = editorEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "verification passed") {
		t.Fatalf("tests/nvim.lua failed: %v\n%s", err, out)
	}
}

func TestVimScript(t *testing.T) {
	// Prefer real Vim; the script also runs under Neovim.
	args := []string{"-Nu", "NONE", "-i", "NONE", "-n", "-es", "-S", "tests/vim.vim"}
	bin, err := exec.LookPath("vim")
	if err != nil {
		if bin, err = exec.LookPath("nvim"); err != nil {
			t.Skip("neither vim nor nvim on PATH")
		}
		args = []string{"--clean", "--headless", "-u", "NONE", "-S", "tests/vim.vim"}
	}
	env := editorEnv(t)
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		var errs string
		for _, kv := range env {
			if p, ok := strings.CutPrefix(kv, "HERDR_NAV_TEST_ERRORS="); ok {
				b, _ := os.ReadFile(filepath.FromSlash(p))
				errs = string(b)
			}
		}
		t.Fatalf("tests/vim.vim failed under %s: %v\n%s%s", bin, err, out, errs)
	}
}
