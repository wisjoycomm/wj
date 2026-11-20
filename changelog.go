package main

import (
	_ "embed"
	"strconv"
	"strings"
)

//go:embed CHANGELOG.md
var changelogText string

// changesSince returns the changelog sections newer than version and not newer than
// upTo, newest first. An empty version means the project has no record: nothing.
func changesSince(text, version, upTo string) string {
	if strings.TrimSpace(version) == "" {
		return ""
	}
	var out []string
	keep := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if head, ok := strings.CutPrefix(line, "## "); ok {
			v := strings.Fields(head)[0]
			keep = versionLess(version, v) && !versionLess(upTo, v)
		}
		if keep {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// versionLess compares dotted numbers: 0.2.9 < 0.2.10.
func versionLess(a, b string) bool {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}
