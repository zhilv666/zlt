package app

import (
	"os/exec"
	"runtime"
	"strings"
)

// dashboardURL builds the browser URL for the control panel served at addr,
// falling back to the default listen address when addr is unknown (e.g. read
// from an older pid file that did not record it).
func dashboardURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = defaultHTTPAddr
	}
	return "http://" + addr
}

// openBrowser best-effort opens url in the user's default browser. Failures are
// intentionally ignored: this is a convenience, never a hard requirement.
func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}
