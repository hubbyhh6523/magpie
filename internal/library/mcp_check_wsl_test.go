package library

import (
	"context"
	"slices"
	"testing"
)

// A server only this machine's agents have is checked here as before,
// WSL never asked; so is a Windows program given to agents in WSL, which
// WSL is gone for: everything is checked here now.
func TestCheckWindowsServerHere(t *testing.T) {
	s := fakeStdio(t, "ok", nil)
	s.Agents = []string{"claude", "codex"}
	h := CheckServer(context.Background(), s)
	if h.State != "ok" || h.Tools != 3 || h.Checked != nil {
		t.Fatalf("got %+v, want ok here with nothing named", h)
	}
	for _, c := range []string{`D:\tools\Magpie\magpie-windows-amd64.exe`, `C:/x/npx.cmd`} {
		if got := checkSides(&Server{Transport: "stdio", Command: c, Agents: []string{"claude@wsl:Ubuntu"}}); !slices.Equal(got, []string{""}) {
			t.Errorf("%s: sides %q, want this machine", c, got)
		}
	}
}
