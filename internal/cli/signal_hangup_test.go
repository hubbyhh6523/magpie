//go:build !windows && !nogui

package cli

import (
	"syscall"
	"testing"
	"time"
)

// The terminal closing (SIGHUP) is left to magpie by Wails
// (ownSignals keeps it), so magpie's app drains the gateway it
// serves, its idle Claude sessions saved, before magpie dies of it: it
// died first, and the next magpie had no session to resume (#7017).
func TestHangupDrainsTheGatewayFirst(t *testing.T) {
	oldGUI, oldApp, oldPanel := guiStop, openApp, openPanel
	t.Cleanup(func() { guiStop, openApp, openPanel = oldGUI, oldApp, oldPanel })
	logs := startLog
	startLog = func(string) {}
	t.Cleanup(func() { startLog = logs })

	// run is the command as it runs, once it has taken SIGINT and SIGTERM
	// back
	for name, open := range map[string]func(run func() error) error{
		"app": func(run func() error) error {
			openApp = func(bool, string) error { ownSignals(); return run() }
			return runGUI(false, "")
		},
		"panel": func(run func() error) error {
			openPanel = func() error { ownSignals(); return run() }
			return runPanel()
		},
	} {
		t.Run(name, func(t *testing.T) {
			died := deaths(t, syscall.SIGHUP)
			stopped := make(chan struct{})
			stop := func() {
				select {
				case got := <-died:
					t.Errorf("died of %v before the gateway drained", got)
				default:
				}
				close(stopped)
			}
			guiStop = stop
			endProbesOnSignal()
			err := open(func() error {
				send(t, syscall.SIGHUP)
				select {
				case <-stopped:
				case <-time.After(5 * time.Second):
					t.Fatal("SIGHUP never drained the gateway")
				}
				diesOf(t, died, syscall.SIGHUP)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
