package sysutil

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/lazyre"
	"github.com/yetone/magpie/internal/proc"
)

// Relaunch opens bundle again once this process (pid) has exited: with its
// window when window is set, else only its menu bar icon (`tray`, as a
// login launch), which a restart by itself uses while no window was up.
// Each try, and what it came to, is written to relaunch.log in magpie's
// own folder. quiet, the restart is one magpie made by itself: the app
// opens in the background (open -g), as it does too with no window, so it
// takes no focus from the app the user is in (relaunchOpen).
func Relaunch(bundle string, window, quiet bool) error {
	return runAfterExit(os.Getpid(), relaunchCommand(bundle, window, quiet, relaunchLog()))
}

// openWaits are the pauses, in seconds, between tries of the relaunch's
// open: about 13s in all. Just after magpie exits, LaunchServices can
// still hold it as running and send the open to the process that is gone,
// and open fails (-600, procNotFound; seen with the screen locked). Tried
// once, nothing started magpie again until the user did, nine hours later.
var openWaits = []string{"0.5", "1", "1", "2", "2", "3", "4"}

// relaunchLogName is the file in magpie's folder the relaunch writes each
// try of its open to, and the start it falls back to: the shell that runs
// them has no output anyone sees, and a magpie that never came back left
// only the system log to say why.
const relaunchLogName = "relaunch.log"

// relaunchLog is the relaunch's log file, made ready to append to; once it
// is past 64KB the last one is kept as relaunch.log.old. "" when magpie's
// folder can't be made: the tries then go unwritten.
func relaunchLog() string {
	h, err := appdir.Home()
	if err != nil || h == "" {
		return ""
	}
	dir := appdir.Config()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	p := filepath.Join(dir, relaunchLogName)
	if fi, err := os.Stat(p); err == nil && fi.Size() > 64<<10 {
		os.Rename(p, p+".old")
	}
	return p
}

// relaunchCommand is the shell command Relaunch runs once magpie has
// exited: open, tried again while it fails, and, when it never worked,
// the bundle's own executable run straight, as the login launch agent
// runs it. With the screen locked, magpie stayed closed after an update
// even with the 13s of tries (molvqingtai, after fb23bca5c): LaunchServices
// can go on refusing the open for as long as the session is locked, and a
// program run by its path needs nothing from it to start.
func relaunchCommand(bundle string, window, quiet bool, log string) string {
	return relaunchScript(bundle, window, quiet, log, openWaits)
}

// relaunchScript is relaunchCommand with the pauses between opens.
func relaunchScript(bundle string, window, quiet bool, log string, waits []string) string {
	if log == "" {
		log = os.DevNull
	}
	exe := filepath.Join(bundle, "Contents", "MacOS", bundleExecutable(bundle))
	run := shellQuote(exe)
	for _, a := range RelaunchArgs(window, "") {
		run += " " + shellQuote(a)
	}
	var b strings.Builder
	b.WriteString("log=" + shellQuote(log) + "; ")
	b.WriteString(`note() { echo "$(date '+%Y-%m-%d %H:%M:%S %z') $*" >> "$log" 2>/dev/null; }; `)
	// open's own exit status and what it said, on one line
	b.WriteString(`try() { n=$((n+1)); e=$(` + relaunchOpen(bundle, window, quiet) + ` 2>&1 >/dev/null); s=$?; note "open try $n: exit $s $(printf '%s' "$e" | tr '\n' ' ')"; return $s; }; `)
	b.WriteString("n=0; note " + shellQuote("magpie has exited; opening "+bundle) + "; ")
	b.WriteString(retried("try", waits))
	b.WriteString(" || { note \"open gave up after $n tries; starting \"" + shellQuote(run) + "; ")
	b.WriteString("[ -x " + shellQuote(exe) + " ] || { note \"no executable at \"" + shellQuote(exe) + "; exit 1; }; ")
	b.WriteString("exec " + run + " </dev/null >/dev/null 2>&1; }")
	return b.String()
}

// bundleExecutableKey finds the program an app's Info.plist names.
var bundleExecutableKey = lazyre.New(`<key>CFBundleExecutable</key>\s*<string>([^<]+)</string>`)

// bundleExecutable is the name of the program in bundle's Contents/MacOS:
// the one its Info.plist names, else this one's own when it runs from
// that bundle, else magpie.
func bundleExecutable(bundle string) string {
	if b, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist")); err == nil {
		if m := bundleExecutableKey.FindSubmatch(b); m != nil {
			if name := strings.TrimSpace(html.UnescapeString(string(m[1]))); name != "" && !strings.ContainsRune(name, '/') {
				return name
			}
		}
	}
	if exe, err := Executable(); err == nil && filepath.Dir(exe) == filepath.Join(bundle, "Contents", "MacOS") {
		return filepath.Base(exe)
	}
	return "magpie"
}

