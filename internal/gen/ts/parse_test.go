package ts

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// TestAParsedEmailMayHaveNoThread checks the threadId of Email/parse, which the
// server gives only where it can tell which thread the message would join, so
// the record holds it as one that may be null.
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
	src := string(files["parseThread.ts"])
	if !strings.Contains(src, "threadId: Id | null") {
		t.Errorf("the threadId of a parsed email may not be null:\n%s", src)
	}
}
