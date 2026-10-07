package gen

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/request"
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
// what the example requests produce now.
func TestGeneratedExampleIsUpToDate(t *testing.T) {
	requests, props := parseExample(t)
	g := &RequestGenerator{
		Spec:       spec.Standard(),
		Package:    "client",
		Qualifier:  "jmapc.",
		Requests:   requests,
		Properties: props,
	}
	files, err := g.Generate()
	if err != nil {
		t.Fatalf("generating the example client: %v", err)
	}
	// One file per request, one holding the types for the named sets of
	// properties, and one holding Verify.
	if len(files) != len(requests)+2 {
		t.Errorf("generated %d files for %d requests", len(files), len(requests))
	}
	for name, src := range files {
		compare(t, filepath.Join(repoRoot, "example", "client", name), src, "go generate ./...")
	}
}

// TestGenerationIsDeterministic checks that generating twice gives the same
// bytes, so that a regenerated client never shows up as a spurious diff.
func TestGenerationIsDeterministic(t *testing.T) {
	newGen := func() *RequestGenerator {
		requests, props := parseExample(t)
		return &RequestGenerator{
			Spec:       spec.Standard(),
			Package:    "client",
			Qualifier:  "jmapc.",
			Requests:   requests,
			Properties: props,
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

// parseExample parses the example requests, together with the sets of
// properties they ask for by name.
func parseExample(t *testing.T) ([]*request.Request, *request.PropertySets) {
	t.Helper()
	dir := filepath.Join(repoRoot, "example", "requests")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	props, err := request.LoadPropertySets(filepath.Join(dir, request.PropertiesName), spec.Standard())
	if err != nil {
		t.Fatalf("checking %s:\n%v", request.PropertiesName, err)
	}
	props.Path = filepath.ToSlash(filepath.Join("requests", request.PropertiesName))
	parser := request.NewParser(spec.Standard())
	parser.Properties = props
	var out []*request.Request
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), request.Extension) {
			continue
		}
		q, err := parser.ParseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("checking %s:\n%v", e.Name(), err)
		}
		// The generated file records the path the generator was given, which
		// go:generate spells relative to the example directory.
		q.Path = filepath.ToSlash(filepath.Join("requests", e.Name()))
		out = append(out, q)
	}
	return out, props
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
	requests, props := parseExample(t)
	for _, q := range requests {
		q.Path = strings.ReplaceAll(q.Path, "/", `\`)
	}
	props.Path = strings.ReplaceAll(props.Path, "/", `\`)
	g := &RequestGenerator{
		Spec:       spec.Standard(),
		Package:    "client",
		Qualifier:  "jmapc.",
		Requests:   requests,
		Properties: props,
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

// generateOne generates the Go for one request written inline, and returns the
// source.
func generateOne(t *testing.T, name, src string) string {
	t.Helper()
	q, err := request.NewParser(spec.Standard()).Parse(name+request.Extension, []byte(src))
	if err != nil {
		t.Fatalf("checking %s:\n%v", name, err)
	}
	g := &RequestGenerator{Spec: spec.Standard(), Package: "client", Qualifier: "jmapc.", Requests: []*request.Request{q}}
	files, err := g.Generate()
	if err != nil {
		t.Fatalf("generating %s: %v", name, err)
	}
	return string(files[fileName(name)])
}

// TestWatchTakesTheAccountFromTheSession checks the account a watch listens
// for, where the request leaves it to the primary account: the events are keyed
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
		"return res.Changes.NewState, res.Changes.HasMoreChanges, nil",
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
		"changesArgs := map[string]any{",
		`"sinceState": p.SinceState,`,
		"if p.MaxChanges != nil {",
		`changesArgs["maxChanges"] = *p.MaxChanges`,
		`{Name: "Email/changes", CallID: "changes", Args: changesArgs},`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated request does not contain %q:\n%s", want, src)
		}
	}
}

// TestRequestsWithoutOptionalArgumentsStateTheirArguments checks that a request
// that leaves nothing out still says its arguments outright, so that the code
// keeps reading like the request it came from.
func TestRequestsWithoutOptionalArgumentsStateTheirArguments(t *testing.T) {
	src := generateOne(t, "GetChanges", `{
	  "methodCalls": [["Email/changes", {"sinceState": "{{sinceState}}"}, "changes"]]
	}`)
	if strings.Contains(src, "changesArgs") {
		t.Errorf("the arguments were built where they could have been stated:\n%s", src)
	}
	if !strings.Contains(src, `{Name: "Email/changes", CallID: "changes", Args: map[string]any{`) {
		t.Errorf("the generated request does not state its arguments:\n%s", src)
	}
}

// TestPatchKeysGoOutAsWritten checks that the key the generated code sends is
// the key the checker resolved, whether or not a parameter stands in part of
// it. A patch key is a JSON pointer with the leading "/" already there, and a
// key spelled with one is refused rather than trimmed, so these two agree.
func TestPatchKeysGoOutAsWritten(t *testing.T) {
	stated := generateOne(t, "MarkRead", `{
	  "methodCalls": [["Email/set", {"update": {"e1": {"keywords/$seen": true}}}, "c0"]]
	}`)
	if !strings.Contains(stated, `{"keywords/$seen":true}`) {
		t.Errorf("the patch does not go out as it was written:\n%s", stated)
	}

	built := generateOne(t, "MarkKeyword", `{
	  "methodCalls": [["Email/set", {"update": {"e1": {"keywords/{{keyword}}": true}}}, "c0"]]
	}`)
	if !strings.Contains(built, `"keywords/" + p.Keyword`) {
		t.Errorf("the patch key built from a parameter is not the one that was checked:\n%s", built)
	}
}

// TestWatchTakesTheAccountFromTheRequest checks the two ways a request names the
// account itself, neither of which costs a session lookup.
func TestWatchTakesTheAccountFromTheRequest(t *testing.T) {
	fromParameter := generateOne(t, "SyncEmails", `{
	  "_watches": "changes",
	  "methodCalls": [["Email/changes", {"accountId": "{{accountId}}", "sinceState": "{{sinceState}}"}, "changes"]]
	}`)
	if !strings.Contains(fromParameter, `return c.Watch(ctx, p.AccountID, "Email", p.SinceState,`) {
		t.Errorf("the watch does not listen for the account the caller names:\n%s", fromParameter)
	}
	if strings.Contains(fromParameter, "PrimaryAccountID") {
		t.Errorf("the watch looked up an account the request already names:\n%s", fromParameter)
	}

	stated := generateOne(t, "SyncOne", `{
	  "_watches": "changes",
	  "methodCalls": [["Email/changes", {"accountId": "a1", "sinceState": "{{sinceState}}"}, "changes"]]
	}`)
	if !strings.Contains(stated, `return c.Watch(ctx, jmapc.ID("a1"), "Email", p.SinceState,`) {
		t.Errorf("the watch does not listen for the account the request states:\n%s", stated)
	}
}

// TestWatchReadsTheStateItReturns checks a request that returns the watched call
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

// TestUnwatchedRequestsGetNoLoop checks that the function is generated only
// where the request asked for it.
func TestUnwatchedRequestsGetNoLoop(t *testing.T) {
	src := generateOne(t, "ListMailboxes", `{
	  "methodCalls": [["Mailbox/get", {"ids": null, "properties": ["id", "name"]}, "all"]],
	  "_returns": "all"
	}`)
	if strings.Contains(src, "Watch") {
		t.Errorf("a request that asked for no watch got one:\n%s", src)
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
		"window := &res.Search",
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

// TestOneShapeIsOneType checks the request whose calls read the same records in
// the same shape. Two types differing only by name would make a caller convert
// between them to pass a record from one call to a function written for the
// other, and the names are numbered by call position, so the second of them
// would move the moment a call was inserted before it.
func TestOneShapeIsOneType(t *testing.T) {
	src := generateOne(t, "TwoReads", `{
	  "methodCalls": [
	    ["Email/get", {"ids": ["{{a}}"], "properties": ["id", "subject"]}, "one"],
	    ["Email/get", {"ids": ["{{b}}"], "properties": ["id", "subject"]}, "two"]
	  ]
	}`)
	if strings.Contains(src, "TwoReadsTwoEmail") {
		t.Errorf("the same shape was given a second type:\n%s", src)
	}
	// The shape is named after the first call that reads it, and the second
	// call reads the same type rather than one of its own.
	if n := strings.Count(src, "type TwoReadsOneEmail struct {"); n != 1 {
		t.Errorf("the record type is declared %d times, want 1:\n%s", n, src)
	}
	if n := strings.Count(src, "type TwoReadsOneResponse struct {"); n != 1 {
		t.Errorf("the response type is declared %d times, want 1:\n%s", n, src)
	}
	// Both calls still answer separately; it is the type they share.
	for _, want := range []string{
		"One TwoReadsOneResponse",
		"Two TwoReadsOneResponse",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the result does not hold %q:\n%s", want, src)
		}
	}
}

// TestDifferentShapesKeepTheirOwnTypes checks the other half of it: calls that
// ask for different properties describe different records, whether the
// difference is in the properties themselves or in the parts of a body.
func TestDifferentShapesKeepTheirOwnTypes(t *testing.T) {
	src := generateOne(t, "TwoReads", `{
	  "methodCalls": [
	    ["Email/get", {"ids": ["{{a}}"], "properties": ["id", "subject"]}, "one"],
	    ["Email/get", {"ids": ["{{b}}"], "properties": ["id", "threadId"]}, "two"]
	  ]
	}`)
	for _, want := range []string{"type TwoReadsOneEmail struct {", "type TwoReadsTwoEmail struct {"} {
		if !strings.Contains(src, want) {
			t.Errorf("two shapes were given one type, and %q is missing:\n%s", want, src)
		}
	}

	bodies := generateOne(t, "TwoBodies", `{
	  "methodCalls": [
	    ["Email/get", {"ids": ["{{a}}"], "properties": ["id"], "bodyProperties": ["partId", "type"]}, "one"],
	    ["Email/get", {"ids": ["{{b}}"], "properties": ["id"], "bodyProperties": ["partId", "size"]}, "two"]
	  ]
	}`)
	for _, want := range []string{"type TwoBodiesOneEmailBodyPart struct {", "type TwoBodiesTwoEmailBodyPart struct {"} {
		if !strings.Contains(bodies, want) {
			t.Errorf("two body shapes were given one type, and %q is missing:\n%s", want, bodies)
		}
	}
}

// TestTheResponseIsNotDropped checks that a generated function hands back what
// the server did answer. Do returns the response alongside a MethodErrors
// because the calls around a failed one may still have run, and a generated
// function that returned nil there would throw that away.
func TestTheResponseIsNotDropped(t *testing.T) {
	src := generateOne(t, "DestroyThread", `{
	  "methodCalls": [
	    ["Thread/get", {"ids": ["{{threadId}}"]}, "thread"],
	    ["Email/set", {"#destroy": {"resultOf": "thread", "name": "Thread/get",
	                                "path": "/list/0/emailIds"}}, "destroy"]
	  ]
	}`)
	for _, want := range []string{
		// Only a response that never arrived leaves nothing to read.
		"if resp == nil {\n\t\treturn nil, err\n\t}",
		// A call the server would not run keeps its zero value, and anything
		// else wrong with the response is still reported on its own.
		`if e := resp.Decode("thread", &out.Thread); e != nil && err == nil {`,
		`if e := resp.Decode("destroy", &out.Destroy); e != nil && err == nil {`,
		// Both levels of failure travel together.
		"return &out, errors.Join(err, e)",
		"return &out, err\n}",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated request does not contain %q:\n%s", want, src)
		}
	}
}

// TestTheOneCallARequestReturnsIsAllOrNothing checks the other shape: a request
// naming one call in "_returns" has nothing to hand back when that call is the
// one that failed.
func TestTheOneCallARequestReturnsIsAllOrNothing(t *testing.T) {
	src := generateOne(t, "ReadOne", `{
	  "_returns": "fetch",
	  "methodCalls": [["Email/get", {"ids": ["{{emailId}}"]}, "fetch"]]
	}`)
	want := "\tif e := resp.Decode(\"fetch\", &out); e != nil {\n" +
		"\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n" +
		"\t\treturn nil, e\n\t}\n"
	if !strings.Contains(src, want) {
		t.Errorf("the generated request does not contain %q:\n%s", want, src)
	}
	if !strings.Contains(src, "return &out, err\n}") {
		t.Errorf("the request does not hand the error back with what it returns:\n%s", src)
	}
}

// TestACreationIDGetsAName checks the one string a caller had to repeat by
// hand. A /set reports what it created under the name the request gave it, so
// the caller wrote that name again in Go, in another file, with nothing
// holding the two together: renaming it in the request left the build green and
// the lookup missing.
func TestACreationIDGetsAName(t *testing.T) {
	src := generateOne(t, "CreateMailbox", `{
	  "_returns": "make",
	  "methodCalls": [["Mailbox/set", {"create": {"newMailbox": {"name": "{{name}}"}}}, "make"]]
	}`)
	for _, want := range []string{
		"// CreateMailboxNewMailbox is the creation id CreateMailbox gives a record it",
		`const CreateMailboxNewMailbox jmapc.ID = "newMailbox"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated request does not contain %q:\n%s", want, src)
		}
	}
}

// TestAnImportedEmailGetsAName checks that Email/import names the emails it
// creates as a /set does: they are keyed by creation id, and the response
// reports each one under it.
func TestAnImportedEmailGetsAName(t *testing.T) {
	src := generateOne(t, "ImportMessage", `{
	  "methodCalls": [["Email/import", {"emails": {"message": {
	    "blobId": "{{blobId}}", "mailboxIds": {"{{mailboxId}}": true}}}}, "import"]]
	}`)
	if want := `const ImportMessageMessage jmapc.ID = "message"`; !strings.Contains(src, want) {
		t.Errorf("the generated request does not contain %q:\n%s", want, src)
	}
}

// TestARecordIDIsNotACreationID checks that the keys of an update are left
// alone: they are record ids the caller already has, not names the request
// invents.
func TestARecordIDIsNotACreationID(t *testing.T) {
	src := generateOne(t, "MarkRead", `{
	  "methodCalls": [["Email/set", {"update": {"e1": {"keywords/$seen": true}}}, "mark"]]
	}`)
	if strings.Contains(src, "const MarkRead") {
		t.Errorf("a record id was named as though the request had invented it:\n%s", src)
	}
}

// TestAWholeFilterIsTyped checks the parameter that stands for a whole filter.
// A filter is either a boolean operator or a condition, and Go used to have no
// way to say that, so the parameter arrived as an any the caller had to get
// right on their own.
func TestAWholeFilterIsTyped(t *testing.T) {
	src := generateOne(t, "Search", `{
	  "methodCalls": [
	    ["Email/query", {"filter": "{{filter}}", "limit": 10}, "search"]
	  ]
	}`)
	want := "Filter jmapc.FilterOperatorOrEmailFilterCondition"
	if !strings.Contains(src, want) {
		t.Errorf("the parameters do not hold %q:\n%s", want, src)
	}
	if strings.Contains(src, "Filter any") {
		t.Errorf("the filter parameter is still an any:\n%s", src)
	}
}

// TestARecordKeepsItsNameWhenACallIsInserted is why a record is named after
// the call that read it. Numbering the types by the position of the call meant
// that inserting one moved a name onto a different shape, which a caller only
// found out about where the two shapes differed enough to stop compiling.
func TestARecordKeepsItsNameWhenACallIsInserted(t *testing.T) {
	before := generateOne(t, "Q", `{
	  "methodCalls": [
	    ["Email/get", {"ids": ["{{a}}"], "properties": ["id", "subject"]}, "one"],
	    ["Email/get", {"ids": ["{{b}}"], "properties": ["id", "keywords"]}, "two"]
	  ]
	}`)
	after := generateOne(t, "Q", `{
	  "methodCalls": [
	    ["Email/get", {"ids": ["{{c}}"], "properties": ["id", "from"]}, "zero"],
	    ["Email/get", {"ids": ["{{a}}"], "properties": ["id", "subject"]}, "one"],
	    ["Email/get", {"ids": ["{{b}}"], "properties": ["id", "keywords"]}, "two"]
	  ]
	}`)
	for _, want := range []string{"type QOneEmail struct {", "type QTwoEmail struct {"} {
		if !strings.Contains(before, want) {
			t.Fatalf("the request does not declare %q:\n%s", want, before)
		}
		if !strings.Contains(after, want) {
			t.Errorf("inserting a call ahead of the others took %q away:\n%s", want, after)
		}
	}
	// The shape each name stands for is the same before and after.
	if !strings.Contains(shapeOf(t, before, "QOneEmail"), "Subject") ||
		!strings.Contains(shapeOf(t, after, "QOneEmail"), "Subject") {
		t.Error("QOneEmail no longer holds the subject the call it is named after asked for")
	}
}

// shapeOf returns the body of a generated struct declaration.
func shapeOf(t *testing.T, src, name string) string {
	t.Helper()
	start := strings.Index(src, "type "+name+" struct {")
	if start < 0 {
		t.Fatalf("%s is not declared:\n%s", name, src)
	}
	end := strings.Index(src[start:], "\n}")
	if end < 0 {
		t.Fatalf("%s is not closed:\n%s", name, src)
	}
	return src[start : start+end]
}

// TestVerifyListsWhatEachRequestNeeds checks the table Verify is generated
// with: the capabilities a request declares, its calls, and the capabilities
// whose primary account fills in an account it leaves out, but not one it
// names itself.
func TestVerifyListsWhatEachRequestNeeds(t *testing.T) {
	parser := request.NewParser(spec.Standard())
	var requests []*request.Request
	for name, src := range map[string]string{
		"ListMailboxes": `{"methodCalls": [["Mailbox/get", {"ids": null}, "all"]]}`,
		"SharedMailboxes": `{"methodCalls": [
		  ["Mailbox/query", {"accountId": "shared"}, "search"],
		  ["Mailbox/get", {"accountId": "shared", "#ids": {"resultOf": "search", "name": "Mailbox/query", "path": "/ids"}}, "fetch"]
		]}`,
	} {
		q, err := parser.Parse(name+request.Extension, []byte(src))
		if err != nil {
			t.Fatalf("checking %s:\n%v", name, err)
		}
		requests = append(requests, q)
	}
	g := &RequestGenerator{Spec: spec.Standard(), Package: "client", Qualifier: "jmapc.", Requests: requests}
	files, err := g.Generate()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	src := string(files[VerifyFileName])
	for _, want := range []string{
		"func Verify(ctx context.Context, c *jmapc.Client) error {",
		"return c.Verify(ctx, []jmapc.RequestNeeds{",
		`Name:            "ListMailboxes",`,
		"Using:           []string{jmapc.CapabilityCore, jmapc.CapabilityMail},",
		"Calls:           1,",
		"PrimaryAccounts: []string{jmapc.CapabilityMail},",
		`Name:  "SharedMailboxes",`,
		"Calls: 2,",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("verify_gen.go has no %q:\n%s", want, src)
		}
	}
	// SharedMailboxes names its account, so no primary account is needed.
	shared := src[strings.Index(src, `"SharedMailboxes"`):]
	if strings.Contains(shared, "PrimaryAccounts") {
		t.Errorf("SharedMailboxes needs a primary account, though it names its own:\n%s", shared)
	}
}

// TestNothingIsGeneratedUnderTheNameOfVerify checks that a request or a set of
// properties named Verify is refused, rather than generated as a second
// declaration of the function, and into the same file.
func TestNothingIsGeneratedUnderTheNameOfVerify(t *testing.T) {
	q, err := request.NewParser(spec.Standard()).Parse("Verify"+request.Extension, []byte(`{"methodCalls": [["Mailbox/get", {"ids": null}, "all"]]}`))
	if err != nil {
		t.Fatalf("checking Verify: %v", err)
	}
	g := &RequestGenerator{Spec: spec.Standard(), Package: "client", Qualifier: "jmapc.", Requests: []*request.Request{q}}
	if _, err := g.Generate(); err == nil || !strings.Contains(err.Error(), "the function that verifies the requests") {
		t.Errorf("generating a request named Verify: %v, want it refused", err)
	}
}

// TestTwoThingsAreNotGeneratedIntoOneFile checks that a request whose file
// name another file already has is refused rather than overwritten: file names
// are lower case, so a request named verify would be written to the file Verify
// is generated into, and two requests differing only in case to one file.
func TestTwoThingsAreNotGeneratedIntoOneFile(t *testing.T) {
	const get = `{"methodCalls": [["Mailbox/get", {"ids": null}, "all"]]}`
	for _, tt := range []struct {
		name     string
		requests []string
		want     string
	}{
		{"a request named verify", []string{"verify"}, "verify_gen.go"},
		{"two requests differing in case", []string{"ListAll", "listAll"}, "listall_gen.go"},
	} {
		var requests []*request.Request
		for _, name := range tt.requests {
			q, err := request.NewParser(spec.Standard()).Parse(name+request.Extension, []byte(get))
			if err != nil {
				t.Fatalf("checking %s: %v", name, err)
			}
			requests = append(requests, q)
		}
		g := &RequestGenerator{Spec: spec.Standard(), Package: "client", Qualifier: "jmapc.", Requests: requests}
		_, err := g.Generate()
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want it refused for %s", tt.name, err, tt.want)
		}
	}
}

// TestARequestMayTakeAnyNameVerifyDoesNotDeclare checks that the table Verify
// is generated with declares nothing at package level, so a request may be
// named as the table would otherwise have been.
func TestARequestMayTakeAnyNameVerifyDoesNotDeclare(t *testing.T) {
	src := generateOne(t, "requestNeeds", `{"methodCalls": [["Mailbox/get", {"ids": null}, "all"]]}`)
	if !strings.Contains(src, "func requestNeeds(") {
		t.Fatalf("the request was not generated:\n%s", src)
	}
}

// TestVerifyNamesTheDirectoryHoldingEveryRequest checks the Source line of
// verify_gen.go where the requests sit in several directories under one, as
// they may: the requests are found by walking the directory given.
func TestVerifyNamesTheDirectoryHoldingEveryRequest(t *testing.T) {
	plans := func(paths ...string) []*plan {
		var ps []*plan
		for _, p := range paths {
			ps = append(ps, &plan{q: &request.Request{Path: p}})
		}
		return ps
	}
	for _, tt := range []struct {
		paths []string
		want  string
	}{
		{[]string{"requests/A.jmap.json"}, "requests"},
		{[]string{"requests/mail/A.jmap.json", "requests/contacts/B.jmap.json"}, "requests"},
		{[]string{"requests/mail/A.jmap.json", "requests/mail/B.jmap.json"}, "requests/mail"},
		{[]string{`requests\mail\A.jmap.json`, "requests/B.jmap.json"}, "requests"},
		{[]string{"a/A.jmap.json", "b/B.jmap.json"}, "."},
	} {
		if got := requestsDir(plans(tt.paths...)); got != tt.want {
			t.Errorf("requestsDir(%q) = %q, want %q", tt.paths, got, tt.want)
		}
	}
}

// TestAnEmptyListOfPropertiesFetchesTheIDAlone checks the record a call asking
// for no properties decodes into. The server answers with the id and nothing
// else, so the record holds the id rather than every property of the type.
func TestAnEmptyListOfPropertiesFetchesTheIDAlone(t *testing.T) {
	src := generateOne(t, "MailboxIDs", `{
	  "methodCalls": [["Mailbox/get", {"ids": null, "properties": []}, "ids"]],
	  "_returns": "ids"
	}`)
	record := src[strings.Index(src, "type MailboxIDsIDsMailbox struct {"):]
	record = record[:strings.Index(record, "\n}\n")]
	if !strings.Contains(record, "ID jmapc.ID `json:\"id\"`") {
		t.Errorf("the record does not hold the id:\n%s", record)
	}
	if n := strings.Count(record, "`json:"); n != 1 {
		t.Errorf("the record holds %d properties, want the id alone:\n%s", n, record)
	}
	if !strings.Contains(src, "\"properties\": json.RawMessage(`[]`)") {
		t.Errorf("the request does not send the empty list:\n%s", src)
	}
}

// TestAnEmptyListOfPropertiesOfAParseIsNoIDOfItsOwn checks an empty list given
// to a method other than a /get. It asks for no properties, as it does of a
// /get, but RFC 8620 promises the id of a /get alone, and a parsed email has
// none, so the record holds nothing.
func TestAnEmptyListOfPropertiesOfAParseIsNoIDOfItsOwn(t *testing.T) {
	src := generateOne(t, "ParseNothing", `{
	  "methodCalls": [["Email/parse", {"blobIds": ["{{blobId}}"], "properties": []}, "parse"]],
	  "_returns": "parse"
	}`)
	i := strings.Index(src, "type ParseNothingParseEmail struct {")
	if i < 0 {
		t.Fatalf("no record was made for the parsed emails, which fetch nothing:\n%s", src)
	}
	record := src[i:]
	record = record[:strings.Index(record, "}\n")]
	if n := strings.Count(record, "`json:"); n != 0 {
		t.Errorf("the record of a parsed email holds %d properties, want none, not even an id:\n%s", n, record)
	}
}

// TestAnEmptyListOfBodyPropertiesFetchesNothingOfAPart checks the body parts of
// a call asking for none of their properties. A part has no id to be returned
// whatever is asked for, so its record holds nothing.
func TestAnEmptyListOfBodyPropertiesFetchesNothingOfAPart(t *testing.T) {
	src := generateOne(t, "Parts", `{
	  "methodCalls": [["Email/get", {"ids": ["{{id}}"], "properties": ["textBody"], "bodyProperties": []}, "g"]],
	  "_returns": "g"
	}`)
	part := src[strings.Index(src, "type PartsGEmailBodyPart struct {"):]
	part = part[:strings.Index(part, "}\n")]
	if n := strings.Count(part, "`json:"); n != 0 {
		t.Errorf("the part holds %d properties, want none:\n%s", n, part)
	}
	if !strings.Contains(src, "TextBody []PartsGEmailBodyPart") {
		t.Errorf("the email does not hold its parts in the narrowed record:\n%s", src)
	}
}

// TestADocumentedPackageIsNotImported checks that the imports are the packages
// the code uses, not the names its comments hold. A request documented as
// reading settings.json. would otherwise import encoding/json, which go vet
// and the compiler then refuse as unused.
func TestADocumentedPackageIsNotImported(t *testing.T) {
	src := generateOne(t, "Settings", `{
	  "_doc": "Settings reads what settings.json. says, and errors.Join any failures.",
	  "methodCalls": [["Mailbox/get", {"ids": null}, "c0"]],
	  "_returns": "c0"
	}`)
	for _, unused := range []string{`"encoding/json"`, `"errors"`} {
		if strings.Contains(src, unused) {
			t.Errorf("the request imports %s, which it does not use:\n%s", unused, src)
		}
	}
}

// TestASetDocumentedWithPackageNamesImportsNeither checks the file of property
// sets, whose imports are decided apart from a request's: a set whose record
// holds strings alone, documented in words that name json. and jmapc., imports
// neither package.
func TestASetDocumentedWithPackageNamesImportsNeither(t *testing.T) {
	props, err := request.ParsePropertySets(request.PropertiesName, []byte(`{
	  "PartLabel": {
	    "doc": "PartLabel is what settings.json. shows of a part, as jmapc.Client reads it.",
	    "type": "EmailBodyPart",
	    "properties": ["partId", "type"]
	  }
	}`), spec.Standard())
	if err != nil {
		t.Fatalf("parsing the sets:\n%v", err)
	}
	g := &RequestGenerator{Spec: spec.Standard(), Package: "client", Qualifier: "jmapc.", Properties: props}
	files, err := g.Generate()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	src := string(files[PropertiesFileName])
	for _, unused := range []string{`"encoding/json"`, `"github.com/linyows/jmapc"`} {
		if strings.Contains(src, unused) {
			t.Errorf("the sets import %s, which they do not use:\n%s", unused, src)
		}
	}
}

// TestAVendorMethodThatReturnsTheIDGivesItsRecordsOne checks the record type
// generated for a vendor method that narrows its properties: where the schema
// says the method returns the id whatever it is asked for, the record holds the
// id the call did not ask for, and where it says nothing, the record holds only
// what was asked for.
func TestAVendorMethodThatReturnsTheIDGivesItsRecordsOne(t *testing.T) {
	for _, returnsID := range []bool{true, false} {
		s := spec.Standard()
		if err := s.Extend(&spec.Schema{
			Capability: "urn:example:notes",
			Types: []*spec.SchemaType{{Name: "Note", Methods: []string{"get"}, Properties: []*spec.SchemaField{
				{Name: "id", Type: "Id", ServerSet: true},
				{Name: "title", Type: "String"},
			}}},
			Methods: []*spec.SchemaMethod{{
				Name:           "Note/recent",
				DataType:       "Note",
				Arguments:      []*spec.SchemaField{{Name: "accountId", Type: "Id"}, {Name: "properties", Type: "String[]"}},
				Response:       []*spec.SchemaField{{Name: "accountId", Type: "Id"}, {Name: "list", Type: "Note[]"}},
				Properties:     "properties",
				ResultProperty: "list",
				ReturnsID:      returnsID,
			}},
		}); err != nil {
			t.Fatalf("Extend: %v", err)
		}
		q, err := request.NewParser(s).Parse("RecentNotes"+request.Extension,
			[]byte(`{"methodCalls": [["Note/recent", {"properties": ["title"]}, "r"]]}`))
		if err != nil {
			t.Fatalf("checking: %v", err)
		}
		g := &RequestGenerator{Spec: s, Package: "client", Qualifier: "jmapc.", Requests: []*request.Request{q}}
		files, err := g.Generate()
		if err != nil {
			t.Fatalf("generating: %v", err)
		}
		src := string(files[fileName("RecentNotes")])
		if has := strings.Contains(src, "ID jmapc.ID `json:\"id\"`"); has != returnsID {
			t.Errorf("returnsId %v: the record holds the id: %v\n%s", returnsID, has, src)
		}
	}
}

// TestABlobAskedForItsDataHoldsBothEncodings checks a Blob/get asking for data:
// the server answers under data:asText or data:asBase64, so the record holds
// both and no member named data, which never comes back.
func TestABlobAskedForItsDataHoldsBothEncodings(t *testing.T) {
	src := generateOne(t, "ReadBlob", `{"methodCalls": [["Blob/get", {"ids": ["b1"], "properties": ["data", "size"]}, "b"]]}`)
	for _, want := range []string{
		"DataAsText *string `json:\"data:asText\"`",
		"DataAsBase64 string `json:\"data:asBase64\"`",
		"whichever suits the value, so it may be absent.",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated code does not hold %s:\n%s", want, src)
		}
	}
	if strings.Contains(src, "`json:\"data\"`") {
		t.Errorf("the generated code holds a member named data:\n%s", src)
	}
}

// TestAParsedEmailHoldsWhatTheParseReturns checks the records of Email/parse:
// left to its defaults, the call is answered with the properties RFC 8621,
// Section 4.9 lists, so the record holds those and no id; and a threadId asked
// for may be null, since the server gives one only where it can tell which
// thread the message would join.
func TestAParsedEmailHoldsWhatTheParseReturns(t *testing.T) {
	src := generateOne(t, "ParseDefault", `{"methodCalls": [["Email/parse", {"blobIds": ["b1"]}, "p"]]}`)
	for _, want := range []string{
		"Parsed map[jmapc.ID]ParseDefaultPEmail `json:\"parsed\"`",
		"Preview string `json:\"preview\"`",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated code does not hold %s:\n%s", want, src)
		}
	}
	if strings.Contains(src, "`json:\"id\"`") || strings.Contains(src, "`json:\"receivedAt\"`") {
		t.Errorf("a parsed email holds a property the parse does not return:\n%s", src)
	}

	src = generateOne(t, "ParseThread", `{"methodCalls": [["Email/parse", {"blobIds": ["b1"], "properties": ["threadId"]}, "p"]]}`)
	if want := "ThreadID *jmapc.ID `json:\"threadId\"`"; !strings.Contains(src, want) {
		t.Errorf("the generated code does not hold %s:\n%s", want, src)
	}
}

// TestAParseGivenItsPropertiesHasItsOwnRecord checks an Email/parse whose
// properties the caller gives. Which come back is not known, but the parse
// answers some of them otherwise than the Email type says, so the record is a
// type of its own rather than the runtime's: no id, which the parse returns as
// null, and a threadId that may be null. A /get given its properties answers
// as the type says, and keeps the runtime's type, but where the method narrows
// the nested records by default, as Email/get does its body parts.
func TestAParseGivenItsPropertiesHasItsOwnRecord(t *testing.T) {
	src := generateOne(t, "ParseAny", `{"methodCalls": [["Email/parse", {"blobIds": ["b1"], "properties": "{{properties}}"}, "p"]]}`)
	for _, want := range []string{
		"Parsed map[jmapc.ID]ParseAnyPEmail `json:\"parsed\"`",
		"ThreadID *jmapc.ID `json:\"threadId\"`",
		"Subject *string `json:\"subject\"`",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated code does not hold %s:\n%s", want, src)
		}
	}
	if strings.Contains(src, "`json:\"id\"`") || strings.Contains(src, "`json:\"receivedAt\"`") {
		t.Errorf("a parsed email holds a property the parse returns as null:\n%s", src)
	}

	src = generateOne(t, "GetAny", `{"methodCalls": [["Mailbox/get", {"ids": ["a"], "properties": "{{properties}}"}, "g"]], "_returns": "g"}`)
	if want := "*jmapc.MailboxGetResponse"; !strings.Contains(src, want) {
		t.Errorf("the generated code does not answer with %s:\n%s", want, src)
	}

	src = generateOne(t, "GetAnyEmail", `{"methodCalls": [["Email/get", {"ids": ["a"], "properties": "{{properties}}"}, "g"]], "_returns": "g"}`)
	if want := "TextBody []GetAnyEmailGEmailBodyPart `json:\"textBody\"`"; !strings.Contains(src, want) {
		t.Errorf("the generated code does not hold %s:\n%s", want, src)
	}
}

