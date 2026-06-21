//go:build windows

package audit

// lookupParentComm returns "" on Windows. There is no /proc and no
// universally-cheap way to resolve a parent process's image name without extra
// syscalls (CreateToolhelp32Snapshot) or a third-party dependency. The Actor
// snapshot is best-effort, so an empty ParentComm here is an acceptable
// degradation rather than a hard failure.
func lookupParentComm(ppid int) string {
	return ""
}

// resolveTTYDevice returns "" on Windows: there is no /proc/self/fd/0 and
// resolving the console device portably is not worth a new dependency.
// CaptureActor falls back to the "tty" sentinel when stdin is a character
// device, which is all downstream needs to answer "is this interactive?".
func resolveTTYDevice() string {
	return ""
}
