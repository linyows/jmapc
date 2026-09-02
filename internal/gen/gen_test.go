package gen

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// repoRoot is the module root, relative to this package.
const repoRoot = "../.."

// TestGeneratedTypesAreUpToDate checks the committed runtime types against what
// the catalogue produces now, so that a change to the data model cannot be
// forgotten on its way into the generated code.
func TestGeneratedTypesAreUpToDate(t *testing.T) {
	g := &TypeGenerator{
		Spec:    spec.Standard(),
		Package: "jmapc",
		Skip:    map[string]bool{"PatchObject": true, "SetError": true},
	}
	got, err := g.Generate()
	if err != nil {
		t.Fatalf("generating types: %v", err)
	}
	compare(t, filepath.Join(repoRoot, "types_gen.go"), got, "go generate ./...")
}

// TestGeneratedExampleIsUpToDate checks the committed example client against
// what the example queries produce now.
func TestGeneratedExampleIsUpToDate(t *testing.T) {
	queries := parseExample(t)
	g := &QueryGenerator{
		Spec:      spec.Standard(),
		Package:   "jmapq",
		Qualifier: "jmapc.",
		Queries:   queries,
	}
	files, err := g.Generate()
	if err != nil {
		t.Fatalf("generating the example client: %v", err)
	}
	if len(files) != len(queries) {
		t.Errorf("generated %d files for %d queries", len(files), len(queries))
	}
	for name, src := range files {
		compare(t, filepath.Join(repoRoot, "example", "jmapq", name), src, "go generate ./...")
	}
}

// TestGenerationIsDeterministic checks that generating twice gives the same
// bytes, so that a regenerated client never shows up as a spurious diff.
func TestGenerationIsDeterministic(t *testing.T) {
	newGen := func() *QueryGenerator {
		return &QueryGenerator{
			Spec:      spec.Standard(),
			Package:   "jmapq",
			Qualifier: "jmapc.",
			Queries:   parseExample(t),
		}
	}
	first, err := newGen().Generate()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	second, err := newGen().Generate()
	if err != nil {
		t.Fatalf("generating again: %v", err)
	}
	for name, src := range first {
		if string(second[name]) != string(src) {
			t.Errorf("%s differs between two runs", name)
		}
	}
}

// parseExample parses the example queries.
func parseExample(t *testing.T) []*query.Query {
	t.Helper()
	dir := filepath.Join(repoRoot, "example", "queries")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	parser := query.NewParser(spec.Standard())
	var out []*query.Query
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), query.Extension) {
			continue
		}
		q, err := parser.ParseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("checking %s:\n%v", e.Name(), err)
		}
		// The generated file records the path the generator was given, which
		// go:generate spells relative to the example directory.
		q.Path = filepath.ToSlash(filepath.Join("queries", e.Name()))
		out = append(out, q)
	}
	return out
}

// compare checks a generated file against the one on disk.
func compare(t *testing.T, path string, got []byte, regenerate string) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if string(got) == string(want) {
		return
	}
	t.Errorf("%s is out of date; run %s\n%s", path, regenerate, firstDifference(string(want), string(got)))
}

// firstDifference describes where two generated files start to differ, which
// says more than dumping both of them.
func firstDifference(want, got string) string {
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			return "first difference at line " + strconv.Itoa(i+1) + ":\n\ton disk:   " + wantLines[i] + "\n\tgenerated: " + gotLines[i]
		}
	}
	return "the files differ in length: on disk has " + strconv.Itoa(len(wantLines)) + " lines, generated has " + strconv.Itoa(len(gotLines))
}
