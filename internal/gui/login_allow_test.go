package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Open at login is gone, so the row always reads as off, and System
// Settings' Login Items pane is no longer one the page may open: none of
// the System Settings panes in this list opens now.
func TestLoginNeedsAllowing(t *testing.T) {
	sandboxHome(t)
	if s := settingsState(); s.Login || s.LoginAllow {
		t.Errorf("login %v, loginAllow %v; want both false", s.Login, s.LoginAllow)
	}

	w := &importWindows{}
	mux := http.NewServeMux()
	providerRoutes(mux, w)
	for _, link := range []string{"x-apple.systempreferences:com.apple.LoginItems-Settings.extension", "x-apple.systempreferences:com.apple.preference.security?Privacy_FilesAndFolders", "x-apple.systempreferences:"} {
		w.opened = ""
		body, _ := json.Marshal(map[string]string{"url": link})
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/open", strings.NewReader(string(body))))
		if w.opened != "" {
			t.Errorf("%s: opened %q, want nothing opened", link, w.opened)
		}
	}
}
