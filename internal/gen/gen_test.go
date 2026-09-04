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
		Skip:    map[string]bool{"PatchObject": true, "SetError": true, "Account": true},
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
		if wantLines[i] == gotLines[i] {
			continue
		}
		onDisk, generated := wantLines[i], gotLines[i]
		if strings.TrimSpace(onDisk) == strings.TrimSpace(generated) {
			// The lines differ only in whitespace, which printed plainly would
			// look identical and send the reader looking in the wrong place.
			onDisk, generated = strconv.Quote(onDisk), strconv.Quote(generated)
		}
		return "first difference at line " + strconv.Itoa(i+1) +
			":\n\ton disk:   " + onDisk + "\n\tgenerated: " + generated
	}
	return "the files differ in length: on disk has " + strconv.Itoa(len(wantLines)) + " lines, generated has " + strconv.Itoa(len(gotLines))
}

// TestGeneratedSourcePathIsPortable checks that the path a generated file
// records is spelled the same however the host spells paths. Without this,
// regenerating on Windows would rewrite the first line of every file, and the
// check that the committed client is up to date would fail on one platform and
// pass on another.
func TestGeneratedSourcePathIsPortable(t *testing.T) {
	queries := parseExample(t)
	for _, q := range queries {
		q.Path = strings.ReplaceAll(q.Path, "/", `\`)
	}
	g := &QueryGenerator{
		Spec:      spec.Standard(),
		Package:   "jmapq",
		Qualifier: "jmapc.",
		Queries:   queries,
	}
	files, err := g.Generate()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	for name, src := range files {
		header := strings.SplitN(string(src), "\n", 3)[1]
		if strings.Contains(header, `\`) {
			t.Errorf("%s records its source as %q, want forward slashes", name, header)
		}
	}
}

// generateOne generates the Go for one query written inline, and returns the
// source.
func generateOne(t *testing.T, name, src string) string {
	t.Helper()
	q, err := query.NewParser(spec.Standard()).Parse(name+query.Extension, []byte(src))
	if err != nil {
		t.Fatalf("checking %s:\n%v", name, err)
	}
	g := &QueryGenerator{Spec: spec.Standard(), Package: "jmapq", Qualifier: "jmapc.", Queries: []*query.Query{q}}
	files, err := g.Generate()
	if err != nil {
		t.Fatalf("generating %s: %v", name, err)
	}
	return string(files[fileName(name)])
}

// TestWatchTakesTheAccountFromTheSession checks the account a watch listens
// for, where the query leaves it to the primary account: the events are keyed
// by account, so the loop has to resolve it before it makes any request.
func TestWatchTakesTheAccountFromTheSession(t *testing.T) {
	src := generateOne(t, "SyncMailboxes", `{
	  "_watches": "changes",
	  "methodCalls": [["Mailbox/changes", {"sinceState": "{{sinceState}}"}, "changes"]]
	}`)
	for _, want := range []string{
		"func SyncMailboxesWatch(ctx context.Context, c *jmapc.Client, p SyncMailboxesParams, fn func(context.Context, *SyncMailboxesResult) error, opts ...jmapc.WatchOption) error {",
		"mailAccountID, err := session.PrimaryAccountID(jmapc.CapabilityMail)",
		`return c.Watch(ctx, mailAccountID, "Mailbox", p.SinceState,`,
		"p.SinceState = sinceState",
		"return res.MailboxChanges.NewState, res.MailboxChanges.HasMoreChanges, nil",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated watch does not contain %q:\n%s", want, src)
		}
	}
}

// TestOptionalArgumentIsPutInOnlyWhenGiven checks the argument a caller may
// leave out: the arguments are built rather than stated, and the member is put
// in only where there is a value for it.
func TestOptionalArgumentIsPutInOnlyWhenGiven(t *testing.T) {
	src := generateOne(t, "GetChanges", `{
	  "methodCalls": [["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": "{{maxChanges?}}"}, "changes"]]
	}`)
	for _, want := range []string{
		"MaxChanges *jmapc.UnsignedInt",
		"emailChangesArgs := map[string]any{",
		`"sinceState": p.SinceState,`,
		"if p.MaxChanges != nil {",
		`emailChangesArgs["maxChanges"] = *p.MaxChanges`,
		`{Name: "Email/changes", CallID: "changes", Args: emailChangesArgs},`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated query does not contain %q:\n%s", want, src)
		}
	}
}

// TestQueriesWithoutOptionalArgumentsStateTheirArguments checks that a query
// that leaves nothing out still says its arguments outright, so that the code
// keeps reading like the query it came from.
func TestQueriesWithoutOptionalArgumentsStateTheirArguments(t *testing.T) {
	src := generateOne(t, "GetChanges", `{
	  "methodCalls": [["Email/changes", {"sinceState": "{{sinceState}}"}, "changes"]]
	}`)
	if strings.Contains(src, "emailChangesArgs") {
		t.Errorf("the arguments were built where they could have been stated:\n%s", src)
	}
	if !strings.Contains(src, `{Name: "Email/changes", CallID: "changes", Args: map[string]any{`) {
		t.Errorf("the generated query does not state its arguments:\n%s", src)
	}
}

// TestWatchTakesTheAccountFromTheQuery checks the two ways a query names the
// account itself, neither of which costs a session lookup.
func TestWatchTakesTheAccountFromTheQuery(t *testing.T) {
	fromParameter := generateOne(t, "SyncEmails", `{
	  "_watches": "changes",
	  "methodCalls": [["Email/changes", {"accountId": "{{accountId}}", "sinceState": "{{sinceState}}"}, "changes"]]
	}`)
	if !strings.Contains(fromParameter, `return c.Watch(ctx, p.AccountID, "Email", p.SinceState,`) {
		t.Errorf("the watch does not listen for the account the caller names:\n%s", fromParameter)
	}
	if strings.Contains(fromParameter, "PrimaryAccountID") {
		t.Errorf("the watch looked up an account the query already names:\n%s", fromParameter)
	}

	stated := generateOne(t, "SyncOne", `{
	  "_watches": "changes",
	  "methodCalls": [["Email/changes", {"accountId": "a1", "sinceState": "{{sinceState}}"}, "changes"]]
	}`)
	if !strings.Contains(stated, `return c.Watch(ctx, jmapc.ID("a1"), "Email", p.SinceState,`) {
		t.Errorf("the watch does not listen for the account the query states:\n%s", stated)
	}
}

// TestWatchReadsTheStateItReturns checks a query that returns the watched call
// alone, where the state is on the response itself rather than on a field of a
// result holding every response.
func TestWatchReadsTheStateItReturns(t *testing.T) {
	src := generateOne(t, "SyncEmails", `{
	  "_watches": "changes",
	  "_returns": "changes",
	  "methodCalls": [["Email/changes", {"sinceState": "{{sinceState}}"}, "changes"]]
	}`)
	if !strings.Contains(src, "return res.NewState, res.HasMoreChanges, nil") {
		t.Errorf("the watch does not read the state off the response it returns:\n%s", src)
	}
}

// TestUnwatchedQueriesGetNoLoop checks that the function is generated only
// where the query asked for it.
func TestUnwatchedQueriesGetNoLoop(t *testing.T) {
	src := generateOne(t, "ListMailboxes", `{
	  "methodCalls": [["Mailbox/get", {"ids": null, "properties": ["id", "name"]}, "all"]],
	  "_returns": "all"
	}`)
	if strings.Contains(src, "Watch") {
		t.Errorf("a query that asked for no watch got one:\n%s", src)
	}
}

// TestPagesWalksAWindow checks the loop generated for a call that answers with
// one window of a longer list: where the next window starts, and the two things
// that end the walk.
func TestPagesWalksAWindow(t *testing.T) {
	src := generateOne(t, "SearchEmails", `{
	  "_pages": "search",
	  "methodCalls": [
	    ["Email/query", {"position": "{{position}}", "limit": 50, "calculateTotal": true}, "search"],
	    ["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
	                   "properties": ["id", "subject"]}, "fetch"]
	  ]
	}`)
	for _, want := range []string{
		"func SearchEmailsPages(ctx context.Context, c *jmapc.Client, p SearchEmailsParams) iter.Seq2[*SearchEmailsResult, error] {",
		"\"iter\"",
		"start := p.Position",
		"window := &res.EmailQuery",
		"if len(window.IDs) == 0 {",
		"start = jmapc.Int(window.Position) + jmapc.Int(len(window.IDs))",
		"if window.Total > 0 && jmapc.UnsignedInt(start) >= window.Total {",
		"yield(nil, err)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated walk does not contain %q:\n%s", want, src)
		}
	}
}

// TestPagesWalksChanges checks the loop generated for a call that answers with
// as much of what changed as the server cares to, which ends when the server
// says there is no more.
func TestPagesWalksChanges(t *testing.T) {
	src := generateOne(t, "CatchUp", `{
	  "_pages": "changes",
	  "_returns": "changes",
	  "methodCalls": [["Email/changes", {"sinceState": "{{sinceState}}"}, "changes"]]
	}`)
	for _, want := range []string{
		"start := p.SinceState",
		"window := res",
		"if !window.HasMoreChanges {",
		"start = window.NewState",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated walk does not contain %q:\n%s", want, src)
		}
	}
	// Every answer is handed back, even one saying nothing changed, because it
	// carries the state to go on from.
	if strings.Contains(src, "== 0 {") {
		t.Errorf("an empty answer was skipped, and it carries the state:\n%s", src)
	}
}
