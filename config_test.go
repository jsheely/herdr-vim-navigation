package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseSimpleTOML(t *testing.T) {
	src := `# comment
passthrough = ["lazygit", 'k9s'] # trailing comment
vim_pattern = '^(n?vim|\w+diff)$'  # literal string: no escapes
quoted = "a \"b\" # not a comment"
debug = true
`
	got, err := parseSimpleTOML(src)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"passthrough": []string{"lazygit", "k9s"},
		"vim_pattern": `^(n?vim|\w+diff)$`,
		"quoted":      `a "b" # not a comment`,
		"debug":       true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseSimpleTOMLRejectsTables(t *testing.T) {
	if _, err := parseSimpleTOML("[keys]\nx = 1\n"); err == nil {
		t.Fatal("expected error for table header")
	}
}

// The file written on first run must parse back to exactly the defaults.
func TestDefaultConfigFileRoundTrips(t *testing.T) {
	cfg, err := parseConfig(defaultConfigTOML)
	if err != nil {
		t.Fatal(err)
	}
	def := defaultConfig()
	if cfg.VimPattern.String() != def.VimPattern.String() || cfg.Keys != def.Keys ||
		cfg.Debug != def.Debug || len(cfg.Passthrough) != 0 {
		t.Fatalf("default file parsed to %+v, want %+v", cfg, def)
	}
}

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig(`
passthrough = ["LazyGit.exe", "k9s", ""]
keys = ["alt+h", "alt+j", "alt+k", "alt+l"]
debug = true
`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"lazygit", "k9s"}; !reflect.DeepEqual(cfg.Passthrough, want) {
		t.Errorf("passthrough = %q, want %q", cfg.Passthrough, want)
	}
	if cfg.Keys != [4]string{"alt+h", "alt+j", "alt+k", "alt+l"} || !cfg.Debug {
		t.Errorf("unexpected config %+v", cfg)
	}
}

// One bad value is reported but does not throw away the rest of the file.
func TestParseConfigKeepsGoodValues(t *testing.T) {
	cfg, err := parseConfig(`
vim_pattern = "("
keys = ["ctrl+h"]
passthrough = ["lazygit"]
`)
	if err == nil || !strings.Contains(err.Error(), "vim_pattern") || !strings.Contains(err.Error(), "keys") {
		t.Fatalf("expected vim_pattern and keys errors, got %v", err)
	}
	if cfg.VimPattern.String() != defaultVimPattern || cfg.Keys != defaultConfig().Keys {
		t.Errorf("bad values should fall back to defaults: %+v", cfg)
	}
	if !reflect.DeepEqual(cfg.Passthrough, []string{"lazygit"}) {
		t.Errorf("good values should still apply: %q", cfg.Passthrough)
	}
}

func TestLoadConfigWritesDefaultFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)
	loadConfig()
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != defaultConfigTOML {
		t.Fatal("first run should write the commented default config")
	}
}