// TestAnEmailLeftToItsDefaultsHoldsWhatTheGetReturns checks an Email/get that
// leaves its properties out. RFC 8621, Section 4.2 has it return a list of its
// own, without the headers or the body structure, and body parts without their
// headers or sub-parts, so the records and the body parts are types of their
// own. A call that fetches no body parts has no body part type.
func TestAnEmailLeftToItsDefaultsHoldsWhatTheGetReturns(t *testing.T) {
	src := generateOne(t, "GetDefault", `{"methodCalls": [["Email/get", {"ids": ["a"]}, "g"]], "_returns": "g"}`)
	for _, want := range []string{
		"List []GetDefaultGEmail `json:\"list\"`",
		"TextBody []GetDefaultGEmailBodyPart `json:\"textBody\"`",
		"ThreadID jmapc.ID `json:\"threadId\"`",
		"PartID *string `json:\"partId\"`",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated code does not hold %s:\n%s", want, src)
		}
	}
	for _, notWant := range []string{"`json:\"bodyStructure\"`", "`json:\"headers\"`", "`json:\"subParts\"`"} {
		if strings.Contains(src, notWant) {
			t.Errorf("the generated code holds %s, which the get does not return:\n%s", notWant, src)
		}
	}

	src = generateOne(t, "GetSubject", `{"methodCalls": [["Email/get", {"ids": ["a"], "properties": ["subject"]}, "g"]], "_returns": "g"}`)
	if strings.Contains(src, "EmailBodyPart struct") {
		t.Errorf("a get fetching no body parts has a body part type:\n%s", src)
	}
}

