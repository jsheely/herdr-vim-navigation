package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// defaultVimPattern is vim-tmux-navigator's matcher: vi, vim, nvim, view,
// gvim, vimdiff, nvimdiff, lvim, vimx, ...
const defaultVimPattern = `^g?(view|l?n?vim?x?)(diff)?$`

// Config is the user-editable plugin configuration, read from
// $HERDR_PLUGIN_CONFIG_DIR/config.toml. A commented default file is written
// there on first run.
type Config struct {
	// Process names (lower-cased, without directory or .exe) that mean Vim
	// is in control of the pane and should receive the key.
	VimPattern *regexp.Regexp
	// Extra programs that should receive the key instead of Herdr moving
	// focus, e.g. lazygit or k9s. Matched exactly, case-insensitively.
	Passthrough []string
	// Keys forwarded into Vim for left, down, up, right. Must match the keys
	// the editor side maps.
	Keys [4]string
	// Log every decision to the plugin log.
	Debug bool
}

const defaultConfigTOML = `# herdr-vim-navigation configuration
#
# Regex matched against the lower-cased foreground process name (no directory,
# no .exe). A match forwards the key into the pane so Vim/Neovim can move
# between its own splits. Default: vim-tmux-navigator's matcher. Use a
# 'single-quoted' string so backslashes in the regex need no escaping.
vim_pattern = '^g?(view|l?n?vim?x?)(diff)?$'

# Other programs that should receive the keys themselves instead of Herdr
# moving focus. Exact names, case-insensitive, ".exe" optional. Unlike Vim
# they cannot hand focus back at an edge; use prefix+h/j/k/l to leave them.
passthrough = []

# Keys forwarded into Vim, in left, down, up, right order. These must match
# the keys your editor maps (editor/nvim.lua and editor/vim.vim map Ctrl+hjkl).
keys = ["ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l"]

# Log every navigation decision to ` + "`herdr plugin log list`" + `.
debug = false
`

func defaultConfig() Config {
	return Config{
		VimPattern: regexp.MustCompile(defaultVimPattern),
		Keys:       [4]string{"ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l"},
	}
}

func configPath() string {
	dir := os.Getenv("HERDR_PLUGIN_CONFIG_DIR")
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config.toml")
}

// loadConfig reads the config file, creating it with defaults when missing.
// Any parse problem falls back to defaults and is reported on stderr (which
// lands in `herdr plugin log`).
func loadConfig() Config {
	path := configPath()
	if path == "" {
		return defaultConfig()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			_ = os.WriteFile(path, []byte(defaultConfigTOML), 0o644)
		} else {
			logf("read %s: %v", path, err)
		}
		return defaultConfig()
	}
	cfg, err := parseConfig(string(data))
	if err != nil {
		logf("%s: %v", path, err)
	}
	return cfg
}

// parseConfig applies the file's values over the defaults. Invalid values are
// reported and skipped individually so one typo does not disable the rest.
func parseConfig(src string) (Config, error) {
	cfg := defaultConfig()
	values, err := parseSimpleTOML(src)
	if err != nil {
		return cfg, fmt.Errorf("%w (using defaults)", err)
	}
	var problems []string
	if v, ok := values["vim_pattern"].(string); ok && strings.TrimSpace(v) != "" {
		if re, err := regexp.Compile(v); err != nil {
			problems = append(problems, fmt.Sprintf("vim_pattern: %v", err))
		} else {
			cfg.VimPattern = re
		}
	}
	if v, ok := values["passthrough"].([]string); ok {
		for _, name := range v {
			if n := normalizeProcessName(name); n != "" {
				cfg.Passthrough = append(cfg.Passthrough, n)
			}
		}
	}
	if v, ok := values["keys"].([]string); ok {
		if len(v) != 4 {
			problems = append(problems, fmt.Sprintf("keys: want 4 entries (left, down, up, right), got %d", len(v)))
		} else {
			for i, k := range v {
				if k = strings.TrimSpace(k); k != "" {
					cfg.Keys[i] = k
				}
			}
		}
	}
	if v, ok := values["debug"].(bool); ok {
		cfg.Debug = v
	}
	if len(problems) > 0 {
		return cfg, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return cfg, nil
}

// parseSimpleTOML handles the flat subset of TOML this plugin uses:
// `key = "string"`, `key = ["a", "b"]`, `key = 12`, `key = 1.5`, `key = true`,
// with `#` comments and blank lines. Tables are rejected so that a typo does
// not silently misconfigure the plugin.
func parseSimpleTOML(src string) (map[string]any, error) {
	out := map[string]any{}
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			return nil, fmt.Errorf("line %d: tables are not supported", i+1)
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected key = value", i+1)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		v, err := parseTOMLValue(val)
		if err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", i+1, key, err)
		}
		out[key] = v
	}
	return out, nil
}

func stripComment(line string) string {
	var quote byte // the open string's delimiter, 0 outside strings
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\\' && quote == '"':
			i++
		case c == '"' || c == '\'':
			if quote == 0 {
				quote = c
			} else if quote == c {
				quote = 0
			}
		case c == '#' && quote == 0:
			return line[:i]
		}
	}
	return line
}

func parseTOMLValue(val string) (any, error) {
	switch {
	case val == "":
		return nil, fmt.Errorf("missing value")
	case val[0] == '"' || val[0] == '\'':
		s, rest, err := parseTOMLString(val)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(rest) != "" {
			return nil, fmt.Errorf("unexpected trailing %q", rest)
		}
		return s, nil
	case val[0] == '[':
		body := strings.TrimSpace(val[1:])
		items := []string{}
		for {
			body = strings.TrimSpace(body)
			if strings.HasPrefix(body, "]") {
				if strings.TrimSpace(body[1:]) != "" {
					return nil, fmt.Errorf("unexpected trailing %q", body[1:])
				}
				return items, nil
			}
			if body == "" {
				return nil, fmt.Errorf("unterminated array")
			}
			if body[0] != '"' && body[0] != '\'' {
				return nil, fmt.Errorf("only string arrays are supported")
			}
			s, rest, err := parseTOMLString(body)
			if err != nil {
				return nil, err
			}
			items = append(items, s)
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, ",") {
				rest = rest[1:]
			} else if !strings.HasPrefix(rest, "]") {
				return nil, fmt.Errorf("expected , or ] after string")
			}
			body = rest
		}
	case val == "true":
		return true, nil
	case val == "false":
		return false, nil
	default:
		f, err := strconv.ParseFloat(strings.ReplaceAll(val, "_", ""), 64)
		if err != nil {
			return nil, fmt.Errorf("unsupported value %q", val)
		}
		return f, nil
	}
}

// parseTOMLString parses a basic "..." or literal '...' string at the start
// of s and returns the decoded value plus the remainder. Literal strings take
// no escapes, which keeps regexes like '\d+' readable.
func parseTOMLString(s string) (string, string, error) {
	if s != "" && s[0] == '\'' {
		end := strings.IndexByte(s[1:], '\'')
		if end < 0 {
			return "", "", fmt.Errorf("unterminated string")
		}
		return s[1 : 1+end], s[2+end:], nil
	}
	if s == "" || s[0] != '"' {
		return "", "", fmt.Errorf("expected string")
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			return b.String(), s[i+1:], nil
		case '\\':
			i++
			if i >= len(s) {
				return "", "", fmt.Errorf("unterminated escape")
			}
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"', '\\':
				b.WriteByte(s[i])
			default:
				return "", "", fmt.Errorf("unsupported escape \\%c", s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", "", fmt.Errorf("unterminated string")
}
