package browser

import (
	"errors"
	"os/exec"
	"runtime"
)

// Open asks the OS to open the given path or URL with the default handler.
func Open(pathOrURL string) error {
	if pathOrURL == "" {
		return errors.New("open: empty path")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", pathOrURL)
	case "windows":
		// start requires a window title argument; empty string is fine.
		cmd = exec.Command("cmd", "/c", "start", "", pathOrURL)
	default:
		cmd = exec.Command("xdg-open", pathOrURL)
	}
	return cmd.Start()
}
