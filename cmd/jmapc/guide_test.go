package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/request"
)

// guideBlock is a JSON example in the guide: a file to write, or a mistake.
type guideBlock struct {
	// file is the name the example is written under, from "file=" in its
	// info string, and is empty for a mistake.
	file string
	// wrong marks a mistake, which validate has to refuse.
	wrong bool
	body  string
}

var fence = regexp.MustCompile("(?ms)^```json([^\\n]*)\\n(.*?)^```$")

// guideBlocks returns every JSON example the guide holds, failing on one that
// is neither a file nor a mistake, since nothing would check it.
func guideBlocks(t *testing.T) []guideBlock {
	t.Helper()
	var blocks []guideBlock
	for _, m := range fence.FindAllStringSubmatch(guide, -1) {
		info := strings.Fields(m[1])
		b := guideBlock{body: m[2]}
		for _, field := range info {
			switch {
			case field == "wrong":
				b.wrong = true
			case strings.HasPrefix(field, "file="):
				b.file = strings.TrimPrefix(field, "file=")
			}
		}
		if b.wrong == (b.file != "") {
			t.Fatalf("a JSON example in the guide is to be marked either wrong or file=, not %q:\n%s", m[1], b.body)
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// TestGuideExamplesGenerate checks that the examples the guide calls right are:
// together they validate and generate, as the guide says they do.
func TestGuideExamplesGenerate(t *testing.T) {
	files := map[string]string{}
	for _, b := range guideBlocks(t) {
		if b.file != "" {
			files["requests/"+b.file] = b.body
		}
	}
	if len(files) < 4 {
		t.Fatalf("the guide has %d examples, want at least the four it describes", len(files))
	}
	dir := workspace(t, files)
	requests := filepath.Join(dir, "requests")
	if _, errOut, err := capture(t, []string{"validate", "-requests", requests}); err != nil {
		t.Fatalf("the guide's examples do not validate: %v\n%s", err, errOut)
	}
	for _, lang := range []string{"go", "typescript", "rust"} {
		out := filepath.Join(dir, lang, "client")
		if _, errOut, err := capture(t, []string{"generate", "-requests", requests, "-out", out, "-lang", lang}); err != nil {
			t.Errorf("the guide's examples do not generate %s: %v\n%s", lang, err, errOut)
		}
	}
}

// TestGuideMistakesAreRefused checks each mistake the guide shows against
// validate, which the guide says refuses it. One that validate takes is either
// no longer a mistake or not the mistake the guide describes.
func TestGuideMistakesAreRefused(t *testing.T) {
	var properties string
	var mistakes []string
	for _, b := range guideBlocks(t) {
		switch {
		case b.file == "properties.json":
			properties = b.body
		case b.wrong:
			mistakes = append(mistakes, b.body)
		}
	}
	if len(mistakes) == 0 {
		t.Fatal("the guide shows no mistakes")
	}
	for _, body := range mistakes {
		// The sets the examples declare are there, so that a mistake is
		// refused for what it shows rather than for a set it names.
		dir := workspace(t, map[string]string{
			"requests/properties.json":   properties,
			"requests/Mistake.jmap.json": body,
		})
		if _, _, err := capture(t, []string{"validate", "-requests", filepath.Join(dir, "requests")}); err == nil {
			t.Errorf("validate takes what the guide calls a mistake:\n%s", body)
		}
	}
}

// TestGuideNamesEveryMember checks that each member jmapc reads in a request
// file is in the guide, so that one added later is not left out of it.
func TestGuideNamesEveryMember(t *testing.T) {
	for _, member := range []string{
		request.DocMember, request.ReturnsMember, request.WatchesMember, request.PagesMember,
		request.CreatedIDsMember, request.SchemaMember, request.CommentArgument,
	} {
		if !strings.Contains(guide, "`"+member+"`") {
			t.Errorf("the guide does not describe %s", member)
		}
	}
}

// TestGuideNamesEveryCommand checks that the commands the usage lists are in
// the guide, so that a renamed one is not described under its old name.
func TestGuideNamesEveryCommand(t *testing.T) {
	commands := regexp.MustCompile(`(?m)^\tjmapc (\w+)`).FindAllStringSubmatch(usage, -1)
	if len(commands) == 0 {
		t.Fatal("found no commands in the usage")
	}
	for _, c := range commands {
		// Printing the version has nothing to do with writing a request.
		if c[1] == "version" {
			continue
		}
		if !strings.Contains(guide, "jmapc "+c[1]) {
			t.Errorf("the guide does not mention jmapc %s", c[1])
		}
	}
}

// TestGuideTopics checks that each topic prints its own section, and that the
// guide lists the topics it has.
func TestGuideTopics(t *testing.T) {
	topics := guideTopics()
	listed := strings.Join(topics[:len(topics)-1], ", ") + " or " + topics[len(topics)-1]
	if !strings.Contains(strings.Join(strings.Fields(guide), " "), listed) {
		t.Errorf("the guide does not list its topics as %q", listed)
	}
	for _, topic := range topics {
		out, _, err := capture(t, []string{"guide", topic})
		if err != nil {
			t.Fatalf("guide %s: %v", topic, err)
		}
		if !strings.HasPrefix(strings.ToLower(out), "## "+topic+"\n") || strings.Count(out, "\n## ") != 0 {
			t.Errorf("guide %s printed something other than its section:\n%s", topic, out)
		}
	}
	whole, _, err := capture(t, []string{"guide"})
	if err != nil || whole != guide {
		t.Errorf("guide did not print the whole guide: %v", err)
	}
	if _, _, err := capture(t, []string{"guide", "nothing"}); err == nil || !strings.Contains(err.Error(), "workflow") {
		t.Errorf("an unknown topic gave %v, want an error listing the topics", err)
	}
}

// TestGuideInstall checks the skill -install writes: where each agent looks for
// it, for one agent where asked, again over its own earlier copy, and never
// over a file someone else wrote.
func TestGuideInstall(t *testing.T) {
	claude := filepath.Join(".claude", "skills", "jmapc", "SKILL.md")
	codex := filepath.Join(".agents", "skills", "jmapc", "SKILL.md")
	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	dir := t.TempDir()
	if _, _, err := capture(t, []string{"guide", "-install", dir}); err != nil {
		t.Fatalf("guide -install: %v", err)
	}
	for _, path := range []string{claude, codex} {
		written, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatalf("the skill was not written to %s: %v", path, err)
		}
		if !strings.HasPrefix(string(written), "---\nname: jmapc\ndescription: ") || !strings.Contains(string(written), "jmapc guide") {
			t.Errorf("%s is not a skill that points at the guide:\n%s", path, written)
		}
		// A module that has jmapc as a tool runs it through go tool, and
		// validating is the one step the skill names beside the guide.
		for _, want := range []string{"go tool jmapc", "jmapc validate"} {
			if !strings.Contains(string(written), want) {
				t.Errorf("%s does not mention %s:\n%s", path, want, written)
			}
		}
	}
	if _, _, err := capture(t, []string{"guide", "-install", dir}); err != nil {
		t.Errorf("installing over the skill it wrote: %v", err)
	}

	one := t.TempDir()
	if _, _, err := capture(t, []string{"guide", "-install", one, "-for", "codex"}); err != nil {
		t.Fatalf("guide -install -for codex: %v", err)
	}
	if !exists(filepath.Join(one, codex)) || exists(filepath.Join(one, claude)) {
		t.Error("-for codex did not write the Codex skill alone")
	}
	if _, _, err := capture(t, []string{"guide", "-install", "-for", "cursor", one}); err == nil {
		t.Error("-for took an agent it does not know")
	}

	theirs := workspace(t, map[string]string{codex: "---\nname: jmapc\n---\nOur own notes.\n"})
	if _, _, err := capture(t, []string{"guide", "-install", theirs}); err == nil {
		t.Error("-install replaced a skill it did not write")
	}
	if got, _ := os.ReadFile(filepath.Join(theirs, codex)); string(got) != "---\nname: jmapc\n---\nOur own notes.\n" {
		t.Errorf("the skill someone else wrote was changed to:\n%s", got)
	}
	if exists(filepath.Join(theirs, claude)) {
		t.Error("-install wrote one skill after refusing the other")
	}
}
