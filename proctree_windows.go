//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// On Windows Herdr cannot see ConPTY foreground process groups, so
// pane.process_info reports only the pane's shell even while Vim is running
// in it. We find the real program ourselves, from the processes attached to
// the pane's console (fast), or else by walking the shell's descendants.
// Windows shells have no job control, so a program sharing the pane's console
// is effectively in the foreground.

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole              = kernel32.NewProc("AttachConsole")
	procFreeConsole                = kernel32.NewProc("FreeConsole")
	procGetConsoleProcessList      = kernel32.NewProc("GetConsoleProcessList")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	processQueryLimitedInformation = 0x1000
	attachParentProcess            = ^uintptr(0) // ATTACH_PARENT_PROCESS (DWORD -1)
	maxDescendants                 = 512
)

// platformProcessNames adds the programs Herdr cannot see on Windows.
func platformProcessNames(info ProcessInfo) []string {
	if info.ShellPID <= 0 {
		return nil
	}
	names, err := consoleProcessNames(info.ShellPID)
	if err == nil {
		return names
	}
	debugf("console of pid %d: %v; walking the process tree", info.ShellPID, err)
	return descendantNames(info.ShellPID)
}

// consoleProcessNames lists the image paths of every process attached to the
// console that pid belongs to: the pane's pseudoconsole. It briefly attaches
// this process to that console, which costs ~1ms, versus ~15-20ms to snapshot
// every process on a busy machine.
func consoleProcessNames(pid int) ([]string, error) {
	// A process can only be attached to one console. Stdout/stderr are pipes
	// when Herdr runs us, but they are console handles when `detect` runs in a
	// terminal; reattach to the parent's console afterwards so they still work.
	restore := consoleHandlesToRestore()
	procFreeConsole.Call()
	r, _, err := procAttachConsole.Call(uintptr(pid))
	if r == 0 {
		restore()
		return nil, fmt.Errorf("attach: %w", err)
	}
	ids := make([]uint32, 32)
	for {
		n, _, err := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
		if n == 0 {
			procFreeConsole.Call()
			restore()
			return nil, fmt.Errorf("list: %w", err)
		}
		if int(n) <= len(ids) {
			ids = ids[:n]
			break
		}
		ids = make([]uint32, n+8)
	}
	procFreeConsole.Call()
	restore()

	self := uint32(os.Getpid())
	var names []string
	for _, id := range ids {
		if id == self {
			continue
		}
		if name := processImageName(id); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// consoleHandlesToRestore returns a func that, after we detach from our
// console, reattaches to the parent's and rebinds any of os.Stdout/os.Stderr
// that were console handles.
func consoleHandlesToRestore() func() {
	isConsole := func(f *os.File) bool {
		t, err := syscall.GetFileType(syscall.Handle(f.Fd()))
		return err == nil && t == syscall.FILE_TYPE_CHAR
	}
	stdout, stderr := isConsole(os.Stdout), isConsole(os.Stderr)
	if !stdout && !stderr {
		return func() {}
	}
	return func() {
		if r, _, _ := procAttachConsole.Call(attachParentProcess); r == 0 {
			return
		}
		name, _ := syscall.UTF16PtrFromString("CONOUT$")
		h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE,
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
		if err != nil {
			return
		}
		out := os.NewFile(uintptr(h), "CONOUT$")
		if stdout {
			os.Stdout = out
		}
		if stderr {
			os.Stderr = out
		}
	}
}

// processImageName returns the full executable path of pid, or "".
func processImageName(pid uint32) string {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, syscall.MAX_LONG_PATH)
	n := uint32(len(buf))
	if r, _, _ := procQueryFullProcessImageNameW.Call(uintptr(h), 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// descendantNames returns the executable names of the live descendants of
// the given root pids, breadth first. It is the fallback for when the pane's
// console cannot be attached.
func descendantNames(roots ...int) []string {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		debugf("process snapshot: %v", err)
		return nil
	}
	defer syscall.CloseHandle(snap)

	type proc struct {
		pid  uint32
		name string
	}
	children := map[uint32][]proc{}
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		if e.ProcessID == 0 || e.ProcessID == e.ParentProcessID {
			continue
		}
		children[e.ParentProcessID] = append(children[e.ParentProcessID],
			proc{e.ProcessID, syscall.UTF16ToString(e.ExeFile[:])})
	}

	created := map[uint32]int64{}
	createdAt := func(pid uint32) int64 {
		if t, ok := created[pid]; ok {
			return t
		}
		created[pid] = processCreationTime(pid)
		return created[pid]
	}

	seen := map[uint32]bool{}
	var queue []uint32
	for _, r := range roots {
		if r > 0 && !seen[uint32(r)] {
			seen[uint32(r)] = true
			queue = append(queue, uint32(r))
		}
	}
	var names []string
	for len(queue) > 0 && len(seen) < maxDescendants {
		parent := queue[0]
		queue = queue[1:]
		for _, ch := range children[parent] {
			if seen[ch.pid] {
				continue
			}
			// Windows recycles PIDs, and a process keeps its dead parent's PID
			// as ParentProcessID. A "child" created before its parent is an
			// orphan of an earlier process that happened to own this PID.
			if pc, cc := createdAt(parent), createdAt(ch.pid); pc != 0 && cc != 0 && cc < pc {
				continue
			}
			seen[ch.pid] = true
			names = append(names, ch.name)
			queue = append(queue, ch.pid)
		}
	}
	return names
}

// processCreationTime returns the process start time in nanoseconds since
// 1601, or 0 when it cannot be read (exited, or access denied).
func processCreationTime(pid uint32) int64 {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(h)
	var creation, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0
	}
	return creation.Nanoseconds()
}
