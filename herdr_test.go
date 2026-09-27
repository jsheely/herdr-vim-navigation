package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// The test binary doubles as a fake `herdr` CLI (like the reference repo's
// mock-herdr.sh, but portable): with HERDR_NAV_FAKE_CLI=1 it logs its argv
// to HERDR_NAV_FAKE_LOG and answers with the CLI's JSON envelope.
func TestMain(m *testing.M) {
	if os.Getenv("HERDR_NAV_FAKE_CLI") == "1" {
		fakeCLI(os.Args[1:])
	}
	os.Exit(m.Run())
}

func fakeCLI(args []string) {
	if f, err := os.OpenFile(os.Getenv("HERDR_NAV_FAKE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(f, strings.Join(args, " "))
		f.Close()
	}
	joined := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(joined, "pane process-info"):
		fmt.Printf(`{"id":"cli:pane:process_info","result":{"type":"pane_process_info","process_info":`+
			`{"pane_id":"w1:p1","shell_pid":7,"foreground_processes":[{"pid":8,"name":%q}]}}}`+"\n",
			os.Getenv("HERDR_NAV_FAKE_PROCESS"))
	case strings.Contains(joined, "w1:missing"):
		fmt.Fprintln(os.Stderr, `{"id":"cli:pane:focus","error":{"code":"pane_not_found","message":"pane not found"}}`)
		os.Exit(1)
	case strings.HasPrefix(joined, "pane focus --direction right"):
		// The fake layout has nothing to the right.
		fmt.Println(noNeighborResult)
	default:
		fmt.Println(`{"id":"cli","result":{"type":"ok"}}`)
	}
	os.Exit(0)
}

// noNeighborResult is pane.focus_direction's reply at an edge, as captured
// from Herdr 0.9.1 (layout trimmed).
const noNeighborResult = `{"id":"cli:pane:focus","result":{"focus":{"changed":false,"focused_pane_id":"w1:p1",` +
	`"layout":{"panes":[],"splits":[],"zoomed":false},"reason":"no_neighbor","source_pane_id":"w1:p1"},` +
	`"type":"pane_focus_direction"}}`

// fakeCLIClient returns a client whose socket is unavailable, so every call
// goes through the fake CLI, plus a func returning the logged commands.
func fakeCLIClient(t *testing.T, process string) (*client, func() []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "herdr.log")
	t.Setenv("HERDR_NAV_FAKE_CLI", "1")
	t.Setenv("HERDR_NAV_FAKE_LOG", log)
	t.Setenv("HERDR_NAV_FAKE_PROCESS", process)
	c := &client{socketPath: filepath.Join(t.TempDir(), "missing.sock"), bin: exe}
	return c, func() []string {
		data, _ := os.ReadFile(log)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
}

func TestCLIFallback(t *testing.T) {
	c, logged := fakeCLIClient(t, "nvim")

	info, err := c.ProcessInfo("w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if info.ShellPID != 7 || len(info.Foreground) != 1 || info.Foreground[0].Name != "nvim" {
		t.Fatalf("unexpected process info %+v", info)
	}
	if res, err := c.FocusDirection("left", "w1:p1"); err != nil || res != (FocusResult{}) {
		t.Fatalf("focus left: %+v, %v", res, err)
	}
	if _, err := c.FocusDirection("up", ""); err != nil {
		t.Fatal(err)
	}
	res, err := c.FocusDirection("right", "w1:p1")
	if err != nil || res.Reason != reasonNoNeighbor || res.SourcePaneID != "w1:p1" {
		t.Fatalf("focus right at edge: %+v, %v", res, err)
	}
	if err := c.SendKeys("w1:p1", "ctrl+h"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"pane process-info --pane w1:p1",
		"pane focus --direction left --pane w1:p1",
		"pane focus --direction up --current",
		"pane focus --direction right --pane w1:p1",
		"pane send-keys w1:p1 ctrl+h",
	}
	if got := logged(); !reflect.DeepEqual(got, want) {
		t.Fatalf("CLI calls:\n got  %q\n want %q", got, want)
	}

	_, err = c.FocusDirection("left", "w1:missing")
	if err == nil || !strings.Contains(err.Error(), "pane_not_found") {
		t.Fatalf("expected pane_not_found error, got %v", err)
	}
}

