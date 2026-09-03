package spec

import (
	"strings"
	"testing"
)

func TestRustTypeName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Email", "Email"},
		{"EmailSubmission", "EmailSubmission"},
		{"UTCDate", "UtcDate"},
		{"MDN", "Mdn"},
		{"SMIMEStatus", "SmimeStatus"},
		{"ListInboxEmails", "ListInboxEmails"},
		{"ListInboxEmailsEmail2", "ListInboxEmailsEmail2"},
		{"TimeZoneId", "TimeZoneId"},
		{"EmailIDs", "EmailIds"},
	}
	for _, tt := range tests {
		if got := RustTypeName(tt.in); got != tt.want {
			t.Errorf("RustTypeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRustFieldName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"accountId", "account_id"},
		{"receivedAt", "received_at"},
		{"htmlBody", "html_body"},
		{"@type", "r#type"},
		{"type", "r#type"},
		{"ref", "r#ref"},
		{"self", "self_"},
		{"header:List-Id:asText", "header_list_id_as_text"},
		{"data:asBase64", "data_as_base_64"},
		{"MailboxID", "mailbox_id"},
		{"emailIds", "email_ids"},
		{"header:List-Post:asURLs", "header_list_post_as_urls"},
	}
	for _, tt := range tests {
		if got := RustFieldName(tt.in); got != tt.want {
			t.Errorf("RustFieldName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestSerdeRename checks that a rename is written where, and only where, serde
// would not arrive at the name on the wire by itself. A rename left out where
// it was needed is a property that silently never round-trips.
func TestSerdeRename(t *testing.T) {
	tests := []struct{ wire, want string }{
		{"accountId", ""},
		{"id", ""},
		{"receivedAt", ""},
		{"htmlBody", ""},
		{"@type", "@type"},
		{"header:List-Id:asText", "header:List-Id:asText"},
		{"reportingUA", "reportingUA"},
		{"mayRSVP", "mayRSVP"},
		{"data:asBase64", "data:asBase64"},
	}
	for _, tt := range tests {
		if got := SerdeRename(tt.wire, RustFieldName(tt.wire)); got != tt.want {
			t.Errorf("SerdeRename(%q) = %q, want %q", tt.wire, got, tt.want)
		}
	}
}

// TestSerdeRenameCoversTheModel checks the rule against every property the
// data model has: whatever serde ends up with, from the identifier and any
// rename beside it, has to be the name the property goes by on the wire. A
// rename left out where one was needed is a property that silently never
// round-trips.
func TestSerdeRenameCoversTheModel(t *testing.T) {
	for _, o := range Standard().Objects() {
		for _, f := range o.Fields {
			ident := RustFieldName(f.Name)
			if ident == "" || ident == "_" {
				t.Errorf("%s.%s has no Rust name", o.Name, f.Name)
				continue
			}
			wire := SerdeRename(f.Name, ident)
			if wire == "" {
				wire = camelCase(strings.TrimPrefix(ident, "r#"))
			}
			if wire != f.Name {
				t.Errorf("%s.%s is written as %s, which serde sends as %q", o.Name, f.Name, ident, wire)
			}
		}
	}
}

// camelCase is serde's rename_all = "camelCase" rule, written out again here so
// that the generator is checked against something other than itself.
func camelCase(ident string) string {
	var b strings.Builder
	for i, word := range strings.Split(ident, "_") {
		if word == "" {
			continue
		}
		if i == 0 {
			b.WriteString(word)
			continue
		}
		b.WriteString(strings.ToUpper(word[:1]) + word[1:])
	}
	return b.String()
}

func TestRustType(t *testing.T) {
	tests := []struct{ in, want string }{
		{"String", "String"},
		{"String|null", "Option<String>"},
		{"Id[]", "Vec<Id>"},
		{"Id[]|null", "Option<Vec<Id>>"},
		{"String[SetError]|null", "Option<BTreeMap<String, SetError>>"},
		{"Id[Email|null]|null", "Option<BTreeMap<Id, Option<Email>>>"},
		{"UnsignedInt", "u64"},
		{"Any[]", "Vec<serde_json::Value>"},
		{"FilterOperator|EmailFilterCondition|null", "Option<FilterOperatorOrEmailFilterCondition>"},
		{"(Date|null)[]", "Vec<Option<Date>>"},
		{"UTCDate", "UtcDate"},
	}
	for _, tt := range tests {
		if got := MustParseType(tt.in).RustType(); got != tt.want {
			t.Errorf("RustType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRustTypeNamesAreUnique checks that no two types of the data model come
// out under one Rust name. Rust writes an initialism as a word, so an MDN and
// an Mdn would collide where Go and TypeScript keep them apart.
func TestRustTypeNamesAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, o := range Standard().Objects() {
		name := RustTypeName(o.Name)
		if other, taken := seen[name]; taken {
			t.Errorf("%s and %s are both %s in Rust", other, o.Name, name)
		}
		seen[name] = o.Name
	}
	for _, alias := range RustPrimitiveAliases() {
		if other, taken := seen[alias.Name]; taken {
			t.Errorf("the %s alias collides with the %s type", alias.Name, other)
		}
		seen[alias.Name] = alias.Name
	}
}
