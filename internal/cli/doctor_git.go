package cli

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Git adds vendor suffixes to its numeric version, including Apple Git and
// Git for Windows.  The minimum follows rev-parse --path-format (Git 2.31).
var gitVersionPattern = regexp.MustCompile(`^git version ([0-9]+)\.([0-9]+)\.([0-9]+)([. +()-][^\r\n]*)?$`)

func (r *doctorReport) checkGit() {
	const advice = "Install a maintained Git package with the latest security fixes, version 2.31.0 or later. Git is needed for repository reviews and `critty tmp`; standalone documents and attachment-only reviews can run without it."
	bin, err := lookPath("git")
	if err != nil {
		r.add("git/version", "warn", "Git is not available on PATH", advice)
		return
	}
	out, err := doctorOutput(bin, "--version")
	if err != nil {
		r.add("git/version", "warn", "could not run Git at "+bin, advice)
		return
	}
	version := strings.TrimSpace(string(out))
	match := gitVersionPattern.FindStringSubmatch(version)
	if match == nil {
		r.add("git/version", "warn", "could not determine the Git version at "+bin, advice)
		return
	}
	var numbers [3]int
	for i := range numbers {
		numbers[i], err = strconv.Atoi(match[i+1])
		if err != nil {
			r.add("git/version", "warn", "could not determine the Git version at "+bin, advice)
			return
		}
	}
	status := "ok"
	message := fmt.Sprintf("%s at %s; minimum 2.31.0", version, bin)
	if slices.Compare(numbers[:], []int{2, 31, 0}) < 0 {
		status = "warn"
		message += "; too old for all Git workflows"
	} else if strings.HasPrefix(match[4], ".rc") || strings.HasPrefix(match[4], "-rc") {
		status = "warn"
		message += "; release candidate, use a released version"
	}
	if status == "ok" {
		r.add("git/version", status, message, "This is a feature-version check, not a security audit. Prefer a maintained package with current security fixes.")
		return
	}
	r.add("git/version", status, message, advice)
}
