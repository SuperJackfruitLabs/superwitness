package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every verb the switch dispatches must be named in the skill, so adding a verb without
// teaching the skill fails here.
func TestSkillMentionsEveryVerb(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	sw := string(src)[strings.Index(string(src), "switch cmd {"):]
	sw = sw[:strings.Index(sw, "default:")]
	var verbs []string
	for _, m := range regexp.MustCompile(`case ((?:"[^"]+",? ?)+):`).FindAllStringSubmatch(sw, -1) {
		for _, v := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(m[1], -1) {
			if !strings.HasPrefix(v[1], "-") { // flag spellings of version
				verbs = append(verbs, v[1])
			}
		}
	}
	if len(verbs) < 6 {
		t.Fatalf("parsed too few verbs from main.go: %v", verbs)
	}
	text := skillText()
	for _, v := range verbs {
		if !strings.Contains(text, "superwitness "+v) && !strings.Contains(text, "`"+v+"`") {
			t.Errorf("SKILL.md does not mention verb %q", v)
		}
	}
	if !strings.Contains(text, "Written for superwitness "+version) {
		t.Error("version not stamped")
	}
	if strings.Contains(text, "{{") {
		t.Error("unreplaced placeholder")
	}
}

func TestSkillPrintsAndInstalls(t *testing.T) {
	var out, errb strings.Builder
	if code := skillCmd(nil, func(string) string { return "" }, &out, &errb); code != 0 || out.String() != skillText() {
		t.Fatalf("skill print: code=%d", code)
	}
	dir := t.TempDir()
	for i := 0; i < 2; i++ { // second run proves idempotence
		out.Reset()
		if code := skillCmd([]string{"install", "--dir", dir}, func(string) string { return "" }, &out, &errb); code != 0 {
			t.Fatalf("install: %d %s", code, errb.String())
		}
		want := filepath.Join(dir, "superwitness-ops", "SKILL.md")
		if strings.TrimSpace(out.String()) != want {
			t.Fatalf("printed %q, want %q", out.String(), want)
		}
		b, err := os.ReadFile(want)
		if err != nil || string(b) != skillText() {
			t.Fatalf("installed file differs from `skill` output: %v", err)
		}
	}
	ents, _ := os.ReadDir(filepath.Join(dir, "superwitness-ops"))
	if len(ents) != 1 || ents[0].Name() != "SKILL.md" {
		t.Fatalf("staging file left behind: %v", ents)
	}
}

func TestSkillInstallDefaultsToHome(t *testing.T) {
	home := t.TempDir()
	var out, errb strings.Builder
	if code := skillCmd([]string{"install"}, func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "superwitness-ops", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
