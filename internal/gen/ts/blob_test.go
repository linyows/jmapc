package ts

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// TestABlobAskedForItsDataHoldsBothEncodings checks a Blob/get asking for data:
// the server answers under data:asText or data:asBase64, whichever suits the
// octets, so the record holds both as optional members, and no member named
// data.
func TestABlobAskedForItsDataHoldsBothEncodings(t *testing.T) {
	q, err := request.NewParser(spec.Standard()).Parse("ReadBlob"+request.Extension,
		[]byte(`{"methodCalls": [["Blob/get", {"ids": ["b1"], "properties": ["data", "size"]}, "b"]]}`))
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	files, err := (&RequestGenerator{Spec: spec.Standard(), Requests: []*request.Request{q}}).Generate()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	src := string(files["readBlob.ts"])
	for _, want := range []string{
		`"data:asText"?: string | null`,
		`"data:asBase64"?: string`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated code does not hold %s:\n%s", want, src)
		}
	}
	if strings.Contains(src, "  data: unknown") {
		t.Errorf("the generated code holds a member named data:\n%s", src)
	}
}
