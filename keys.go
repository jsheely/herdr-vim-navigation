package main

import (
	"fmt"
	"strings"
)

// Herdr types a key like ctrl+j into a pane as its legacy control byte (0x0A).
// On Windows the pane is a ConPTY, which turns those bytes back into console
// key events by guessing: 0x0A becomes Ctrl+Enter and 0x08 Ctrl+Backspace. Vim
// reads the bytes and is fine, but programs that read console key events
// (crossterm, so most Rust TUIs) never see Ctrl+J or Ctrl+H. ConPTY also
// accepts win32-input-mode sequences, which carry the full key event, so on
// Windows the plugin types Ctrl+letter keys that way: they arrive exactly as a
// real keypress would.

// scanCodes are the set-1 keyboard scan codes for a-z.
var scanCodes = [26]byte{
	0x1E, 0x30, 0x2E, 0x20, 0x12, 0x21, 0x22, 0x23, 0x17, 0x24, 0x25, 0x26, 0x32,
	0x31, 0x18, 0x19, 0x10, 0x13, 0x1F, 0x14, 0x16, 0x2F, 0x11, 0x2D, 0x15, 0x2C,
}

const leftCtrlPressed = 0x0008

// win32InputSequence returns the win32-input-mode press and release for a
// ctrl+<letter> key, or false for any other key.
func win32InputSequence(key string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(key))
	letter, ok := strings.CutPrefix(k, "ctrl+")
	if !ok || len(letter) != 1 || letter[0] < 'a' || letter[0] > 'z' {
		return "", false
	}
	i := letter[0] - 'a'
	vk := 'A' + int(i) // virtual key codes for letters are their uppercase ASCII
	char := int(i) + 1 // the control character, ctrl+a = 0x01
	sc := scanCodes[i]
	// ESC [ Vk ; Sc ; Uc ; Kd ; Cs ; Rc _
	return fmt.Sprintf("\x1b[%d;%d;%d;1;%d;1_\x1b[%d;%d;%d;0;%d;1_",
		vk, sc, char, leftCtrlPressed, vk, sc, char, leftCtrlPressed), true
}

// win32InputText encodes every key as win32-input-mode, or returns false if
// any of them can't be.
func win32InputText(keys []string) (string, bool) {
	var b strings.Builder
	for _, k := range keys {
		seq, ok := win32InputSequence(k)
		if !ok {
			return "", false
		}
		b.WriteString(seq)
	}
	return b.String(), len(keys) > 0
}
