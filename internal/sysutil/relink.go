package sysutil

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/yetone/magpie/internal/appdir"
)

// CopiedCLI is the command's path when it's a plain copied file rather than
// install.sh's link, or "" when it's absent, a link, or not a regular file.
// A bare Lstat — nothing is run, nothing is written — so the app's window
// can ask on every launch. It can't tell a stale copy from a current one
// (that needs the copy's version, which only `magpie update` may read), so
// what the window says off it is advice with a dismiss, not a warning that
// knows. Only on the Mac; on Linux the app and the command are one file.
func CopiedCLI() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	bin := appdir.Getenv("MAGPIE_BIN_DIR")
	if bin == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return ""
		}
		bin = filepath.Join(home, ".local", "bin")
	}
	cli := filepath.Join(bin, "magpie")
	st, err := os.Lstat(cli)
	if err != nil || !st.Mode().IsRegular() {
		return "" // absent, the installer's link, or nothing to point at
	}
	return cli
}
