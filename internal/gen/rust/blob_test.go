package rust

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// TestABlobAskedForItsDataHoldsBothEncodings checks a Blob/get asking for data:
// the server answers under data:asText or data:asBase64, whichever suits the
// octets, so the record holds both as options, and no member named data.
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
	src := string(files["read_blob.rs"])
	for _, want := range []string{
		"pub data_as_text: Option<String>,",
		"pub data_as_base_64: Option<String>,",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated code does not hold %s:\n%s", want, src)
		}
	}
	if strings.Contains(src, "pub data:") {
		t.Errorf("the generated code holds a member named data:\n%s", src)
	}
}
