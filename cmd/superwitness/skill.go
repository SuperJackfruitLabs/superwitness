package main

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed skill/SKILL.md
var skillTemplate string

// skillText is the skill with this build's version stamped in.
func skillText() string { return strings.ReplaceAll(skillTemplate, "{{VERSION}}", version) }

// skillCmd prints the skill, or with "install" writes it under a skills directory.
func skillCmd(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, skillText())
		return 0
	}
	if args[0] != "install" {
		fmt.Fprintln(stderr, "usage: superwitness skill [install [--dir DIR]]")
		return 2
	}
	fs := flag.NewFlagSet("skill install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "", "skills directory (default ~/.claude/skills)")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return 2
	}
	if *dir == "" {
		home := getenv("HOME")
		if home == "" {
			h, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintln(stderr, "skill install: no home directory; pass --dir")
				return 2
			}
			home = h
		}
		*dir = filepath.Join(home, ".claude", "skills")
	}
	path, err := installSkill(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "skill install:", err)
		return 1
	}
	fmt.Fprintln(stdout, path)
	return 0
}

// installSkill writes <dir>/superwitness-ops/SKILL.md through a staged file in the same
// directory and a rename, so a reader never sees half a file. It touches only that one file.
func installSkill(dir string) (string, error) {
	dest := filepath.Join(dir, "superwitness-ops")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dest, ".SKILL.md.incoming.*")
	if err != nil {
		return "", err
	}
	staged := f.Name()
	_, werr := f.WriteString(skillText())
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(staged, 0o644)
	}
	if werr == nil {
		werr = os.Rename(staged, filepath.Join(dest, "SKILL.md"))
	}
	if werr != nil {
		os.Remove(staged)
		return "", werr
	}
	return filepath.Join(dest, "SKILL.md"), nil
}