// retried is a shell command that runs command until it succeeds, sleeping
// each of waits in turn after a failure; the last try's status is its own.
func retried(command string, waits []string) string {
	if len(waits) == 0 {
		return command
	}
	return fmt.Sprintf("for w in %s; do %s && exit 0; sleep $w; done; %s", strings.Join(waits, " "), command, command)
}

// runAfterExit starts a shell that runs command once pid has exited. The
// shell is in a session of its own: launchd ends the process group of a
// job whose process exits, so one left in magpie's group was killed as
// magpie quit and the new version never opened. That is the launch agent
// of Open at login without AbandonProcessGroup (ee16c657), and, on macOS
// 13 at least, the job LaunchServices runs an app in when it is opened from
// the Dock or Finder, which magpie can't set that key on (#2362).
func runAfterExit(pid int, command string) error {
	script := fmt.Sprintf(`while kill -0 %d 2>/dev/null; do sleep 0.2; done; %s`, pid, command)
	cmd := proc.Command("/bin/sh", "-c", script)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detach(cmd)
	return proc.StartReaped(cmd)
}

// relaunchOpen is the open command Relaunch runs for bundle. -n asks
// LaunchServices for a new instance: without it, open looks the app up
// among those running first, and just after magpie exits LaunchServices
// can still have the gone process registered as magpie. open then sends
// it a reopen Apple event (aevt/rapp) rather than launching the bundle,
// and fails with -600, procNotFound ("Specified target process … does
// not exist", "LAUNCH: Launch failed in CSUI"; Linka's log on Discord,
// macOS 27.0.1, v0.1.1308), as often as it is tried. A magpie that is in
// fact up still has the one instance: the new process finds the instance
// lock held and hands over to it (runningAlready in internal/gui).
//
// -g opens it in the background, unless it comes back with its window on
// a restart the user asked for. Without it, LaunchServices brings magpie
// to the front as it starts: a restart into an update with only the menu
// bar icon to come back, or one magpie made by itself (Settings'
// AutoRestart), took the focus from the app the user was typing in
// (xyxyxy_15591 on Discord).
func relaunchOpen(bundle string, window, quiet bool) string {
	return openCommand(bundle, window, quiet)
}

// openCommand is the open of relaunchOpen and openBeside.
func openCommand(bundle string, window, quiet bool) string {
	g := ""
	if quiet || !window {
		g = "-g "
	}
	if window {
		return fmt.Sprintf("open -n %s%q", g, bundle)
	}
	return fmt.Sprintf("open -n %s%q --args tray", g, bundle)
}

// RelaunchArgs is what a relaunch starts magpie with: `gui [view]` for
// the window, `tray` for the tray icon alone.
func RelaunchArgs(window bool, view string) []string {
	switch {
	case !window:
		return []string{"tray"}
	case view != "":
		return []string{"gui", view}
	}
	return []string{"gui"}
}

// OpenBeside opens bundle now, a new instance beside this one (open -n),
// tried again while open fails: a restart that hands the gateway over
// (internal/handoff) starts the program before this one quits. With
// window, its window; else its menu bar icon only, as Relaunch, in the
// background as Relaunch's when quiet or with no window.
func OpenBeside(bundle string, window, quiet bool) error {
	cmd := proc.Command("/bin/sh", "-c", retried(openBeside(bundle, window, quiet), openWaits))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detach(cmd)
	return proc.StartReaped(cmd)
}

// openBeside is the open command OpenBeside runs for bundle.
func openBeside(bundle string, window, quiet bool) string {
	return openCommand(bundle, window, quiet)
}

// StartBeside starts exe now, beside this process, as RelaunchArgs say: a
// restart that hands the gateway over. It isn't told to wait for this one
// (MAGPIE_REPLACES): it takes the gateway from it while it runs.
func StartBeside(exe string, window bool, view string) error {
	_, _, err := launch(exe, RelaunchArgs(window, view), os.Environ())
	return err
}

// RelaunchBinary starts exe beside this process, as RelaunchArgs say, with
// MAGPIE_REPLACES set so the new one waits for this one to exit. old was
// the binary this one replaced, put back when the new one wouldn't start;
// with no self-update there is never one, and it is ignored.
func RelaunchBinary(exe, old string, window bool, view string) error {
	_ = old
	args := RelaunchArgs(window, view)
	env := append(os.Environ(), "MAGPIE_REPLACES="+strconv.Itoa(os.Getpid()))
	_, _, err := launch(exe, args, env)
	return err
}
