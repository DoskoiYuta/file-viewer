package server

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Server) buildTree() (*treeNode, error) {
	root := &treeNode{Name: filepath.Base(s.cfg.Root), Path: "", IsDir: true}
	dirs := map[string]*treeNode{"": root}

	err := filepath.WalkDir(s.cfg.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == s.cfg.Root {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(s.cfg.Root, p)
		rel = filepath.ToSlash(rel)
		parent := filepath.ToSlash(filepath.Dir(rel))
		if parent == "." {
			parent = ""
		}
		pNode, ok := dirs[parent]
		if !ok {
			return nil
		}
		node := &treeNode{Name: name, Path: rel, IsDir: d.IsDir()}
		if d.IsDir() {
			dirs[rel] = node
			pNode.Children = append(pNode.Children, node)
		} else {
			if !s.pathAllowed(rel) {
				return nil
			}
			pNode.Children = append(pNode.Children, node)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	pruneEmpty(root)
	sortTree(root)
	return root, nil
}

// pruneEmpty removes directories that contain no allowed files (recursively).
func pruneEmpty(n *treeNode) bool {
	if !n.IsDir {
		return true
	}
	kept := n.Children[:0]
	for _, c := range n.Children {
		if pruneEmpty(c) {
			kept = append(kept, c)
		}
	}
	n.Children = kept
	return len(n.Children) > 0 || n.Path == ""
}

func sortTree(n *treeNode) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	for _, c := range n.Children {
		if c.IsDir {
			sortTree(c)
		}
	}
}
