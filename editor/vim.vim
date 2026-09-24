" herdr-vim-navigation — Vim side
"
" Maps <C-h/j/k/l> to move between Vim splits. When there is no split in the
" requested direction (Vim is at an edge), hand off to Herdr so focus crosses
" into the neighbouring Herdr pane. Outside Herdr it falls back to
" vim-tmux-navigator (if installed) or plain wincmd. Works on Linux, macOS,
" and Windows.
"
" Load it from your vimrc:
"   source /path/to/herdr-vim-navigation/editor/vim.vim
"
" Set g:herdr_navigation_no_mappings = 1 before loading to skip the default
" normal-mode maps and map the commands yourself:
" :HerdrNavigateLeft / Down / Up / Right.

if exists('g:loaded_herdr_navigation')
  finish
endif
let g:loaded_herdr_navigation = 1

function! s:Run(argv) abort
  if exists('g:HerdrNavigationRun')
    " Test hook.
    call call(g:HerdrNavigationRun, [a:argv])
  elseif has('job')
    " An argv list runs without a shell, so quoting is right on every OS no
    " matter what 'shell' is set to.
    call job_start(a:argv, {'in_io': 'null', 'out_io': 'null', 'err_io': 'null'})
  else
    call system(join(map(copy(a:argv), 'shellescape(v:val)'), ' '))
  endif
endfunction

let s:tmux = {'left': 'Left', 'down': 'Down', 'up': 'Up', 'right': 'Right'}

function! s:Navigate(wincmd, dir) abort
  let l:prev = winnr()
  execute 'wincmd ' . a:wincmd
  if winnr() != l:prev
    return
  endif
  " At a split edge: cross into the surrounding multiplexer.
  if !empty($HERDR_PANE_ID)
    let l:herdr = empty($HERDR_BIN_PATH) ? 'herdr' : $HERDR_BIN_PATH
    " Target this pane explicitly: --current resolves to the server's globally
    " focused pane, which is not necessarily the Vim instance making the call.
    call s:Run([l:herdr, 'pane', 'focus', '--direction', a:dir, '--pane', $HERDR_PANE_ID])
  elseif !empty($TMUX) && exists(':TmuxNavigate' . s:tmux[a:dir]) == 2
    execute 'TmuxNavigate' . s:tmux[a:dir]
  endif
endfunction

command! HerdrNavigateLeft  call s:Navigate('h', 'left')
command! HerdrNavigateDown  call s:Navigate('j', 'down')
command! HerdrNavigateUp    call s:Navigate('k', 'up')
command! HerdrNavigateRight call s:Navigate('l', 'right')

if !get(g:, 'herdr_navigation_no_mappings', 0)
  nnoremap <silent> <C-h> :<C-u>HerdrNavigateLeft<CR>
  nnoremap <silent> <C-j> :<C-u>HerdrNavigateDown<CR>
  nnoremap <silent> <C-k> :<C-u>HerdrNavigateUp<CR>
  nnoremap <silent> <C-l> :<C-u>HerdrNavigateRight<CR>
endif
