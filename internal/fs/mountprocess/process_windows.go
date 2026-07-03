//go:build windows

package mountprocess

import (
	"os"
	"time"
)

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	return err == nil && process != nil
}

func Terminate(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func Kill(pid int) error {
	return Terminate(pid)
}

func WaitExit(pid int, timeout time.Duration) bool {
	if timeout <= 0 {
		return !Alive(pid)
	}
	deadline := time.Now().Add(timeout)
	for {
		if !Alive(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}
