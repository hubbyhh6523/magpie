package sysutil

// Binary staging helpers for the first install: swapping a staged copy in
// for the installed one on Windows (where a running .exe can't be
// overwritten, only moved aside), and clearing what earlier installs left
// beside it. Kept from the removed self-updater, which swapped in new
// versions the same way.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yetone/magpie/internal/appdir"
)

// besideStage is the hidden folder beside the program an update is
// downloaded into when no rename from the cache reaches the program.
const besideStage = ".magpie-update"

// stageCache is magpie's cache folder; tests set it.
var stageCache = appdir.SystemCache

// cacheStage is exe's own folder in the cache for its downloads, "" with
// no cache: two copies of magpie (a portable one and the installed one)
// never share one.
func cacheStage(exe string) string {
	cache, err := stageCache()
	if err != nil || cache == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(exe))))
	return filepath.Join(cache, "update", hex.EncodeToString(sum[:6]))
}

// dropStage removes the folder a staged download was in once it is empty,
// when it is one binaryStage made.
func dropStage(staged string) {
	d := filepath.Dir(staged)
	if filepath.Base(d) == besideStage || (len(filepath.Base(d)) == 12 && filepath.Base(filepath.Dir(d)) == "update") {
		os.Remove(d)
	}
}

// stagedFiles are the places a download of exe's next build may be: where
// binaryStage puts it, under its name or one of freshStage's beside it,
// and beside exe as exe.new, where earlier versions put it.
func stagedFiles(exe string) []string {
	name := filepath.Base(exe) + ".new"
	out := []string{exe + ".new"}
	dirs := []string{filepath.Join(filepath.Dir(exe), besideStage)}
	if d := cacheStage(exe); d != "" {
		dirs = append(dirs, d)
	}
	for _, d := range dirs {
		out = append(out, filepath.Join(d, name))
		es, _ := os.ReadDir(d)
		for _, e := range es {
			if strings.HasPrefix(e.Name(), name+"-") && e.Type().IsRegular() {
				out = append(out, filepath.Join(d, e.Name()))
			}
		}
	}
	return out
}

// InstallBinaryAside swaps a staged binary in for exe, saying where the
// running exe was moved to on Windows ("" elsewhere), for the caller to
// put it back should the staged one not start. A staged copy this
// computer won't start (Smart App Control, #894) isn't put in: a
// *BlockedError, and the staged copy removed.
func InstallBinaryAside(staged, exe string) (string, error) {
	var old string
	if runtime.GOOS == "windows" {
		// tried once per staged copy: a restart clicked again after a failed
		// one doesn't start it again, which would have an antivirus that
		// checks a program as it starts hold it again for the move below
		if !trialled.passed(staged) {
			if err := tryStart(staged); heldOpen(err) {
				// held, not refused: kept, for a restart to try again
				// once whatever holds it lets go
				return "", &saidError{"couldn't start the new version to check it: another program still has the download open (often an antivirus checking it); restart to update again in a minute, or let magpie through it", err}
			} else if err != nil {
				os.Remove(staged)
				dropStage(staged)
				return "", &BlockedError{Err: err}
			}
			trialled.note(staged)
		}
		// A running .exe cannot be overwritten, but it can be moved aside.
		// What the last update moved aside may still be running too (a
		// `magpie serve` started before it), and can't be removed or
		// replaced then: this one goes beside it, under a name of its own.
		// The download is kept, for another try.
		RemoveOld(exe)
		var err error
		if old, err = moveAside(exe); err != nil {
			return "", err
		}
	}
	if err := moveIn(staged, exe); err != nil {
		if heldOpen(err) {
			// the reason and what to do, shown as is in the version row
			// and the Update pill's tooltip, without the paths
			err = &saidError{fmt.Sprintf("couldn't put the new version in place of %s: another program still has the download open (often an antivirus checking it): let magpie through it and restart to update again, or download the new version and put it in place of this one", filepath.Base(exe)), err}
		}
		if old != "" {
			if restoreErr := moveIn(old, exe); restoreErr != nil {
				return "", errors.Join(err, fmt.Errorf("couldn't restore %s from %s: %w", filepath.Base(exe), old, restoreErr))
			}
		} else if !NeedsAdmin(err) { // kept for InstallBinaryAsAdmin
			os.Remove(staged)
			dropStage(staged)
		}
		return "", err
	}
	dropStage(staged)
	return old, nil
}

// saidError is err said in other words: what is shown, err what
// errors.Is and errors.As find.
type saidError struct {
	said string
	err  error
}

// renameFile is os.Rename; tests make it fail.
var renameFile = os.Rename

// asideWaits are the pauses between tries at moving the running exe aside:
// an antivirus scanning it or a sync client (OneDrive) reading it holds it
// open, which Windows won't rename under, for a moment, and lets go.
var asideWaits = []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 1500 * time.Millisecond}

// heldFor is how long a file Windows says another program holds open is
// waited for, past asideWaits, the last pause repeated: an antivirus that
// checks a new program as it starts may hold a 50 MB download longer than
// asideWaits' three seconds, and a restart that gave up then would fail
// the same way on every click, each click's trial start having it checked
// again (白菜 on Discord). The user clicked restart and sees "Restarting…"
// meanwhile. A var for tests.
var heldFor = 30 * time.Second

