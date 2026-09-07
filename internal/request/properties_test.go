package request

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/spec"
)

// emailSets is a properties file naming two shapes of Email, the second adding
// the body to the first.
const emailSets = `{
  "EmailSummary": {
    "doc": "EmailSummary is what a list of messages shows.",
    "type": "Email",
    "properties": ["id", "threadId", "subject", "from", "receivedAt"]
  },
  "EmailWithBody": {
    "extends": "EmailSummary",
    "properties": ["textBody", "bodyValues"]
  }
}`

// sets checks src as a properties file and fails the test if it does not check
// out.
func sets(t *testing.T, src string) *PropertySets {
	t.Helper()
	p, err := ParsePropertySets(PropertiesName, []byte(src), spec.Standard())
	if err != nil {
		t.Fatalf("parsing %s:\n%v", PropertiesName, err)
	}
	return p
}

// setsErr checks src and returns the error, failing the test if there is none.
func setsErr(t *testing.T, src string) string {
	t.Helper()
	_, err := ParsePropertySets(PropertiesName, []byte(src), spec.Standard())
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	return err.Error()
}

// parseWith checks a request against the given property sets.
func parseWith(t *testing.T, props *PropertySets, src string) (*Request, error) {
	t.Helper()
	p := NewParser(spec.Standard())
	p.Properties = props
	return p.Parse("GetEmails"+Extension, []byte(src))
}

func TestPropertySets(t *testing.T) {
	p := sets(t, emailSets)

	if got := p.Names(); strings.Join(got, ",") != "EmailSummary,EmailWithBody" {
		t.Errorf("Names() = %v, want the two sets in declaration order", got)
	}
	summary, ok := p.Find("EmailSummary")
	if !ok {
		t.Fatal("EmailSummary is not there")
	}
	if summary.Type != "Email" {
		t.Errorf("Type = %q, want Email", summary.Type)
	}
	if summary.Doc == "" {
		t.Error("the doc was dropped")
	}
	body, _ := p.Find("EmailWithBody")
	if body.Extends != summary {
		t.Error("EmailWithBody does not extend EmailSummary")
	}
	if body.Type != "Email" {
		t.Errorf("Type = %q, want the type of the set it extends", body.Type)
	}
	want := "id,threadId,subject,from,receivedAt,textBody,bodyValues"
	if got := strings.Join(body.Properties(), ","); got != want {
		t.Errorf("Properties() = %q, want %q", got, want)
	}
	if got := strings.Join(body.Own, ","); got != "textBody,bodyValues" {
		t.Errorf("Own = %q, want only what the set itself adds", got)
	}
}

func TestPropertySetErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "unknown type",
		src:  `{"S": {"type": "Emial", "properties": ["id"]}}`,
		want: `unknown type "Emial"`,
	}, {
		name: "unknown property",
		src:  `{"S": {"type": "Email", "properties": ["subjcet"]}}`,
		want: `Email has no property "subjcet"`,
	}, {
		name: "no type",
		src:  `{"S": {"properties": ["id"]}}`,
		want: "S says nothing about which type its properties belong to",
	}, {
		name: "no properties",
		src:  `{"S": {"type": "Email", "properties": []}}`,
		want: "S selects no properties",
	}, {
		name: "unknown set extended",
		src:  `{"S": {"extends": "Base", "properties": ["id"]}}`,
		want: `no set is named "Base"`,
	}, {
		name: "extends itself",
		src:  `{"S": {"extends": "S", "properties": ["id"]}}`,
		want: "a set cannot extend itself",
	}, {
		name: "circle",
		src: `{"A": {"extends": "B", "properties": ["id"]},
		       "B": {"extends": "A", "properties": ["subject"]}}`,
		want: "extend one another in a circle",
	}, {
		name: "type disagrees with the extended set",
		src: `{"A": {"type": "Email", "properties": ["id"]},
		       "B": {"extends": "A", "type": "Mailbox", "properties": ["name"]}}`,
		want: "B narrows Mailbox, but the set it extends narrows Email",
	}, {
		name: "property selected twice",
		src:  `{"S": {"type": "Email", "properties": ["id", "subject", "subject"]}}`,
		want: `"subject" is selected twice`,
	}, {
		name: "property already in the extended set",
		src: `{"A": {"type": "Email", "properties": ["id", "subject"]},
		       "B": {"extends": "A", "properties": ["subject"]}}`,
		want: `"subject" is selected twice`,
	}, {
		name: "name is not a type name",
		src:  `{"email summary": {"type": "Email", "properties": ["id"]}}`,
		want: `"email summary" is not a name a generated type can take`,
	}, {
		name: "name is not exported",
		src:  `{"emailSummary": {"type": "Email", "properties": ["id"]}}`,
		want: `"emailSummary" is not a name a generated type can take`,
	}, {
		name: "not an object",
		src:  `["EmailSummary"]`,
		want: "expected an object naming one set of properties per member, found an array",
	}, {
		name: "unknown member",
		src:  `{"S": {"type": "Email", "props": ["id"]}}`,
		want: `unknown field "props"`,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setsErr(t, tt.src); !strings.Contains(got, tt.want) {
				t.Errorf("error was:\n%s\nwant it to mention %q", got, tt.want)
			}
		})
	}
}

