-- herdr-vim-navigation — Neovim side
--
-- Seamless <C-h/j/k/l> navigation between Neovim splits and Herdr panes: move
-- between Neovim splits, and at a split edge hand off to Herdr so focus
-- crosses into the neighbouring Herdr pane. Outside Herdr it falls back to
-- vim-tmux-navigator (if installed) or plain wincmd, so a tmux setup keeps
-- working. Works on Linux, macOS, and Windows.
--
-- Load it after your plugins so it wins over other <C-h/j/k/l> mappings:
--   dofile("/path/to/herdr-vim-navigation/editor/nvim.lua")
-- or copy it to ~/.config/nvim/after/plugin/herdr_navigation.lua
-- (%LOCALAPPDATA%\nvim\after\plugin\ on Windows).
--
-- Set vim.g.herdr_navigation_no_mappings = 1 before loading to skip the
-- default normal-mode <C-h/j/k/l> maps and map the commands yourself:
-- :HerdrNavigateLeft / Down / Up / Right.

local M = {}

-- run executes argv without a shell (tests replace it). Called only at a
-- split edge, so the one blocking Herdr call is off the common path.
function M.run(argv)
  vim.fn.system(argv)
end

local tmux = { left = "Left", down = "Down", up = "Up", right = "Right" }

function M.navigate(wincmd, dir)
  local prev = vim.api.nvim_get_current_win()
  vim.cmd("wincmd " .. wincmd)
  if vim.api.nvim_get_current_win() ~= prev then
    return -- moved within Neovim
  end
  -- At a split edge: cross into the surrounding multiplexer.
  local pane = vim.env.HERDR_PANE_ID
  if pane and pane ~= "" then
    local herdr = vim.env.HERDR_BIN_PATH
    if herdr == nil or herdr == "" then
      herdr = "herdr"
    end
    -- Target this pane explicitly: `--current` resolves to the server's
    -- globally focused pane, which is not necessarily the one we are in.
    M.run({ herdr, "pane", "focus", "--direction", dir, "--pane", pane })
  elseif vim.env.TMUX and vim.env.TMUX ~= "" then
    pcall(vim.cmd, "TmuxNavigate" .. tmux[dir])
  end
end

local dirs = {
  { "h", "left", "Left" },
  { "j", "down", "Down" },
  { "k", "up", "Up" },
  { "l", "right", "Right" },
}

for _, d in ipairs(dirs) do
  local wincmd, dir, name = d[1], d[2], d[3]
  vim.api.nvim_create_user_command("HerdrNavigate" .. name, function()
    M.navigate(wincmd, dir)
  end, { desc = "Navigate " .. dir .. " (Neovim split or Herdr pane)" })

  if vim.g.herdr_navigation_no_mappings ~= 1 then
    vim.keymap.set("n", "<C-" .. wincmd .. ">", function()
      M.navigate(wincmd, dir)
    end, { silent = true, desc = "Navigate " .. dir .. " (Neovim/Herdr)" })
  end
end

return M
