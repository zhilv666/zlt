package process

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func readUnixProcessCommand(pid int) (string, []string, bool) {
	data, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", nil, false
	}
	return parseDarwinProcArgs(data)
}

func unixProcessMatchesWorkingDir(pid int, expected string) bool {
	info, err := os.Stat(expected)
	if err != nil || !info.IsDir() {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	// Compare directory identity, not lsof's escaped display name. This also
	// recognizes /var and /private/var as the same directory, without cgo.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-FfDi0").Output()
	if err != nil {
		return false
	}
	device, inode, ok := parseDarwinCWDIdentity(out)
	return ok && device == uint64(uint32(stat.Dev)) && inode == stat.Ino
}
