# herdr-vim-navigation

A [Herdr](https://herdr.dev) plugin, written in Go, that lets you move between
Herdr panes and Vim/Neovim splits as if they were one app, on **Linux, macOS,
and Windows**. `Ctrl+h/j/k/l` moves between Vim splits while you're in Vim,
crosses into the neighbouring Herdr pane when Vim hits an edge, and moves
between Herdr panes everywhere else.

It is [`vim-tmux-navigator`](https://github.com/christoomey/vim-tmux-navigator)
for Herdr, and a cross-platform port of
[paulbkim-dev/vim-herdr-navigation](https://github.com/paulbkim-dev/vim-herdr-navigation)
(which needs bash and `jq`, and cannot detect Vim on Windows).

## How it works

There are two cooperating sides, as in `vim-tmux-navigator`:

- **Herdr side** (the Go binary): a Herdr keybind runs a plugin action on each
  press. The action checks what is running in the focused pane. If it's
  Vim/Neovim, it forwards the key into the pane; otherwise it moves Herdr's
  pane focus. If there is no pane in that direction, the key goes into the
  pane instead, so the program there gets it as if the plugin weren't
  installed: a lone pane keeps all four keys (see `forward_at_edge`).
- **Editor side** (`editor/nvim.lua`, `editor/vim.vim`): maps the same keys to
  `wincmd h/j/k/l`. If the window didn't change (Vim is at an edge), it runs
  `herdr pane focus --direction …` to cross into the neighbouring Herdr pane.
  Vim finds its own pane through the `$HERDR_PANE_ID` Herdr sets in every pane.

### Detecting Vim on each platform

| Platform | Signal |
| --- | --- |
| Linux, macOS | Herdr's `pane.process_info` foreground process group. This is exact: a Vim suspended with `Ctrl+Z` is correctly not foreground. |
| Windows | Herdr can only report the pane's shell (ConPTY has no foreground process groups), so the plugin looks at the processes attached to the pane's console. If it can't attach, it walks the shell's process tree, with a guard against Windows PID reuse. |

### Speed

A navigation key costs one process launch plus two Herdr requests. The binary
talks to Herdr's socket directly (a Unix socket, or a named pipe on Windows)
instead of spawning the `herdr` CLI. Measured on Windows 11 inside a Herdr
pane:

| Step | Time |
| --- | --- |
| Launching the plugin binary (Herdr → Go process) | ~17 ms (a native C exe is ~11 ms) |
| Plugin work: `process_info` + Vim detection + focus/send-keys | ~1–2 ms |
| The same two requests through the `herdr` CLI instead | ~55 ms |

If the socket can't be opened, the plugin falls back to the CLI. A request is
never retried after it has been sent, so focus can't move twice.

## Requirements

- Herdr 0.8.0 or newer (Linux, macOS, Windows).
- Go 1.22 or newer at install time. The plugin is compiled during install and
  has no third-party dependencies, so the build works offline.
- Vim or Neovim with the editor script loaded (see below).

## Install

```sh
herdr plugin install jsheely/herdr-vim-navigation
```

For local development, build once and link the checkout in place:

```sh
go build -o bin/herdr-vim-navigation.exe .   # Windows
go build -o bin/herdr-vim-navigation .       # Linux / macOS
herdr plugin link /path/to/herdr-vim-navigation
```

### 1. Bind the keys in Herdr

Plugins cannot register keys themselves, so add these to your Herdr config
(`~/.config/herdr/config.toml`, or `%APPDATA%\herdr\config.toml` on Windows).
Herdr uses none of these chords by default.

```toml
[[keys.command]]
key = "ctrl+h"
type = "plugin_action"
command = "jsheely.vim-navigation.left"
description = "navigate left (vim/herdr)"

[[keys.command]]
key = "ctrl+j"
type = "plugin_action"
command = "jsheely.vim-navigation.down"
description = "navigate down (vim/herdr)"

[[keys.command]]
key = "ctrl+k"
type = "plugin_action"
command = "jsheely.vim-navigation.up"
description = "navigate up (vim/herdr)"

[[keys.command]]
key = "ctrl+l"
type = "plugin_action"
command = "jsheely.vim-navigation.right"
description = "navigate right (vim/herdr)"
```

Then run `herdr server reload-config` (or press the reload binding).

### 2. Wire up your editor

**Neovim.** Load `editor/nvim.lua` after your plugins so it wins over any
other `<C-h/j/k/l>` mapping. With lazy.nvim, fold it into your
`vim-tmux-navigator` spec so there is one source of truth:

```lua
{
  "christoomey/vim-tmux-navigator",
  lazy = false,
  init = function()
    vim.g.tmux_navigator_no_mappings = 1
  end,
  config = function()
    dofile(vim.fn.expand("~/src/herdr-vim-navigation/editor/nvim.lua"))
  end,
}
```

No plugin manager? Copy it into `after/plugin`:

```sh
cp editor/nvim.lua ~/.config/nvim/after/plugin/herdr_navigation.lua           # Linux / macOS
copy editor\nvim.lua %LOCALAPPDATA%\nvim\after\plugin\herdr_navigation.lua   # Windows
```

Outside Herdr it falls back to vim-tmux-navigator (when `$TMUX` is set) or
plain `wincmd`, so an existing tmux setup keeps working.

**Vim.** Source it from your `vimrc` (`_vimrc` or `vimfiles\vimrc` on Windows):

```vim
source /path/to/herdr-vim-navigation/editor/vim.vim
```

Both scripts define `:HerdrNavigateLeft`, `:HerdrNavigateDown`,
`:HerdrNavigateUp`, and `:HerdrNavigateRight`. To use other keys, set
`g:herdr_navigation_no_mappings = 1` before loading and map the commands
yourself. For example, to navigate out of terminal mode as well:

```vim
tnoremap <silent> <C-h> <C-\><C-n>:HerdrNavigateLeft<CR>
```

The editor scripts call Herdr without a shell (`vim.fn.system` with an argv
list in Neovim, `job_start` in Vim), so they work unchanged under cmd.exe,
PowerShell, or bash on Windows.

## Configuration

`herdr plugin config-dir jsheely.vim-navigation` prints the config folder. A
`config.toml` with these defaults is created there on first run:

```toml
# Regex matched against the lower-cased foreground process name (no directory,
# no .exe). A match forwards the key into the pane.
vim_pattern = '^g?(view|l?n?vim?x?)(diff)?$'

# Other programs that should receive the keys themselves instead of Herdr
# moving focus. Exact names, case-insensitive, ".exe" optional.
passthrough = []

# Keys forwarded into Vim, in left, down, up, right order.
keys = ["ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l"]

# When there is no Herdr pane in a direction, send that direction's key into
# the pane instead, as if the plugin weren't installed. Use [] to always just
# move focus.
forward_at_edge = ["left", "down", "up", "right"]

# Log every navigation decision to `herdr plugin log list`.
debug = false
```

The config is read on every keypress, so changes apply immediately. If you
bind different chords in Herdr (say `alt+h/j/k/l`), set `keys` to the same
chords and map them in your editor too.

## Troubleshooting

See what the plugin sees in a pane and what it would do (run from the plugin
directory; `herdr plugin list` shows where it lives):

```sh
./bin/herdr-vim-navigation detect            # inside a Herdr pane: this pane
./bin/herdr-vim-navigation detect w1:p2      # any pane id from `herdr pane list`
```

```text
pane:      w1:p1
processes: pwsh.exe, C:\Program Files\PowerShell\7\pwsh.exe, C:\Program Files\Neovim\bin\nvim.exe, …
decision:  forward keys to nvim (ctrl+h ctrl+j ctrl+k ctrl+l)
```

Set `debug = true` to log each decision and its timing, then:

```sh
herdr plugin log list --plugin jsheely.vim-navigation
```

## Notes & tradeoffs

- **Other TUIs that use `Ctrl+h/j/k/l`** (`lazygit`, `k9s`, …). By default
  every non-Vim pane just moves Herdr focus. To let one of these handle the
  chord itself, add it to `passthrough`. Unlike Vim, these apps can't hand
  focus back at an edge, so use `prefix+h/j/k/l` to leave the pane.
- **`Ctrl+l` / `Ctrl+k` in shells.** Binding these globally would shadow
  readline's clear-screen and kill-line (and PSReadLine's `Ctrl+L` clear on
  Windows). With `forward_at_edge`, a key with nowhere to go reaches the shell
  instead: in a lone pane `Ctrl+L` always clears, and in a split it clears
  from the rightmost pane. Herdr's reply to the focus request says whether
  focus moved, so this adds no request when focus does move. With a pane to
  the right, `Ctrl+L` still moves focus, so bind clear to something like
  `prefix+l` if you want it everywhere, or use `alt+h/j/k/l` for navigation.
  `Ctrl+J` at the bottom edge reaches the shell too, and in bash and zsh it
  is Enter, so one press too many runs the command line. Remove `"down"` from
  `forward_at_edge` if that bites.
- **`Ctrl+h` vs Backspace.** Herdr delivers `ctrl+h` as a distinct key. This
  was verified on Windows ConPTY with Neovim 0.12, where `<C-h>` and `<BS>`
  arrive separately. Older Vims outside the kitty keyboard protocol may still
  see them as the same byte (`0x08`). In that case, map `<BS>` separately if
  it starts navigating.
- **How keys are typed on Windows.** Herdr's `send-keys` types `ctrl+j` as
  its control byte (`0x0A`), and ConPTY reads `0x0A` back as Ctrl+Enter
  (`0x08` as Ctrl+Backspace). Programs that read console key events, such as
  crossterm-based Rust TUIs and PSReadLine, then never see Ctrl+J or Ctrl+H.
  So on Windows the plugin types `ctrl+<letter>` keys as win32-input-mode
  sequences, which ConPTY delivers exactly like a real keypress. Neovim, pwsh
  and crossterm all receive the right key. Other keys still go through
  `send-keys`.
- **WSL and SSH.** A Vim running inside `wsl.exe` or over `ssh` is invisible
  to process detection (the pane shows `wsl.exe`/`ssh`), so the keys move
  Herdr focus instead of reaching that Vim.
- **Windows detection is "attached to the pane's console", not strictly
  "foreground".** Windows shells have no job control, so in practice the two
  are the same.

## Development

```sh
go test ./...   # unit tests, fake Herdr CLI + socket/pipe server, and the
                # editor scripts headlessly when nvim/vim are on PATH
go vet ./... && GOOS=linux go vet ./... && GOOS=darwin go vet ./...
```

| File | Purpose |
| --- | --- |
| `navigate.go` | The per-keypress decision: forward to Vim or move focus. |
| `herdr.go` | Herdr client: direct socket/pipe with CLI fallback. |
| `transport_{unix,windows}.go` | Unix socket vs `\\.\pipe\` named pipe. |
| `proctree_windows.go` | Console process list and process-tree fallback. |
| `config.go` | `config.toml` loading (dependency-free TOML subset). |
| `editor/` | Neovim and Vim halves. |

## License

MIT
