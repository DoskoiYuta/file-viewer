package daemon

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func pidPath() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "file-viewer.pid")
	}
	return filepath.Join(os.TempDir(), "file-viewer.pid")
}

// WritePID stores "<pid> <port>" to the pid file.
func WritePID(port int) error {
	p := pidPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("%d %d", os.Getpid(), port)
	return os.WriteFile(p, []byte(content), 0o644)
}

// RemovePID best-effort removes the pid file.
func RemovePID() {
	_ = os.Remove(pidPath())
}

// readPID returns (pid, port, exists).
func readPID() (int, int, bool) {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return 0, 0, false
	}
	parts := strings.Fields(strings.TrimSpace(string(b)))
	if len(parts) == 0 {
		return 0, 0, false
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid <= 0 {
		return 0, 0, false
	}
	port := 0
	if len(parts) > 1 {
		port, _ = strconv.Atoi(parts[1])
	}
	return pid, port, true
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// Stop terminates the running server, waits for it to exit, and removes the pid file.
func Stop() error {
	pid, _, ok := readPID()
	if !ok {
		return errors.New("no running file-viewer server")
	}
	if !processAlive(pid) {
		RemovePID()
		return errors.New("no running file-viewer server (stale pid)")
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			RemovePID()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	RemovePID()
	return nil
}

// StopExisting stops any prior running server. Returns nil if none.
func StopExisting(logger *log.Logger) error {
	pid, _, ok := readPID()
	if !ok {
		return nil
	}
	if !processAlive(pid) {
		RemovePID()
		return nil
	}
	if logger != nil {
		logger.Printf("overwriting existing server pid=%d", pid)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			RemovePID()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	RemovePID()
	return nil
}
