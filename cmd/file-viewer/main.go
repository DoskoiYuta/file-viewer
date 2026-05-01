package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/doskoiyuta/file-viewer/internal/daemon"
	"github.com/doskoiyuta/file-viewer/internal/server"
)

const (
	defaultPort = 6275
	defaultExt  = "pdf,md,png,jpg,svg"
	envChild    = "FILE_VIEWER_CHILD"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "file-viewer:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) >= 1 && args[0] == "down" {
		return daemon.Stop()
	}

	fs := flag.NewFlagSet("file-viewer", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: file-viewer [options] [path]
       file-viewer down

Options:
  -d        run in background; prints URL to stdout, logs to stderr
  -e EXTS   comma-separated extensions (default %q)
  -p PORT   listen port (default %d)
`, defaultExt, defaultPort)
	}
	background := fs.Bool("d", false, "run in background")
	extStr := fs.String("e", defaultExt, "extensions")
	port := fs.Int("p", defaultPort, "port")
	if err := fs.Parse(args); err != nil {
		return err
	}

	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("stat path: %w", err)
	}
	rootDir := abs
	openFile := ""
	if !info.IsDir() {
		rootDir = filepath.Dir(abs)
		openFile = filepath.Base(abs)
	}

	exts := normalizeExts(*extStr)
	if len(exts) == 0 {
		return errors.New("no extensions configured")
	}

	if os.Getenv(envChild) == "1" {
		return runForeground(rootDir, openFile, exts, *port, true)
	}

	if *background {
		return spawnBackground(target, exts, *port)
	}

	return runForeground(rootDir, openFile, exts, *port, false)
}

func normalizeExts(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(p), "."))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func runForeground(rootDir, openFile string, exts []string, port int, fromDaemon bool) error {
	logger := log.New(os.Stderr, "", log.LstdFlags)

	if err := daemon.StopExisting(logger); err != nil {
		logger.Printf("stop existing: %v", err)
	}

	srv, err := server.New(server.Config{
		Root:       rootDir,
		Extensions: exts,
		Port:       port,
		Logger:     logger,
	})
	if err != nil {
		return fmt.Errorf("init server: %w", err)
	}

	if err := daemon.WritePID(port); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}
	defer daemon.RemovePID()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run(ctx) }()

	logger.Printf("file-viewer listening on http://localhost:%d  root=%s", port, rootDir)
	if fromDaemon {
		// Signal readiness: parent watches stdout (a single line).
		fmt.Fprintln(os.Stdout, buildURL(port, rootDir, openFile))
	}

	select {
	case <-ctx.Done():
		logger.Printf("shutdown signal")
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	shutdownCtx, c2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer c2()
	return srv.Shutdown(shutdownCtx)
}

func spawnBackground(targetArg string, exts []string, port int) error {
	if err := daemon.StopExisting(log.New(os.Stderr, "", log.LstdFlags)); err != nil {
		fmt.Fprintln(os.Stderr, "stop existing:", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	childArgs := []string{
		"-e", strings.Join(exts, ","),
		"-p", fmt.Sprintf("%d", port),
		targetArg,
	}

	cmd := exec.Command(exe, childArgs...)
	cmd.Env = append(os.Environ(), envChild+"=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr // child logs go to parent's stderr until parent exits
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return err
	}

	// Read first line from child stdout = the URL
	urlCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 0, 256)
		tmp := make([]byte, 256)
		for {
			n, err := stdout.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				if i := strings.IndexByte(string(buf), '\n'); i >= 0 {
					urlCh <- strings.TrimSpace(string(buf[:i]))
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					urlCh <- ""
				}
				return
			}
		}
	}()

	var serverURL string
	select {
	case u := <-urlCh:
		if u == "" {
			_ = cmd.Process.Kill()
			return errors.New("background server exited before reporting URL")
		}
		serverURL = u
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		return errors.New("background server failed to start in time")
	}

	// Detach: don't wait, redirect remaining stderr to /dev/null
	_ = cmd.Process.Release()
	fmt.Println(serverURL)
	return nil
}

func buildURL(port int, root, openFile string) string {
	base := fmt.Sprintf("http://localhost:%d/", port)
	if openFile == "" {
		return base
	}
	rel, err := filepath.Rel(root, filepath.Join(root, openFile))
	if err != nil {
		rel = openFile
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return base + "view/" + strings.Join(parts, "/")
}
