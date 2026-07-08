package env

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW; not exported by the syscall package.
const createNoWindow = 0x08000000

// hideConsole stops docker.exe from allocating a visible console window when
// spawned by a console-less parent (zeroclawd runs detached, so without this
// every docker call pops a foreground terminal).
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
