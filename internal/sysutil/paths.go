package sysutil

import (
	"os"
	"path/filepath"
	"runtime"
)

// Bundle is the .app the running binary lives in, or "" when it is not in
// one.
func Bundle() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	exe, err := Executable()
	if err != nil {
		return ""
	}
	// …/magpie.app/Contents/MacOS/magpie
	app := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if filepath.Ext(app) != ".app" || filepath.Base(filepath.Dir(exe)) != "MacOS" {
		return ""
	}
	return app
}

// Executable is the running binary, symlinks resolved. When they can't be
// (Go's EvalSymlinks fails on the current junction Scoop runs an app from,
// "The system cannot find the path specified"), it is the path the binary
// was started from, which still names it.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return resolved(exe), nil
}

// resolved is path with its symlinks resolved, or path when that fails.
func resolved(path string) string {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return p
	}
	return path
}

// Writable reports whether magpie may replace what lives in dir.
func Writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".magpie-update-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// Alive reports whether the process pid is still running.
func Alive(pid int) bool { return alive(pid) }
