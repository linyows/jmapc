package rust

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// TestAParsedEmailMayHaveNoThread checks the threadId of Email/parse, which the
// server gives only where it can tell which thread the message would join, so
// the record holds it as an Option.
func TestAParsedEmailMayHaveNoThread(t *testing.T) {
	q, err := request.NewParser(spec.Standard()).Parse("ParseThread"+request.Extension,
		[]byte(`{"methodCalls": [["Email/parse", {"blobIds": ["b1"], "properties": ["threadId", "subject"]}, "p"]]}`))
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	files, err := (&RequestGenerator{Spec: spec.Standard(), Requests: []*request.Request{q}}).Generate()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	src := string(files["parse_thread.rs"])
	if !strings.Contains(src, "pub thread_id: Option<Id>,") {
		t.Errorf("the threadId of a parsed email is not an Option:\n%s", src)
	}
}