// End to end through the real client: the reference repo's test.sh cases.
func TestNavigateThroughCLI(t *testing.T) {
	stubPlatform(t)
	for _, tc := range []struct {
		process string
		dir     Direction
		want    string
	}{
		{"nvim", "left", "pane send-keys w1:p1 ctrl+h"},
		{"zsh", "down", "pane focus --direction down --pane w1:p1"},
		{"zsh", "right", "pane send-keys w1:p1 ctrl+l"}, // no pane to the right
	} {
		c, logged := fakeCLIClient(t, tc.process)
		if err := navigate(defaultConfig(), c, tc.dir, "w1:p1"); err != nil {
			t.Fatal(err)
		}
		calls := logged()
		if got := calls[len(calls)-1]; got != tc.want {
			t.Errorf("%s %s: last call %q, want %q", tc.process, tc.dir, got, tc.want)
		}
	}
}

func TestSocket(t *testing.T) {
	var mu sync.Mutex
	var reqs []map[string]any
	sock := startFakeServer(t, func(line []byte) string {
		var req map[string]any
		if err := json.Unmarshal(line, &req); err != nil {
			return `{"id":"?","error":{"code":"invalid_request","message":"bad json"}}`
		}
		mu.Lock()
		reqs = append(reqs, req)
		mu.Unlock()
		id, _ := json.Marshal(req["id"])
		params, _ := req["params"].(map[string]any)
		switch {
		case req["method"] == "pane.process_info":
			return fmt.Sprintf(`{"id":%s,"result":{"type":"pane_process_info","process_info":`+
				`{"pane_id":"w1:p1","shell_pid":7,"foreground_processes":[{"pid":8,"name":"nvim"}]}}}`, id)
		case params["pane_id"] == "w1:missing":
			return fmt.Sprintf(`{"id":%s,"error":{"code":"pane_not_found","message":"pane not found"}}`, id)
		case params["direction"] == "right":
			return noNeighborResult
		}
		return fmt.Sprintf(`{"id":%s,"result":{"type":"ok"}}`, id)
	})

	// A bogus CLI proves nothing falls back to it while the socket works.
	c := &client{socketPath: sock, bin: filepath.Join(t.TempDir(), "no-such-herdr")}
	info, err := c.ProcessInfo("w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if info.ShellPID != 7 || info.Foreground[0].Name != "nvim" {
		t.Fatalf("unexpected process info %+v", info)
	}
	if _, err := c.FocusDirection("left", "w1:p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FocusDirection("up", ""); err != nil {
		t.Fatal(err)
	}
	if res, err := c.FocusDirection("right", "w1:p1"); err != nil || res.Reason != reasonNoNeighbor {
		t.Fatalf("focus right at edge: %+v, %v", res, err)
	}
	if err := c.SendKeys("w1:p1", "ctrl+h"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FocusDirection("left", "w1:missing"); err == nil || !strings.Contains(err.Error(), "pane_not_found") {
		t.Fatalf("expected pane_not_found error, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	got := make([]string, len(reqs))
	for i, r := range reqs {
		p, _ := json.Marshal(r["params"])
		got[i] = fmt.Sprintf("%s %s", r["method"], p)
	}
	want := []string{
		`pane.process_info {"pane_id":"w1:p1"}`,
		`pane.focus_direction {"direction":"left","pane_id":"w1:p1"}`,
		`pane.focus_direction {"direction":"up"}`,
		`pane.focus_direction {"direction":"right","pane_id":"w1:p1"}`,
		`pane.send_keys {"keys":["ctrl+h"],"pane_id":"w1:p1"}`,
		`pane.focus_direction {"direction":"left","pane_id":"w1:missing"}`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("socket requests:\n got  %q\n want %q", got, want)
	}
}

func TestDecodeEnvelope(t *testing.T) {
	var out struct {
		Type string `json:"type"`
	}
	if err := decodeEnvelope("x", []byte(`{"id":"1","result":{"type":"pong"}}`+"\n"), &out); err != nil || out.Type != "pong" {
		t.Fatalf("result: %v %+v", err, out)
	}
	if err := decodeEnvelope("x", []byte(`{"id":"1","error":{"code":"c","message":"m"}}`), nil); err == nil || err.Error() != "x: c: m" {
		t.Fatalf("error envelope: %v", err)
	}
	if err := decodeEnvelope("x", []byte(`{"id":"1"}`), &out); err == nil {
		t.Fatal("missing result should fail when a result is expected")
	}
	if err := decodeEnvelope("x", []byte(`not json`), nil); err == nil {
		t.Fatal("garbage should fail")
	}
}
