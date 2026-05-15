//go:build darwin

package audit

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// lookupParentComm shells out to `ps -o comm= -p <ppid>` on macOS. There is no
// /proc on Darwin, and `ps` is universally present. A 1-second timeout guards
// against the (extremely unlikely) case of ps hanging.
//
// Returns "" if ppid is not plausible, ps exits non-zero, or output is empty.
func lookupParentComm(ppid int) string {
	if ppid <= 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "comm=", "-p", strconv.Itoa(ppid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// resolveTTYDevice returns "" on macOS: there is no /proc/self/fd/0 and
// resolving the controlling terminal portably is not worth a new dependency.
// CaptureActor falls back to the "tty" sentinel when stdin is a character
// device, which is all downstream needs to answer "is this interactive?".
func resolveTTYDevice() string {
	return ""
}
