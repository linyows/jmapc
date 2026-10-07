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

// TestARecordGivenItsPropertiesMayLackAny checks the records of a call whose
// properties the caller gives, so any of them may be absent: every member is
// optional but the id a /get returns whatever it is asked for, and a parse
// holds no id at all, which it returns as null.
func TestARecordGivenItsPropertiesMayLackAny(t *testing.T) {
	tests := []struct {
		name, file, src string
		want, notWant   []string
	}{{
		name:    "ParseAny",
		file:    "parseAny.ts",
		src:     `{"methodCalls": [["Email/parse", {"blobIds": ["b1"], "properties": "{{properties}}"}, "p"]]}`,
		want:    []string{"parsed: { [key: Id]: ParseAnyPEmail } | null", "threadId?: Id | null", "subject?: string | null"},
		notWant: []string{"  id", "  receivedAt"},
	}, {
		// Only the body parts are narrowed, which gives the records a type
		// of their own as well.
		name:    "GetAny",
		file:    "getAny.ts",
		src:     `{"methodCalls": [["Email/get", {"ids": ["a"], "properties": "{{properties}}", "bodyProperties": ["partId"]}, "g"]]}`,
		want:    []string{"  id: Id\n", "blobId?: Id", "textBody?: GetAnyGEmailBodyPart[]"},
		notWant: []string{"id?: Id"},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := request.NewParser(spec.Standard()).Parse(tt.name+request.Extension, []byte(tt.src))
			if err != nil {
				t.Fatalf("checking: %v", err)
			}
			files, err := (&RequestGenerator{Spec: spec.Standard(), Requests: []*request.Request{q}}).Generate()
			if err != nil {
				t.Fatalf("generating: %v", err)
			}
			src := string(files[tt.file])
			for _, want := range tt.want {
				if !strings.Contains(src, want) {
					t.Errorf("the generated code does not hold %q:\n%s", want, src)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(src, notWant) {
					t.Errorf("the generated code holds %q:\n%s", notWant, src)
				}
			}
		})
	}
}
