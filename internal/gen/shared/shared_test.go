package shared

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// TestWriteComment checks the wrapping both generators depend on: a comment
// wraps at column 78 counting its indent and the "// " prefix, so that a
// generated file reads the way handwritten source does.
func TestWriteComment(t *testing.T) {
	tests := []struct {
		name   string
		indent string
		text   string
		want   string
	}{
		{
			name: "empty text writes nothing",
			text: "   \n  ",
			want: "",
		},
		{
			name: "short text is one line",
			text: "The Subject header field value.",
			want: "// The Subject header field value.\n",
		},
		{
			name: "a paragraph wraps at the width",
			text: "The mailboxes the email is in, as a set of ids mapped to true, which is how JMAP spells a set.",
			want: "// The mailboxes the email is in, as a set of ids mapped to true, which is how\n" +
				"// JMAP spells a set.\n",
		},
		{
			name:   "the indent counts against the width",
			indent: "\t",
			text:   "The mailboxes the email is in, as a set of ids mapped to true, which is how JMAP spells a set.",
			want: "\t// The mailboxes the email is in, as a set of ids mapped to true, which is\n" +
				"\t// how JMAP spells a set.\n",
		},
		{
			name: "a blank line becomes a bare comment marker",
			text: "It makes one Email/get call.\n\nIt returns the response.",
			want: "// It makes one Email/get call.\n//\n// It returns the response.\n",
		},
		{
			name: "a word longer than the width is left whole",
			text: "See urn:ietf:params:jmap:principals:availability:and:then:some:more:words:still",
			want: "// See\n// urn:ietf:params:jmap:principals:availability:and:then:some:more:words:still\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			WriteComment(&buf, tt.indent, tt.text)
			if buf.String() != tt.want {
				t.Errorf("WriteComment(%q, %q) =\n%q\nwant\n%q", tt.indent, tt.text, buf.String(), tt.want)
			}
		})
	}
}

// TestWriteCommentKeepsRoomAtDeepIndents checks the floor under the width: an
// indent past column 78 leaves nothing to write in, and without the floor every
// word would land on a line of its own.
func TestWriteCommentKeepsRoomAtDeepIndents(t *testing.T) {
	indent := strings.Repeat("\t", 80)
	var buf strings.Builder
	WriteComment(&buf, indent, "Find the ids of the matching emails.")
	want := indent + "// Find the ids of the\n" + indent + "// matching emails.\n"
	if buf.String() != want {
		t.Errorf("WriteComment at an indent of 80 =\n%q\nwant\n%q", buf.String(), want)
	}
}

// TestUnique checks that a name taken twice comes back numbered, which is what
// keeps two requests in one package from declaring the same type.
func TestUnique(t *testing.T) {
	taken := make(map[string]bool)
	want := []string{
		"SearchEmailsEmail",
		"SearchEmailsEmail2",
		"SearchEmailsEmail3",
	}
	for i, w := range want {
		if got := Unique(taken, "SearchEmailsEmail"); got != w {
			t.Errorf("call %d = %q, want %q", i+1, got, w)
		}
	}
	if got := Unique(taken, "SearchEmailsParams"); got != "SearchEmailsParams" {
		t.Errorf("an untaken name = %q, want it unchanged", got)
	}
}

// TestUniqueSkipsNamesTakenElsewhere checks that a name claimed by something
// other than Unique is honoured, since the plan reserves the request names first.
func TestUniqueSkipsNamesTakenElsewhere(t *testing.T) {
	taken := map[string]bool{"Agenda": true, "Agenda2": true}
	if got := Unique(taken, "Agenda"); got != "Agenda3" {
		t.Errorf("Unique = %q, want %q", got, "Agenda3")
	}
}

