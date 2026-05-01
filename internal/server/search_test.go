package server

import "testing"

func TestFuzzyScore(t *testing.T) {
	tests := []struct {
		text, query string
		match       bool
	}{
		{"docs/readme.md", "rdm", true},
		{"docs/readme.md", "readme", true},
		{"docs/readme.md", "xyz", false},
		{"a/b/c.md", "abc", true},
		{"", "x", false},
	}
	for _, tc := range tests {
		_, ok := fuzzyScore(tc.text, tc.query)
		if ok != tc.match {
			t.Errorf("fuzzyScore(%q, %q) match=%v want %v", tc.text, tc.query, ok, tc.match)
		}
	}

	// Substring should beat scattered subsequence.
	subSc, _ := fuzzyScore("readme.md", "readme")
	scaSc, _ := fuzzyScore("real-name-mid.md", "readme")
	if subSc <= scaSc {
		t.Errorf("expected substring match to outrank scattered: sub=%d sca=%d", subSc, scaSc)
	}
}
