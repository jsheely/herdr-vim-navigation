-- Run by editor_test.go: nvim --clean --headless -l tests/nvim.lua
local repo = assert(vim.env.HERDR_NAV_TEST_REPO, "HERDR_NAV_TEST_REPO is required")

local nav = dofile(repo .. "/editor/nvim.lua")
local calls = {}
nav.run = function(argv)
  table.insert(calls, table.concat(argv, " "))
end

local function callback(lhs)
  for _, m in ipairs(vim.api.nvim_get_keymap("n")) do
    if m.lhs == lhs then
      assert(type(m.callback) == "function", lhs .. " must use a Lua callback")
      return m.callback
    end
  end
  error(lhs .. " must have a normal-mode mapping")
end

for _, name in ipairs({ "Left", "Down", "Up", "Right" }) do
  assert(vim.fn.exists(":HerdrNavigate" .. name) == 2, "missing :HerdrNavigate" .. name)
end

vim.env.HERDR_PANE_ID = "w-test:p1"
vim.env.HERDR_BIN_PATH = "herdr-test"

-- Inside Neovim: move between splits without calling Herdr.
vim.cmd("vsplit")
vim.cmd("wincmd l")
local right = vim.api.nvim_get_current_win()
callback("<C-H>")()
assert(vim.api.nvim_get_current_win() ~= right, "Ctrl+h must move inside Neovim first")
assert(#calls == 0, "internal split movement must not call Herdr")

-- At an edge: hand off to Herdr, targeting this pane explicitly.
vim.cmd("only")
callback("<C-L>")()
assert(#calls == 1, "edge navigation must make one Herdr call, made " .. #calls)
assert(calls[1] == "herdr-test pane focus --direction right --pane w-test:p1", "unexpected call: " .. calls[1])

vim.cmd("HerdrNavigateUp")
assert(calls[2] == "herdr-test pane focus --direction up --pane w-test:p1", "unexpected call: " .. tostring(calls[2]))

-- Outside Herdr (and tmux): stay put, call nothing.
vim.env.HERDR_PANE_ID = nil
vim.env.TMUX = nil
callback("<C-J>")()
assert(#calls == 2, "must not call Herdr outside a Herdr pane")

print("Neovim navigation verification passed")
