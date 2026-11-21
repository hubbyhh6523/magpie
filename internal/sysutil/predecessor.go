package sysutil

import (
	"os"
	"strconv"
)

// AwaitPredecessor blocks, for a while at most, until the magpie that
// relaunched this one has exited; on Windows its WebView2 too (awaitGone).
func AwaitPredecessor() {
	reportStarted() // first: the one that started this waits for it
	pid, err := strconv.Atoi(os.Getenv("MAGPIE_REPLACES"))
	os.Unsetenv("MAGPIE_REPLACES")
	if err != nil || pid <= 0 {
		return
	}
	awaitGone(pid)
}
