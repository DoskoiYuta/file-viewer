package server

// defaultIgnoreDirs are directory base names that the watcher, tree builder,
// and search routines skip by default. They tend to contain huge numbers of
// files (build outputs, dependency caches) that exhaust fsnotify's per-process
// fd budget and aren't useful to browse.
var defaultIgnoreDirs = []string{
	".git",
	".hg",
	".svn",
	".idea",
	".vscode",
	".cache",
	".turbo",
	".gradle",
	".next",
	".nuxt",
	".svelte-kit",
	".parcel-cache",
	".pnpm-store",
	".yarn",
	".venv",
	"venv",
	"__pycache__",
	"node_modules",
	"bower_components",
	"vendor",
	"dist",
	"build",
	"out",
	"target",
}

// ignoreSet is the resolved lookup for directory base names to skip.
type ignoreSet map[string]struct{}

func newIgnoreSet(extra []string) ignoreSet {
	s := make(ignoreSet, len(defaultIgnoreDirs)+len(extra))
	for _, n := range defaultIgnoreDirs {
		s[n] = struct{}{}
	}
	for _, n := range extra {
		if n != "" {
			s[n] = struct{}{}
		}
	}
	return s
}

func (s ignoreSet) skip(name string) bool {
	_, ok := s[name]
	return ok
}
