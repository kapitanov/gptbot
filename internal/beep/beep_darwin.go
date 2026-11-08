//go:build darwin

package beep

import "os/exec"

func Beep() {
	exec.Command("afplay", "/System/Library/Sounds/Ping.aiff").Run()
}
