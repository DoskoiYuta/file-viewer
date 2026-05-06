package daemon

import (
	"encoding/json"
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

// State is the on-disk record of a running file-viewer server.
type State struct {
	PID        int       `json:"pid"`
	Port       int       `json:"port"`
	Root       string    `json:"root"`
	Extensions []string  `json:"extensions"`
	StartedAt  time.Time `json:"started_at"`
}

func pidPath() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "file-viewer.pid")
	}
	return filepath.Join(os.TempDir(), "file-viewer.pid")
}

// WriteState persists the current server state to the pid file as JSON.
func WriteState(s State) error {
	p := pidPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if s.PID == 0 {
		s.PID = os.Getpid()
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = time.Now()
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// RemovePID best-effort removes the pid file.
func RemovePID() {
	_ = os.Remove(pidPath())
}

// ReadState returns the persisted state and whether a record exists.
// It transparently handles the legacy "<pid> <port>" format.
func ReadState() (State, bool) {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return State{}, false
	}
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" {
		return State{}, false
	}
	if trimmed[0] == '{' {
		var s State
		if err := json.Unmarshal([]byte(trimmed), &s); err != nil || s.PID <= 0 {
			return State{}, false
		}
		return s, true
	}
	// Legacy "<pid> <port>" format.
	parts := strings.Fields(trimmed)
	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid <= 0 {
		return State{}, false
	}
	port := 0
	if len(parts) > 1 {
		port, _ = strconv.Atoi(parts[1])
	}
	return State{PID: pid, Port: port}, true
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// IsAlive reports whether the process referenced by the state is still running.
func IsAlive(s State) bool {
	return s.PID > 0 && processAlive(s.PID)
}

// Stop terminates the running server, waits for it to exit, and removes the pid file.
func Stop() error {
	s, ok := ReadState()
	if !ok {
		return errors.New("no running file-viewer server")
	}
	if !processAlive(s.PID) {
		RemovePID()
		return errors.New("no running file-viewer server (stale pid)")
	}
	if err := syscall.Kill(s.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(s.PID) {
			RemovePID()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(s.PID, syscall.SIGKILL)
	RemovePID()
	return nil
}

// StopExisting stops any prior running server. Returns nil if none.
func StopExisting(logger *log.Logger) error {
	s, ok := ReadState()
	if !ok {
		return nil
	}
	if !processAlive(s.PID) {
		RemovePID()
		return nil
	}
	if logger != nil {
		logger.Printf("overwriting existing server pid=%d", s.PID)
	}
	if err := syscall.Kill(s.PID, syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(s.PID) {
			RemovePID()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(s.PID, syscall.SIGKILL)
	RemovePID()
	return nil
}
