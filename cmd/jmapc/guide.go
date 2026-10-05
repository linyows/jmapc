package main

import (
	"bytes"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// guide is what a coding agent reads before writing a request: the format,
// the steps, and the mistakes validate refuses. It is built into the command so
// that it describes the jmapc that prints it, whichever version that is.
//
//go:embed guide.md
var guide string

// guideUsage describes the guide command.
const guideUsage = `jmapc guide prints how to write jmapc requests, for a coding agent to read
before adding or changing one.

Usage:
	jmapc guide [topic]
	jmapc guide -install [-for claude|codex] [dir]

Topics: files, workflow, syntax, mistakes, examples, reference. Without one,
the whole guide is printed.

-install writes a skill to dir (default: the current directory) that has the
agent run "jmapc guide" before writing a request and "jmapc validate" after:
.claude/skills/jmapc/SKILL.md for Claude Code and .agents/skills/jmapc/SKILL.md
for Codex. -for writes only one of them. The skill holds no more than that, so
it stays right when jmapc is upgraded.
`

// skillMarker identifies a skill file jmapc wrote, which -install may replace.
// A file without it was written by someone else, and is left alone.
const skillMarker = "<!-- Written by jmapc guide -install. Run it again to update. -->"

// skill is the file -install writes. It points at the guide rather than
// repeating it, since a copy of the guide would describe the jmapc that wrote
// it rather than the one the project has now.
const skill = `---
name: jmapc
description: Write or change jmapc request files (*.jmap.json) and properties.json, from which jmapc generates a typed JMAP client. Use when adding or editing a JMAP request in a project with a jmapc.json or a directory of .jmap.json files.
---

` + skillMarker + `

Before writing or changing a request file, run ` + "`jmapc guide`" + ` and follow it. It
describes the installed version of jmapc. In a Go module that lists jmapc as a
tool, run ` + "`go tool jmapc guide`" + ` instead.

After changing a request, run ` + "`jmapc validate`" + ` and fix what it reports until it
passes, then run ` + "`jmapc generate`" + `. Never edit a generated file.
`

// skillDirs are where each agent looks for a skill in a repository.
var skillDirs = map[string]string{
	"claude": filepath.Join(".claude", "skills", "jmapc"),
	"codex":  filepath.Join(".agents", "skills", "jmapc"),
}

// printGuide prints the guide, or one section of it, or installs the skill
// that points an agent at it.
func printGuide(args []string) error {
	fs := flag.NewFlagSet("jmapc guide", flag.ContinueOnError)
	var (
		install = fs.Bool("install", false, "write the skill that points an agent at this guide")
		agent   = fs.String("for", "", "write the skill for one agent only: claude or codex")
	)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, guideUsage) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("guide takes one argument, not %d", fs.NArg())
	}

	if *install {
		dir := "."
		if fs.NArg() == 1 {
			dir = fs.Arg(0)
		}
		return installSkill(dir, *agent)
	}
	if *agent != "" {
		return errors.New("-for is for -install")
	}
	if fs.NArg() == 0 {
		_, err := fmt.Fprint(stdout, guide)
		return err
	}
	section, err := guideSection(fs.Arg(0))
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(stdout, section)
	return err
}

// guideTopics returns the topics the guide has, in order: each second-level
// heading, lower-cased.
func guideTopics() []string {
	var topics []string
	for _, line := range strings.Split(guide, "\n") {
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			topics = append(topics, strings.ToLower(heading))
		}
	}
	return topics
}

// guideSection returns the section of the guide under the heading the topic
// names, up to the next heading of the same level.
func guideSection(topic string) (string, error) {
	lines := strings.SplitAfter(guide, "\n")
	start := -1
	for i, line := range lines {
		heading, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "## ")
		if !ok {
			continue
		}
		if start >= 0 {
			return strings.TrimRight(strings.Join(lines[start:i], ""), "\n") + "\n", nil
		}
		if strings.EqualFold(heading, topic) {
			start = i
		}
	}
	if start >= 0 {
		return strings.Join(lines[start:], ""), nil
	}
	return "", fmt.Errorf("the guide has no topic %q; the topics are %s", topic, strings.Join(guideTopics(), ", "))
}

// installSkill writes the skill under dir for the agent named, or for every
// agent where none is.
func installSkill(dir, agent string) error {
	agents := []string{"claude", "codex"}
	if agent != "" {
		if _, ok := skillDirs[agent]; !ok {
			return fmt.Errorf("-for is claude or codex, not %q", agent)
		}
		agents = []string{agent}
	}
	// Every file is checked before any is written, so that a refusal leaves
	// nothing half installed.
	paths := make([]string, len(agents))
	for i, a := range agents {
		paths[i] = filepath.Join(dir, skillDirs[a], "SKILL.md")
		existing, err := os.ReadFile(paths[i])
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !bytes.Contains(existing, []byte(skillMarker)) {
			return fmt.Errorf("%s was not written by jmapc guide -install, so it is left alone; move it to install the skill there", paths[i])
		}
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(skill), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(stdout, path)
	}
	return nil
}