func TestPropertySetHeaderProperty(t *testing.T) {
	p := sets(t, `{"S": {"type": "Email", "properties": ["id", "header:List-Id:asText"]}}`)
	s, _ := p.Find("S")
	if got := strings.Join(s.Properties(), ","); got != "id,header:List-Id:asText" {
		t.Errorf("Properties() = %q, want the header property kept", got)
	}
}

// getWithSet asks for a named set instead of listing the properties.
const getWithSet = `{
  "methodCalls": [
    ["Email/get", {"ids": "{{ids}}", "properties": "@EmailSummary"}, "get"]
  ]
}`

func TestRequestAsksForPropertySet(t *testing.T) {
	q, err := parseWith(t, sets(t, emailSets), getWithSet)
	if err != nil {
		t.Fatalf("parsing:\n%v", err)
	}
	call := q.Calls[0]
	if call.PropertySet == nil || call.PropertySet.Name != "EmailSummary" {
		t.Fatalf("PropertySet = %v, want EmailSummary", call.PropertySet)
	}
	want := "id,threadId,subject,from,receivedAt"
	if got := strings.Join(call.Properties, ","); got != want {
		t.Errorf("Properties = %q, want the set spelled out, %q", got, want)
	}
	node, ok := call.Args.Find("properties")
	if !ok {
		t.Fatal("the properties argument is not in the request")
	}
	arr, ok := node.(*Array)
	if !ok {
		t.Fatalf("the properties argument is %T, want the array the set stands for", node)
	}
	if string(arr.Raw) != `["id","threadId","subject","from","receivedAt"]` {
		t.Errorf("the request sends %s, want the properties the set holds", arr.Raw)
	}
}

func TestRequestAsksForNestedPropertySet(t *testing.T) {
	props := sets(t, `{
	  "BodyPartSummary": {"type": "EmailBodyPart", "properties": ["partId", "blobId", "type"]}
	}`)
	q, err := parseWith(t, props, `{
	  "methodCalls": [
	    ["Email/get", {"ids": "{{ids}}", "properties": ["id", "textBody"],
	                   "bodyProperties": "@BodyPartSummary"}, "get"]
	  ]
	}`)
	if err != nil {
		t.Fatalf("parsing:\n%v", err)
	}
	call := q.Calls[0]
	if call.NestedPropertySet == nil || call.NestedPropertySet.Name != "BodyPartSummary" {
		t.Fatalf("NestedPropertySet = %v, want BodyPartSummary", call.NestedPropertySet)
	}
	if got := strings.Join(call.NestedProperties, ","); got != "partId,blobId,type" {
		t.Errorf("NestedProperties = %q, want the set spelled out", got)
	}
}

func TestRequestPropertySetErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "unknown set",
		src:  `{"methodCalls": [["Email/get", {"properties": "@EmailSummry"}, "get"]]}`,
		want: `no set of properties is named "EmailSummry"`,
	}, {
		name: "not a name",
		src:  `{"methodCalls": [["Email/get", {"properties": "@email summary"}, "get"]]}`,
		want: "is not the name of a set of properties",
	}, {
		name: "set of another type",
		src:  `{"methodCalls": [["Mailbox/get", {"properties": "@EmailSummary"}, "get"]]}`,
		want: "properties of Mailbox",
	}, {
		name: "set with a nested narrowing",
		src: `{"methodCalls": [["Email/get", {"properties": "@EmailWithBody",
		       "bodyProperties": ["partId"]}, "get"]]}`,
		want: "narrows bodyProperties as well",
	}, {
		name: "a set is not a value elsewhere",
		src:  `{"methodCalls": [["Email/get", {"ids": ["@EmailSummary"], "properties": ["id"]}, "get"]]}`,
		want: "@EmailSummary",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseWith(t, sets(t, emailSets), tt.src)
			if err == nil {
				t.Fatal("expected an error, got none")
			}
			if got := err.Error(); !strings.Contains(got, tt.want) {
				t.Errorf("error was:\n%s\nwant it to mention %q", got, tt.want)
			}
		})
	}
}

func TestRequestAsksForSetWithoutAnyDeclared(t *testing.T) {
	_, err := parseWith(t, nil, getWithSet)
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	if got := err.Error(); !strings.Contains(got, PropertiesName) {
		t.Errorf("error was:\n%s\nwant it to say where sets are declared", got)
	}
}
