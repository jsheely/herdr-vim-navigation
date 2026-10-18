package main

import (
	"reflect"
	"testing"
)

func TestWin32InputSequence(t *testing.T) {
	// The ctrl+j, ctrl+k and ctrl+h sequences were checked by hand against a
	// crossterm app in a Herdr pane on Windows 11.
	for key, want := range map[string]string{
		"ctrl+j": "\x1b[74;36;10;1;8;1_\x1b[74;36;10;0;8;1_",
		"ctrl+k": "\x1b[75;37;11;1;8;1_\x1b[75;37;11;0;8;1_",
		"ctrl+h": "\x1b[72;35;8;1;8;1_\x1b[72;35;8;0;8;1_",
		"Ctrl+L": "\x1b[76;38;12;1;8;1_\x1b[76;38;12;0;8;1_",
		"ctrl+a": "\x1b[65;30;1;1;8;1_\x1b[65;30;1;0;8;1_",
		"ctrl+z": "\x1b[90;44;26;1;8;1_\x1b[90;44;26;0;8;1_",
	} {
		if got, ok := win32InputSequence(key); !ok || got != want {
			t.Errorf("%s: got %q, %v; want %q", key, got, ok, want)
		}
	}
	for _, key := range []string{"h", "alt+h", "ctrl+alt+h", "ctrl+1", "ctrl+left", "ctrl+", ""} {
		if got, ok := win32InputSequence(key); ok {
			t.Errorf("%s: got %q, want no sequence", key, got)
		}
	}
}

func TestSendKeysWin32Input(t *testing.T) {
	c, logged := fakeCLIClient(t, "tt-git")
	c.win32Input = true
	if err := c.SendKeys("w1:p1", "ctrl+j"); err != nil {
		t.Fatal(err)
	}
	// Keys it can't encode go through send-keys as before.
	if err := c.SendKeys("w1:p1", "alt+j"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"pane send-text w1:p1 \x1b[74;36;10;1;8;1_\x1b[74;36;10;0;8;1_",
		"pane send-keys w1:p1 alt+j",
	}
	if got := logged(); !reflect.DeepEqual(got, want) {
		t.Fatalf("CLI calls:\n got  %q\n want %q", got, want)
	}
}
