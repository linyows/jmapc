package rust

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

const verifyGet = `{"methodCalls": [["Mailbox/get", {"ids": null}, "all"]]}`

func parseVerifyRequests(t *testing.T, names ...string) []*request.Request {
	t.Helper()
	var requests []*request.Request
	for _, name := range names {
		q, err := request.NewParser(spec.Standard()).Parse("requests/"+name+request.Extension, []byte(verifyGet))
		if err != nil {
			t.Fatalf("checking %s: %v", name, err)
		}
		requests = append(requests, q)
	}
	return requests
}

// TestVerifyListsWhatEachRequestNeeds checks the file verify is generated
// into: the function, and for each request the capabilities it declares, its
// calls, and the capabilities whose primary account fills in its account.
func TestVerifyListsWhatEachRequestNeeds(t *testing.T) {
	files, err := (&RequestGenerator{Spec: spec.Standard(), Requests: parseVerifyRequests(t, "ListMailboxes")}).Generate()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	src := string(files["verify.rs"])
	for _, want := range []string{
		"// Source: requests\n",
		`pub async fn verify<T: Transport>(client: &Client<T>) -> Result<(), Error> {`,
		`name: "ListMailboxes",`,
		`using: &["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"],`,
		"calls: 1,",
		`primary_accounts: &["urn:ietf:params:jmap:mail"],`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("%s has no %q:\n%s", "verify.rs", want, src)
		}
	}
}

// TestARequestCannotTakeTheModuleOfAnother checks that a request whose module
// would be one the runtime, verify or another request is generated into is
// refused rather than written over it.
func TestARequestCannotTakeTheModuleOfAnother(t *testing.T) {
	for _, name := range []string{"Client", "Types", "Mod", "Verify"} {
		_, err := (&RequestGenerator{Spec: spec.Standard(), Requests: parseVerifyRequests(t, name)}).Generate()
		if err == nil || !strings.Contains(err.Error(), "would both be generated into") {
			t.Errorf("a request named %s: %v, want it refused", name, err)
		}
	}
	_, err := (&RequestGenerator{Spec: spec.Standard(), Requests: parseVerifyRequests(t, "ListAll", "list_All")}).Generate()
	if err == nil || !strings.Contains(err.Error(), "list_all.rs") {
		t.Errorf("two requests generated into one module: %v, want them refused", err)
	}
}
