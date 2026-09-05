package shared

import (
	"strings"
	"testing"
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
// keeps two queries in one package from declaring the same type.
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
// other than Unique is honoured, since the plan reserves the query names first.
func TestUniqueSkipsNamesTakenElsewhere(t *testing.T) {
	taken := map[string]bool{"Agenda": true, "Agenda2": true}
	if got := Unique(taken, "Agenda"); got != "Agenda3" {
		t.Errorf("Unique = %q, want %q", got, "Agenda3")
	}
}

// TestRecordProperties checks that a record type carries the id whether or not
// the query asked for it, because a /get response always returns it.
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecordProperties(tt.in)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("RecordProperties(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestRecordPropertiesLeavesItsInputAlone checks that the properties the query
// holds are not rewritten, since the generator reads them again for the
// TypeScript pass.
func TestRecordPropertiesLeavesItsInputAlone(t *testing.T) {
	props := []string{"subject", "from"}
	RecordProperties(props)
	if strings.Join(props, ",") != "subject,from" {
		t.Errorf("the input became %v", props)
	}
}

// TestPrimaryAccountPhrase checks what a generated function says about the
// account it is sent to. A session has a primary account for each capability
// rather than one for everything, so the capability is named: two queries in
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
		want: "The query does not say which account to use, so the session's primary account for " +
			"urn:ietf:params:jmap:mail is used, which costs a session lookup on first use.",
	}, {
		name:         "two",
		capabilities: []string{"urn:ietf:params:jmap:blob", "urn:ietf:params:jmap:mail"},
		want: "The query does not say which account to use, so the session's primary account is used for each of " +
			"urn:ietf:params:jmap:blob and urn:ietf:params:jmap:mail, which costs a session lookup on first use. " +
			"They need not be the same account.",
	}, {
		name: "three",
		capabilities: []string{
			"urn:ietf:params:jmap:blob", "urn:ietf:params:jmap:mail", "urn:ietf:params:jmap:submission",
		},
		want: "The query does not say which account to use, so the session's primary account is used for each of " +
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
