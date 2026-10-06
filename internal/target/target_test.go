package target

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// TestEachLanguageIsWrittenWhole checks the files each language's client is
// made of: a file per request in all three, and beside them the data model and
// the runtime where the language has no runtime package to import.
func TestEachLanguageIsWrittenWhole(t *testing.T) {
	q, err := request.NewParser(spec.Standard()).Parse("ListMailboxes"+request.Extension,
		[]byte(`{"methodCalls": [["Mailbox/get", {"ids": null}, "c0"]]}`))
	if err != nil {
		t.Fatalf("checking the request:\n%v", err)
	}
	for lang, want := range map[string][]string{
		Go:         {"listmailboxes_gen.go"},
		Rust:       {"list_mailboxes.rs", "types.rs", "client.rs", "mod.rs"},
		TypeScript: {"listMailboxes.ts", "types.ts", "client.ts"},
	} {
		files, err := (&Client{Lang: lang, Package: "client", Spec: spec.Standard(), Requests: []*request.Request{q}}).Generate()
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		for _, name := range want {
			if _, ok := files[name]; !ok {
				t.Errorf("%s: no %s among the files", lang, name)
			}
		}
	}
	if _, err := (&Client{Lang: "cobol", Spec: spec.Standard()}).Generate(); err == nil || !strings.Contains(err.Error(), "cobol") {
		t.Errorf("an unknown language: %v, want it refused", err)
	}
}
