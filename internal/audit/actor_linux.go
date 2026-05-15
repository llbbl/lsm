//go:build linux

package audit

import (
	"fmt"
	"os"
	"strings"
)

// lookupParentComm reads /proc/<ppid>/comm on Linux. Returns "" if ppid is not
// a plausible PID or if the file cannot be read.
func lookupParentComm(ppid int) string {
	if ppid <= 0 {
		return ""
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", ppid))
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\n")
}

// resolveTTYDevice returns the resolved device path for stdin via
// /proc/self/fd/0, or "" if it cannot be resolved.
func resolveTTYDevice() string {
	dev, err := os.Readlink("/proc/self/fd/0")
	if err != nil {
		return ""
	}
	return dev
}
