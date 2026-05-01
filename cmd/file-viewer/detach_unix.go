//go:build unix

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// detachStdio replaces fds 0/1/2 so the daemonized child no longer
// holds inherited pipes/ttys from the parent. fd 0 and 1 are pointed
// at /dev/null; fd 2 is pointed at logPath (or /dev/null if empty).
func detachStdio(logPath string) error {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()

	stderrFD := int(null.Fd())
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			defer f.Close()
			stderrFD = int(f.Fd())
		}
	}

	nullFD := int(null.Fd())
	if err := unix.Dup2(nullFD, 0); err != nil {
		return err
	}
	if err := unix.Dup2(nullFD, 1); err != nil {
		return err
	}
	if err := unix.Dup2(stderrFD, 2); err != nil {
		return err
	}
	return nil
}
