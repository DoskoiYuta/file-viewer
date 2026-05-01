package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark/parser"
	"go.abhg.dev/goldmark/frontmatter"
)

type indexData struct {
	Root       string
	Extensions []string
	Port       int
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/view/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "index.html", indexData{
		Root:       s.cfg.Root,
		Extensions: s.cfg.Extensions,
		Port:       s.cfg.Port,
	}); err != nil {
		s.cfg.Logger.Printf("render index: %v", err)
	}
}

type treeNode struct {
	Name     string
	Path     string // relative, slash-separated
	IsDir    bool
	Children []*treeNode
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	root, err := s.buildTree()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "tree.html", root); err != nil {
		s.cfg.Logger.Printf("render tree: %v", err)
	}
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("file")
	if rel == "" {
		fmt.Fprint(w, `<div class="empty">Select a file</div>`)
		return
	}
	abs, err := s.safeJoin(rel)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "is directory", http.StatusBadRequest)
		return
	}
	if !s.pathAllowed(rel) {
		http.Error(w, "extension not allowed", http.StatusForbidden)
		return
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(rel)), ".")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	switch ext {
	case "md":
		s.renderMarkdown(w, abs, rel)
	case "pdf":
		fmt.Fprintf(w, `<div class="pdf-wrap"><embed src="/api/raw?file=%s" type="application/pdf" width="100%%" height="100%%"></div>`, template.URLQueryEscaper(filepath.ToSlash(rel)))
	case "png", "jpg", "jpeg", "gif", "webp", "svg":
		fmt.Fprintf(w, `<div class="img-wrap"><img src="/api/raw?file=%s" alt="%s"></div>`, template.URLQueryEscaper(filepath.ToSlash(rel)), template.HTMLEscapeString(rel))
	default:
		fmt.Fprintf(w, `<pre>unsupported: %s</pre>`, template.HTMLEscapeString(rel))
	}
}

var mermaidRe = regexp.MustCompile(`(?s)<pre><code class="language-mermaid">(.*?)</code></pre>`)

func (s *Server) renderMarkdown(w io.Writer, abs, rel string) {
	src, err := os.ReadFile(abs)
	if err != nil {
		fmt.Fprintf(w, `<pre>read error: %s</pre>`, template.HTMLEscapeString(err.Error()))
		return
	}
	pCtx := parser.NewContext()
	var buf bytes.Buffer
	if err := s.md.Convert(src, &buf, parser.WithContext(pCtx)); err != nil {
		fmt.Fprintf(w, `<pre>render error: %s</pre>`, template.HTMLEscapeString(err.Error()))
		return
	}
	fm := ""
	if data := frontmatter.Get(pCtx); data != nil {
		var meta map[string]any
		if err := data.Decode(&meta); err == nil {
			fm = renderFrontmatterTable(meta)
		}
	}
	out := mermaidRe.ReplaceAllString(buf.String(), `<pre class="mermaid">$1</pre>`)
	fmt.Fprintf(w, `<article class="markdown-body" data-path="%s">%s%s</article>`,
		template.HTMLEscapeString(rel), fm, out)
}

func renderFrontmatterTable(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(`<table class="frontmatter"><thead><tr><th colspan="2">front matter</th></tr></thead><tbody>`)
	for _, k := range keys {
		fmt.Fprintf(&sb, `<tr><th>%s</th><td>%s</td></tr>`,
			template.HTMLEscapeString(k), renderFMValue(meta[k]))
	}
	sb.WriteString(`</tbody></table>`)
	return sb.String()
}

func renderFMValue(v any) string {
	switch x := v.(type) {
	case nil:
		return `<span class="fm-null">null</span>`
	case string:
		return template.HTMLEscapeString(x)
	case bool:
		return fmt.Sprintf("%t", x)
	case []any:
		if len(x) == 0 {
			return `<span class="fm-null">[]</span>`
		}
		// All-scalar list → comma-separated; otherwise <ul>.
		allScalar := true
		for _, it := range x {
			switch it.(type) {
			case []any, map[string]any:
				allScalar = false
			}
			if !allScalar {
				break
			}
		}
		if allScalar {
			parts := make([]string, 0, len(x))
			for _, it := range x {
				parts = append(parts, renderFMValue(it))
			}
			return strings.Join(parts, ", ")
		}
		var sb strings.Builder
		sb.WriteString(`<ul class="fm-list">`)
		for _, it := range x {
			fmt.Fprintf(&sb, `<li>%s</li>`, renderFMValue(it))
		}
		sb.WriteString(`</ul>`)
		return sb.String()
	case map[string]any:
		if len(x) == 0 {
			return `<span class="fm-null">{}</span>`
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		sb.WriteString(`<table class="frontmatter sub"><tbody>`)
		for _, k := range keys {
			fmt.Fprintf(&sb, `<tr><th>%s</th><td>%s</td></tr>`,
				template.HTMLEscapeString(k), renderFMValue(x[k]))
		}
		sb.WriteString(`</tbody></table>`)
		return sb.String()
	default:
		return template.HTMLEscapeString(fmt.Sprintf("%v", x))
	}
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("file")
	if rel == "" {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	abs, err := s.safeJoin(rel)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if !s.pathAllowed(rel) {
		http.Error(w, "extension not allowed", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, abs)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	results := s.fuzzyFiles(q, 200)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "search.html", results); err != nil {
		s.cfg.Logger.Printf("render search: %v", err)
	}
}

func (s *Server) handleGrep(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	results := s.grepMarkdown(q, 100)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "grep.html", results); err != nil {
		s.cfg.Logger.Printf("render grep: %v", err)
	}
}
