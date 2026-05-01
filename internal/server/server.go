package server

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/frontmatter"
)

//go:embed all:web
var webFS embed.FS

type Config struct {
	Root       string
	Extensions []string // lowercase, no dot
	Port       int
	Logger     *log.Logger
}

type Server struct {
	cfg     Config
	extSet  map[string]bool
	tmpl    *template.Template
	md      goldmark.Markdown
	hub     *sseHub
	w       *watcher
	httpSrv *http.Server
	ln      net.Listener
}

func New(cfg Config) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	if !filepath.IsAbs(cfg.Root) {
		abs, err := filepath.Abs(cfg.Root)
		if err != nil {
			return nil, err
		}
		cfg.Root = abs
	}
	extSet := map[string]bool{}
	for _, e := range cfg.Extensions {
		extSet[strings.ToLower(e)] = true
	}

	tmplFS, err := fs.Sub(webFS, "web/templates")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"join":    strings.Join,
		"urlPath": urlPath,
	}).ParseFS(tmplFS, "*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.DefinitionList,
			extension.Typographer,
			&frontmatter.Extender{},
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	s := &Server{
		cfg:    cfg,
		extSet: extSet,
		tmpl:   tmpl,
		md:     md,
		hub:    newSSEHub(),
	}
	w, err := newWatcher(cfg.Root, cfg.Logger, s.hub)
	if err != nil {
		return nil, err
	}
	s.w = w

	mux := http.NewServeMux()
	staticFS, err := fs.Sub(webFS, "web/static")
	if err != nil {
		return nil, err
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/view/", s.handleIndex)
	mux.HandleFunc("/api/tree", s.handleTree)
	mux.HandleFunc("/api/view", s.handleView)
	mux.HandleFunc("/api/raw", s.handleRaw)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/grep", s.handleGrep)
	mux.HandleFunc("/api/events", s.hub.serveSSE)

	addr := fmt.Sprintf(":%d", cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	s.ln = ln
	s.httpSrv = &http.Server{Handler: mux}
	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	go s.w.run(ctx)
	errCh := make(chan error, 1)
	go func() {
		err := s.httpSrv.Serve(s.ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.hub.close()
	if s.w != nil {
		s.w.close()
	}
	return s.httpSrv.Shutdown(ctx)
}

// pathAllowed reports whether the file extension is one we render.
func (s *Server) pathAllowed(rel string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(rel)), ".")
	return s.extSet[ext]
}

// urlPath escapes each segment of a slash-separated relative path for use in a URL.
func urlPath(p string) string {
	parts := strings.Split(p, "/")
	for i, x := range parts {
		parts[i] = url.PathEscape(x)
	}
	return strings.Join(parts, "/")
}

// safeJoin resolves a relative path against the root, rejecting traversal.
func (s *Server) safeJoin(rel string) (string, error) {
	rel = filepath.FromSlash(rel)
	abs := filepath.Join(s.cfg.Root, rel)
	clean := filepath.Clean(abs)
	if clean != s.cfg.Root && !strings.HasPrefix(clean, s.cfg.Root+string(filepath.Separator)) {
		return "", errors.New("path escapes root")
	}
	return clean, nil
}
