package gateway

import (
	"bytes"
	"encoding/json"
	"log"
	"runtime"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/settings"
)

// A Claude run is Claude Code's interactive mode on macOS, Linux and
// Windows, for this machine's Claude Code, unless the user turned it off
// or the run needs what only -p takes.
func TestClaudeTUIChosen(t *testing.T) {
	was := settings.Load()
	t.Cleanup(func() { _ = settings.Save(was) })
	claudeTUIRuns = true
	t.Cleanup(func() { claudeTUIRuns = false })
	here := claudeCLI{path: "/usr/local/bin/claude"}
	text := func(s string) []Message {
		return []Message{{Role: "user", Parts: []Part{{Kind: Text, Text: s}}}}
	}
	on := runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows"
	cases := []struct {
		name   string
		req    *Request
		binary claudeCLI
		print  bool
		want   bool
		why    string // what is logged, on macOS, Linux and Windows
	}{
		{"a plain turn", &Request{Messages: text("hi")}, here, false, on, ""},
		{"turned off", &Request{Messages: text("hi")}, here, true, false, "the interactive mode is turned off"},
		{"safeguards", &Request{Messages: text("hi"), Safeguards: json.RawMessage(`{"a":1}`)}, here, false, on, ""},
		{"client cache marks", &Request{Messages: text("hi"), clientCache: &clientCache{}}, here, false, on, ""},
		{"a schema", &Request{Messages: text("hi"), Format: &Format{Type: "json_schema", Schema: json.RawMessage(`{"type":"object"}`)}}, here, false, on, ""},
		{"an image", &Request{Messages: []Message{{Role: "user", Parts: []Part{{Kind: Image, MediaType: "image/png", Data: "AA=="}}}}}, here, false, on, ""},
		{"an image by its URL", &Request{Messages: []Message{{Role: "user", Parts: []Part{{Kind: Image, URL: "https://example.com/a.png"}}}}}, here, false, false, "an image known by its URL"},
		{"a tool's image by its URL", &Request{Messages: []Message{{Role: "user", Parts: []Part{{Kind: ToolResult, Images: []Part{{Kind: Image, URL: "https://example.com/a.png"}}}}}}}, here, false, false, "an image known by its URL"},
		{"a long first message", &Request{Messages: text(strings.Repeat("the codebase as read so far. ", 40000))}, here, false, on, ""},
	}
	for _, c := range cases {
		st := settings.Load()
		st.ClaudePrintMode = c.print
		if err := settings.Save(st); err != nil {
			t.Fatal(err)
		}
		if got, _ := claudeTUI(c.req, c.binary); got != c.want {
			t.Errorf("%s: claudeTUI = %v, want %v", c.name, got, c.want)
		}
		if got := claudePrintWhy(c.req, c.binary); on && got != c.why {
			t.Errorf("%s: -p because %q, want %q", c.name, got, c.why)
		}
	}
}

// The helper's line saying why it became -p is logged; the rest of Claude
// Code's stderr isn't.
func TestClaudeTUIFallbackLogged(t *testing.T) {
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	w := &lockedWriter{run: &subscriptionRun{}}
	_, _ = w.Write([]byte("warming up\nclaude-tui: Claude Code ended before it showed its prompt (exit status 1): B; this run is -p\n"))
	_, _ = w.Write([]byte("Error: something else\n"))
	got := logs.String()
	if !strings.Contains(got, "claude: Claude Code ended before it showed its prompt (exit status 1): B; this run is -p") || strings.Contains(got, "warming up") || strings.Contains(got, "something else") {
		t.Fatalf("logged %q", got)
	}
	if !strings.Contains(w.run.stderr.String(), "something else") {
		t.Fatalf("stderr kept %q", w.run.stderr.String())
	}
}

// The provider page's codes for why every run is -p: the setting, none
// found; "" for the TUI.
func TestClaudeMachineWhy(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("no interactive mode on " + runtime.GOOS)
	}
	was := settings.Load()
	t.Cleanup(func() { _ = settings.Save(was) })
	claudeTUIRuns = true
	t.Cleanup(func() { claudeTUIRuns = false })
	here := claudeCLI{path: "/usr/local/bin/claude"}
	for _, c := range []struct {
		name   string
		binary claudeCLI
		print  bool
		code   string
	}{
		{"here", here, false, ""},
		{"turned off", here, true, "off"},
		{"none", claudeCLI{}, false, "missing"},
	} {
		st := settings.Load()
		st.ClaudePrintMode = c.print
		if err := settings.Save(st); err != nil {
			t.Fatal(err)
		}
		if code, why := claudeMachineWhy(c.binary); code != c.code || (code == "") != (why == "") {
			t.Errorf("%s: %q %q, want code %q", c.name, code, why, c.code)
		}
	}
	claudeTUIRuns = false
	if code, _ := claudeMachineWhy(here); code != "no-shim" {
		t.Errorf("no shim: %q", code)
	}
}