// TestAParsedEventHoldsNoMetadata checks a CalendarEvent/parse that fetches
// every property: an event read from a file is in no calendar, so the record
// holds none of the metadata the parse returns as null, and only the properties
// the file gives come back, so any may be absent.
func TestAParsedEventHoldsNoMetadata(t *testing.T) {
	src := generateOne(t, "ParseEvents", `{"methodCalls": [["CalendarEvent/parse", {"blobIds": ["b1"]}, "p"]]}`)
	if want := "Parsed map[jmapc.ID][]ParseEventsPCalendarEvent `json:\"parsed\"`"; !strings.Contains(src, want) {
		t.Errorf("the generated code does not hold %s:\n%s", want, src)
	}
	for _, notWant := range []string{"`json:\"id\"`", "`json:\"calendarIds\"`", "`json:\"baseEventId\"`", "`json:\"isDraft\"`", "`json:\"isOrigin\"`"} {
		if strings.Contains(src, notWant) {
			t.Errorf("the generated code holds %s, which the parse returns as null:\n%s", notWant, src)
		}
	}
}

// TestEventsLeftToTheirDefaultsHoldNoTimesInUTC checks a CalendarEvent/get
// that leaves its properties out, which returns every property but utcStart and
// utcEnd: the record is a type of its own without those, and holds the id,
// which a /get always returns, while any other property may be absent. Given
// its properties, the call may ask for them, and keeps the runtime's type.
func TestEventsLeftToTheirDefaultsHoldNoTimesInUTC(t *testing.T) {
	src := generateOne(t, "GetEvents", `{"methodCalls": [["CalendarEvent/get", {"ids": ["e1"]}, "g"]], "_returns": "g"}`)
	for _, want := range []string{
		"List []GetEventsGCalendarEvent `json:\"list\"`",
		"ID jmapc.ID `json:\"id\"`",
		"Title string `json:\"title\"`",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated code does not hold %s:\n%s", want, src)
		}
	}
	for _, notWant := range []string{"`json:\"utcStart\"`", "`json:\"utcEnd\"`"} {
		if strings.Contains(src, notWant) {
			t.Errorf("the generated code holds %s, which the get does not return:\n%s", notWant, src)
		}
	}

	src = generateOne(t, "GetAnyEvents", `{"methodCalls": [["CalendarEvent/get", {"ids": ["e1"], "properties": "{{properties}}"}, "g"]], "_returns": "g"}`)
	if want := "*jmapc.CalendarEventGetResponse"; !strings.Contains(src, want) {
		t.Errorf("the generated code does not answer with %s:\n%s", want, src)
	}
}

// TestABlobLeftToItsDefaultsHoldsBothEncodings checks a Blob/get that leaves
// its properties out: RFC 9404, Section 4.2 has it return data and size, so the
// record holds both encodings for the server to pick between, and the size.
func TestABlobLeftToItsDefaultsHoldsBothEncodings(t *testing.T) {
	src := generateOne(t, "ReadBlob", `{"methodCalls": [["Blob/get", {"ids": ["b1"]}, "b"]]}`)
	for _, want := range []string{
		"DataAsText *string `json:\"data:asText\"`",
		"whichever suits the value, so it may be absent.",
		"Size jmapc.UnsignedInt `json:\"size\"`",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated code does not hold %s:\n%s", want, src)
		}
	}
}