// TestRecordProperties checks that a record type carries the id whether or not
// the request asked for it, because a /get response always returns it.
func TestRecordProperties(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"id is added at the front", []string{"subject", "from"}, []string{"id", "subject", "from"}},
		{"id already asked for", []string{"id", "subject"}, []string{"id", "subject"}},
		{"id asked for last stays where it is", []string{"subject", "id"}, []string{"subject", "id"}},
		{"nothing asked for", nil, []string{"id"}},
	}
	email, _ := spec.Standard().Object("Email")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecordProperties(email, tt.in, true)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("RecordProperties(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestRecordPropertiesLeavesItsInputAlone checks that the properties the request
// holds are not rewritten, since the generator reads them again for the
// TypeScript pass.
func TestRecordPropertiesLeavesItsInputAlone(t *testing.T) {
	props := []string{"subject", "from"}
	email, _ := spec.Standard().Object("Email")
	RecordProperties(email, props, true)
	if strings.Join(props, ",") != "subject,from" {
		t.Errorf("the input became %v", props)
	}
}

// TestPrimaryAccountPhrase checks what a generated function says about the
// account it is sent to. A session has a primary account for each capability
// rather than one for everything, so the capability is named: two requests in
// one package may be talking to two different accounts, and this is the only
// place that shows.
func TestPrimaryAccountPhrase(t *testing.T) {
	tests := []struct {
		name         string
		capabilities []string
		want         string
	}{{
		name:         "none",
		capabilities: nil,
		want:         "",
	}, {
		name:         "one",
		capabilities: []string{"urn:ietf:params:jmap:mail"},
		want: "The request does not say which account to use, so the session's primary account for " +
			"urn:ietf:params:jmap:mail is used, which costs a session lookup on first use.",
	}, {
		name:         "two",
		capabilities: []string{"urn:ietf:params:jmap:blob", "urn:ietf:params:jmap:mail"},
		want: "The request does not say which account to use, so the session's primary account is used for each of " +
			"urn:ietf:params:jmap:blob and urn:ietf:params:jmap:mail, which costs a session lookup on first use. " +
			"They need not be the same account.",
	}, {
		name: "three",
		capabilities: []string{
			"urn:ietf:params:jmap:blob", "urn:ietf:params:jmap:mail", "urn:ietf:params:jmap:submission",
		},
		want: "The request does not say which account to use, so the session's primary account is used for each of " +
			"urn:ietf:params:jmap:blob, urn:ietf:params:jmap:mail, and urn:ietf:params:jmap:submission, " +
			"which costs a session lookup on first use. They need not be the same account.",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PrimaryAccountPhrase(tt.capabilities); got != tt.want {
				t.Errorf("PrimaryAccountPhrase() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestSameNarrowingTellsNoListFromAnEmptyOne checks two calls that read the
// same records in different shapes: one asks for no properties of the email,
// which is its id alone, and the other for every property of it and none of
// its body parts. Neither list being given is not the same as one being empty.
func TestSameNarrowingTellsNoListFromAnEmptyOne(t *testing.T) {
	q, err := request.NewParser(spec.Standard()).Parse("Shapes"+request.Extension, []byte(`{"methodCalls": [
	  ["Email/get", {"ids": ["e1"], "properties": []}, "ids"],
	  ["Email/get", {"ids": ["e1"], "bodyProperties": []}, "bodies"]
	]}`))
	if err != nil {
		t.Fatalf("checking the request:\n%v", err)
	}
	same := SameNarrowing(q.Calls)
	if same[q.Calls[1]] == q.Calls[0] {
		t.Error("a call fetching every property was given the shape of one fetching the id alone")
	}
}

// TestRecordPropertiesWithoutAnID checks a method that does not return the id
// whatever it is asked for: the record holds what was asked for and no more.
func TestRecordPropertiesWithoutAnID(t *testing.T) {
	email, _ := spec.Standard().Object("Email")
	if got := RecordProperties(email, []string{"subject"}, false); strings.Join(got, ",") != "subject" {
		t.Errorf("RecordProperties = %v, want subject alone", got)
	}
}

// TestRecordPropertiesHoldTheFieldsDataComesBackAs checks a blob asked for its
// data: the server answers under data:asText or data:asBase64, so the record
// holds both, each picked by the server, and a field also asked for by its own
// name is held once and comes back whatever the server would pick.
func TestRecordPropertiesHoldTheFieldsDataComesBackAs(t *testing.T) {
	blob, _ := spec.Standard().Object("BlobData")
	tests := []struct {
		in, want, picked string
	}{
		{"data,size", "id,data:asText,data:asBase64,size", "data:asText,data:asBase64"},
		{"data:asText,data", "id,data:asText,data:asBase64", "data:asBase64"},
		{"data:asBase64", "id,data:asBase64", ""},
	}
	for _, tt := range tests {
		props := strings.Split(tt.in, ",")
		got := RecordProperties(blob, props, true)
		if strings.Join(got, ",") != tt.want {
			t.Errorf("RecordProperties(%v) = %v, want %s", props, got, tt.want)
		}
		var picked []string
		for _, name := range got {
			if PickedByServer(blob, props, name) {
				picked = append(picked, name)
			}
		}
		if strings.Join(picked, ",") != tt.picked {
			t.Errorf("asked for %v, the server picks %v, want %s", props, picked, tt.picked)
		}
	}
	field, _ := blob.Field("data:asText")
	if got := RecordFieldDoc(blob, []string{"data"}, field); !strings.HasSuffix(got,
		"The server sends this or data:asBase64, whichever suits the value, so it may be absent.") {
		t.Errorf("the doc of data:asText was %q", got)
	}
}

// TestAnExtendingSetDoesNotRedeclareWhatItInherits checks a set adding data to
// one that holds data:asText: the base already has that member, asked for by
// name and so required, and the derived set holds only data:asBase64, which the
// server picks.
func TestAnExtendingSetDoesNotRedeclareWhatItInherits(t *testing.T) {
	blob, _ := spec.Standard().Object("BlobData")
	base := &request.PropertySet{Name: "BlobText", Type: "BlobData", Own: []string{"data:asText"}}
	derived := &request.PropertySet{Name: "BlobContent", Type: "BlobData", Extends: base, Own: []string{"data"}}
	if got := strings.Join(SetProperties(base, blob), ","); got != "id,data:asText" {
		t.Errorf("the base holds %s, want id,data:asText", got)
	}
	if got := strings.Join(SetProperties(derived, blob), ","); got != "data:asBase64" {
		t.Errorf("the derived set holds %s, want data:asBase64", got)
	}

	// The other way round, the base has data:asText where the server picks
	// it, and the derived set asking for it by name declares it again, for
	// the server to send whatever it picks.
	base = &request.PropertySet{Name: "BlobContent", Type: "BlobData", Own: []string{"data"}}
	derived = &request.PropertySet{Name: "BlobText", Type: "BlobData", Extends: base, Own: []string{"data:asText"}}
	if got := strings.Join(SetProperties(derived, blob), ","); got != "data:asText" {
		t.Errorf("the derived set holds %s, want data:asText", got)
	}
	if PickedByServer(blob, derived.Own, "data:asText") {
		t.Error("the derived set leaves data:asText for the server to pick")
	}
}

