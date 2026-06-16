package oauth

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openInBrowser asks the OS to open url in the user's default browser. It
// returns once the launcher process has been spawned; whether the browser
// actually displayed the page is opaque from here.
func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// `rundll32 url.dll,FileProtocolHandler` is the long-standing
		// "open the default browser" incantation that works without a
		// shell.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux", "freebsd", "openbsd", "netbsd":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("don't know how to open a browser on %s; visit %s manually", runtime.GOOS, url)
	}
	return cmd.Start()
}
