//go:build !windows

package cli

import (
	"os/exec"
	"runtime"
)

func openBrowser(url string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, url).Start()
}
