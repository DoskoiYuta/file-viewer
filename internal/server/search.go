package server

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type fileHit struct {
	Path  string
	Score int
}

type grepHit struct {
	Path string
	Line int
	Text string
}

// fuzzyFiles returns files whose relative path matches q in order of subsequence chars.
func (s *Server) fuzzyFiles(q string, limit int) []fileHit {
	q = strings.ToLower(q)
	var hits []fileHit
	_ = filepath.WalkDir(s.cfg.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(s.cfg.Root, p)
		rel = filepath.ToSlash(rel)
		if !s.pathAllowed(rel) {
			return nil
		}
		if q == "" {
			hits = append(hits, fileHit{Path: rel})
			return nil
		}
		score, ok := fuzzyScore(strings.ToLower(rel), q)
		if !ok {
			return nil
		}
		hits = append(hits, fileHit{Path: rel, Score: score})
		return nil
	})
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Path < hits[j].Path
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// fuzzyScore: subsequence match scoring; higher is better. Returns (score, matched).
func fuzzyScore(text, query string) (int, bool) {
	if query == "" {
		return 0, true
	}
	if strings.Contains(text, query) {
		// substring bonus
		return 1000 - strings.Index(text, query), true
	}
	ti, qi := 0, 0
	score := 0
	prevMatch := false
	for ti < len(text) && qi < len(query) {
		if text[ti] == query[qi] {
			score += 5
			if prevMatch {
				score += 10
			}
			if ti == 0 || text[ti-1] == '/' || text[ti-1] == '-' || text[ti-1] == '_' || text[ti-1] == '.' {
				score += 8
			}
			qi++
			prevMatch = true
		} else {
			prevMatch = false
		}
		ti++
	}
	if qi < len(query) {
		return 0, false
	}
	return score, true
}

// grepMarkdown searches contents of .md files for the query (case-insensitive substring).
func (s *Server) grepMarkdown(q string, limit int) []grepHit {
	if q == "" {
		return nil
	}
	needle := strings.ToLower(q)
	var hits []grepHit
	_ = filepath.WalkDir(s.cfg.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".md" {
			return nil
		}
		if !s.pathAllowed(d.Name()) {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		rel, _ := filepath.Rel(s.cfg.Root, p)
		rel = filepath.ToSlash(rel)
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		lineNum := 0
		for sc.Scan() {
			lineNum++
			line := sc.Text()
			if strings.Contains(strings.ToLower(line), needle) {
				hits = append(hits, grepHit{Path: rel, Line: lineNum, Text: trimSnippet(line, 200)})
				if len(hits) >= limit {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	return hits
}

func trimSnippet(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
