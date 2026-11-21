package sysutil

import (
	"os"
	"strconv"
)

// Respawn starts this program again with the arguments it was started
// with and extra (NAME=value) added to its environment, detached. The new
// process waits for this one to exit, as a relaunch does (AwaitPredecessor).
func Respawn(extra ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	env := append(os.Environ(), "MAGPIE_REPLACES="+strconv.Itoa(os.Getpid()))
	_, _, err = launch(exe, os.Args[1:], append(env, extra...))
	return err
}
