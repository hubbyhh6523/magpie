package sysutil

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/yetone/magpie/internal/proc"
)

// startWait is how long a restarted magpie has to say it is up.
var startWait = 20 * time.Second

// startedEnv names the file a relaunched magpie writes once it runs
// (AwaitPredecessor), for the one that started it to know.
const startedEnv = "MAGPIE_STARTED"

// launch starts a program with args and env, detached from this one; the
// channel is closed when it exits, and kill ends it. A var for tests.
var launch = func(exe string, args, env []string) (<-chan struct{}, func(), error) {
	cmd := proc.Command(exe, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	exited := make(chan struct{})
	go func() { cmd.Wait(); close(exited) }()
	return exited, func() { cmd.Process.Kill() }, nil
}

// awaitStarted waits for marker to be written, the started program's
// exit (exited closing) or startWait, whichever comes first.
func awaitStarted(marker string, exited <-chan struct{}, kill func()) error {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(startWait)
	for {
		if _, err := os.Stat(marker); err == nil {
			return nil
		}
		select {
		case <-exited:
			if _, err := os.Stat(marker); err == nil {
				return nil // it said so and then went on as another
			}
			return errors.New("it quit as soon as it started")
		case <-deadline:
			kill()
			// its file let go of, to be put back over
			select {
			case <-exited:
			case <-time.After(5 * time.Second):
			}
			return fmt.Errorf("it didn't start within %s", startWait)
		case <-tick.C:
		}
	}
}

// reportStarted writes the file the magpie that started this one waits
// for, once.
func reportStarted() {
	p := os.Getenv(startedEnv)
	if p == "" {
		return
	}
	os.Unsetenv(startedEnv)
	os.WriteFile(p, []byte(strconv.Itoa(os.Getpid())), 0o644)
}
