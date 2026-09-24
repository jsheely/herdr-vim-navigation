" Run by editor_test.go: vim -Nu NONE -i NONE -n -es -S tests/vim.vim
" (or the same script under nvim --headless).
let s:calls = []
function! s:Record(argv) abort
  call add(s:calls, join(a:argv, ' '))
endfunction
let g:HerdrNavigationRun = function('s:Record')

let $HERDR_PANE_ID = 'w-test:p1'
let $HERDR_BIN_PATH = 'herdr-test'
execute 'source ' . fnameescape($HERDR_NAV_TEST_REPO . '/editor/vim.vim')

for s:key in ['<C-h>', '<C-j>', '<C-k>', '<C-l>']
  call assert_false(empty(maparg(s:key, 'n')), s:key . ' must have a normal-mode mapping')
endfor
for s:name in ['Left', 'Down', 'Up', 'Right']
  call assert_equal(2, exists(':HerdrNavigate' . s:name), ':HerdrNavigate' . s:name . ' must exist')
endfor

" Inside Vim: move between splits without calling Herdr.
vsplit
wincmd l
let s:right = win_getid()
HerdrNavigateLeft
call assert_notequal(s:right, win_getid(), 'Ctrl+h must move inside Vim first')
call assert_equal([], s:calls, 'internal split movement must not call Herdr')

" At an edge: hand off to Herdr, targeting this pane explicitly.
only
HerdrNavigateRight
call assert_equal(['herdr-test pane focus --direction right --pane w-test:p1'], s:calls)

" Outside Herdr (and tmux): stay put, call nothing.
let $HERDR_PANE_ID = ''
let $TMUX = ''
HerdrNavigateDown
call assert_equal(1, len(s:calls), 'must not call Herdr outside a Herdr pane')

if !empty(v:errors)
  call writefile(v:errors, $HERDR_NAV_TEST_ERRORS)
  cquit 1
endif
quitall!