// waitHeld is whether to try again after try i (from 0) failed with err,
// begun at start, sleeping first when so: through asideWaits for any error
// retry allows, then on while err is a file held open, up to heldFor.
func waitHeld(i int, err error, start time.Time, retry bool) bool {
	if !retry {
		return false
	}
	if i < len(asideWaits) {
		time.Sleep(asideWaits[i])
		return true
	}
	if !heldOpen(err) || len(asideWaits) == 0 {
		return false
	}
	pause := asideWaits[len(asideWaits)-1]
	if time.Since(start)+pause > heldFor {
		return false
	}
	time.Sleep(pause)
	return true
}

// tryStart is canStart, tried again while another program holds the
// download open, as moveIn waits for it: a scanner that checks a new
// program holds its file from the moment it is written, and Windows then
// won't start it ("being used by another process"), which is no refusal
// of the program (mycf_711 on Discord).
func tryStart(staged string) error {
	start := time.Now()
	for i := 0; ; i++ {
		err := canStart(staged)
		if err == nil || !waitHeld(i, err, start, heldOpen(err)) {
			return err
		}
	}
}

// trialled is the download canStart last let through, by path, size and
// time written, so a restart tried again on it doesn't start it again.
var trialled trialNote

type trialNote struct {
	mu   sync.Mutex
	path string
	size int64
	mod  time.Time
}

// Windows' errors for a file another program holds open without letting
// it be renamed.
const (
	errSharingViolation syscall.Errno = 32
	errLockViolation    syscall.Errno = 33
)

// moveIn moves from to exe, trying again for a few seconds while
// something holds from open, as moveAside does for the running exe. On
// Windows that is the download just after canStart's trial: an antivirus
// that checks a program as its process starts can hold its file a moment
// after the process has ended, and a move made at once failed the first
// install of every update (#2272). Any other failure is returned at once.
func moveIn(from, exe string) error {
	start := time.Now()
	for i := 0; ; i++ {
		err := os.Rename(from, exe)
		if err == nil || !waitHeld(i, err, start, heldOpen(err)) {
			return err
		}
	}
}

// heldOpen is whether err is Windows refusing a file another program has
// open.
func heldOpen(err error) bool {
	return errors.Is(err, errSharingViolation) || errors.Is(err, errLockViolation)
}

// moveAside moves the running exe out of the way of the new version, under
// a name of its own, trying again for a few seconds while something has it
// open. What it says when it can't is the reason and what to do: the error
// is shown as is in the version row. On success it returns the actual name,
// so a failed installation can put the running exe back.
func moveAside(exe string) (string, error) {
	var err error
	start := time.Now()
	for i := 0; ; i++ {
		old := oldName(exe)
		if err = renameFile(exe, old); err == nil {
			return old, nil
		}
		if i == 0 {
			os.Chmod(exe, 0o755) // a read-only exe: the attribute off
		}
		if !waitHeld(i, err, start, !errors.Is(err, fs.ErrNotExist)) {
			break
		}
	}
	why := err.Error()
	var le *os.LinkError
	if errors.As(err, &le) {
		why = le.Err.Error() // the paths are known; the reason is what's new
	}
	var hint string
	switch {
	case heldOpen(err):
		hint = "another program has it open (often an antivirus or OneDrive): let magpie through it and restart to update again, or download the new version and put it in place of this one"
	case errors.Is(err, fs.ErrPermission):
		hint = "Windows doesn't let magpie change files in " + filepath.Dir(exe) + " (an antivirus' folder protection, or the folder's permissions): let magpie through, or download the new version and put it in place of this one"
	default:
		hint = "download the new version and put it in place of this one"
	}
	return "", fmt.Errorf("couldn't move %s aside to put the new version in: %s; %s", filepath.Base(exe), strings.TrimRight(why, ". 。"), hint)
}

// oldName is where a running exe is moved aside to: exe.old, or when
// that is still there (in use), exe.old-2, exe.old-3…
func oldName(exe string) string {
	name := exe + ".old"
	for n := 2; ; n++ {
		if _, err := os.Lstat(name); os.IsNotExist(err) {
			return name
		}
		name = fmt.Sprintf("%s.old-%d", exe, n)
	}
}

// RemoveOld removes what earlier updates moved aside of exe, those no
// longer running.
func RemoveOld(exe string) {
	os.Remove(exe + ".old")
	olds, _ := filepath.Glob(exe + ".old-*")
	for _, o := range olds {
		os.Remove(o)
	}
}

// removeStaged removes every download of exe's next build: what nothing
// will put in.
func removeStaged(exe string) {
	for _, staged := range stagedFiles(exe) {
		os.Remove(staged)
		dropStage(staged)
	}
}

func fileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// canStart is whether the program at path can be started here: Windows
// starts it suspended and ends it at once, so none of it runs; elsewhere
// nothing refuses a build that downloaded whole. A var for tests.
var canStart = trialStart

func (n *trialNote) passed(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.path == path && n.size == fi.Size() && n.mod.Equal(fi.ModTime())
}

func (n *trialNote) note(path string) {
	fi, err := os.Stat(path)
	n.mu.Lock()
	defer n.mu.Unlock()
	if err != nil {
		n.path = ""
		return
	}
	n.path, n.size, n.mod = path, fi.Size(), fi.ModTime()
}

func (e *saidError) Error() string { return e.said }
func (e *saidError) Unwrap() error { return e.err }
