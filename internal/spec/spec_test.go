package spec

import (
	"strings"
	"testing"
)

// TestStandardCatalogueResolves checks that every type expression in the
// catalogue parses and that every name it mentions is registered. A typo in the
// catalogue would otherwise only surface as a confusing generator error.
func TestStandardCatalogueResolves(t *testing.T) {
	s := Standard()
	for _, o := range s.Objects() {
		for _, f := range o.Fields {
			ty, err := ParseType(f.Type)
			if err != nil {
				t.Errorf("%s.%s: %v", o.Name, f.Name, err)
				continue
			}
			checkNames(t, s, ty, o.Name+"."+f.Name)
		}
	}
	for _, m := range s.Methods() {
		if _, err := s.ArgumentsOf(m.Name); err != nil {
			t.Errorf("%s: %v", m.Name, err)
		}
		if _, err := s.ResponseOf(m.Name); err != nil {
			t.Errorf("%s: %v", m.Name, err)
		}
		if m.Capability == "" {
			t.Errorf("%s: no capability", m.Name)
		}
	}
}

// checkNames walks a type expression and reports any object name that the
// catalogue does not define.
func checkNames(t *testing.T, s *Spec, ty *Type, where string) {
	t.Helper()
	switch {
	case ty.IsArray():
		checkNames(t, s, ty.Elem, where)
	case ty.IsMap():
		checkNames(t, s, ty.Key, where)
		checkNames(t, s, ty.Value, where)
	case ty.IsUnion():
		for _, m := range ty.Union {
			checkNames(t, s, m, where)
		}
	case ty.IsObject():
		if _, ok := s.Object(ty.Name); !ok {
			t.Errorf("%s: unknown type %q", where, ty.Name)
		}
	}
}

func TestParseType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"String", "String"},
		{"Id[]", "Id[]"},
		{"Id[]|null", "Id[]|null"},
		{"String[Boolean]", "String[Boolean]"},
		{"Id[Boolean]", "Id[Boolean]"},
		{"String[Email|null]|null", "String[Email|null]|null"},
		{"EmailBodyPart[]|null", "EmailBodyPart[]|null"},
		{"FilterOperator|EmailFilterCondition|null", "FilterOperator|EmailFilterCondition|null"},
		{"String[SetError]|null", "String[SetError]|null"},
	}
	for _, tt := range tests {
		got, err := ParseType(tt.in)
		if err != nil {
			t.Errorf("ParseType(%q): %v", tt.in, err)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("ParseType(%q) = %q, want %q", tt.in, got.String(), tt.want)
		}
	}
}

func TestParseTypeErrors(t *testing.T) {
	for _, in := range []string{"", "Foo[", "null", "Foo]["} {
		if _, err := ParseType(in); err == nil {
			t.Errorf("ParseType(%q) succeeded, want an error", in)
		}
	}
}

func TestResolvePath(t *testing.T) {
	s := Standard()
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{"Email/query", "/ids", "Id[]"},
		{"Email/query", "/ids/*", "Id[]"},
		{"Email/get", "/list", "Email[]"},
		{"Email/get", "/list/*/id", "Id[]"},
		{"Email/get", "/list/*/threadId", "Id[]"},
		{"Email/get", "/list/0/subject", "String|null"},
		{"Email/get", "/state", "String"},
		{"Mailbox/query", "/ids", "Id[]"},
		{"Thread/get", "/list/*/emailIds", "Id[]"},
		{"Email/changes", "/created", "Id[]"},
		{"Email/set", "/created", "Id[Email|null]|null"},
		{"Email/get", "/list/*/mailboxIds", "Id[Boolean][]"},
	}
	for _, tt := range tests {
		got, err := s.ResolvePath(tt.method, tt.path)
		if err != nil {
			t.Errorf("ResolvePath(%q, %q): %v", tt.method, tt.path, err)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("ResolvePath(%q, %q) = %q, want %q", tt.method, tt.path, got.String(), tt.want)
		}
	}
}

func TestResolvePathErrors(t *testing.T) {
	s := Standard()
	tests := []struct {
		method  string
		path    string
		wantSub string
	}{
		{"Email/query", "/nope", "has no property"},
		{"Email/query", "ids", "must start with"},
		{"Email/get", "/list/*/subject/x", "cannot select"},
		{"Email/query", "/ids/x", "neither a number"},
		{"Nope/get", "/ids", "unknown method"},
	}
	for _, tt := range tests {
		_, err := s.ResolvePath(tt.method, tt.path)
		if err == nil {
			t.Errorf("ResolvePath(%q, %q) succeeded, want an error", tt.method, tt.path)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantSub) {
			t.Errorf("ResolvePath(%q, %q) error = %q, want it to mention %q", tt.method, tt.path, err, tt.wantSub)
		}
	}
}

func TestExportedName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"id", "ID"},
		{"ids", "IDs"},
		{"accountId", "AccountID"},
		{"emailIds", "EmailIDs"},
		{"mailboxIds", "MailboxIDs"},
		{"htmlBody", "HTMLBody"},
		{"fetchHTMLBodyValues", "FetchHTMLBodyValues"},
		{"apiUrl", "APIURL"},
		{"notFound", "NotFound"},
		{"inReplyTo", "InReplyTo"},
		{"messageId", "MessageID"},
		{"isSubscribed", "IsSubscribed"},
		{"maxBodyValueBytes", "MaxBodyValueBytes"},
		{"cc", "Cc"},
		{"bcc", "Bcc"},
		{"subParts", "SubParts"},
		{"queryState", "QueryState"},
		{"sinceQueryState", "SinceQueryState"},
		{"upToId", "UpToID"},
		{"get", "Get"},
		{"queryChanges", "QueryChanges"},
	}
	for _, tt := range tests {
		if got := ExportedName(tt.in); got != tt.want {
			t.Errorf("ExportedName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUnexportedName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"accountId", "accountID"},
		{"id", "id"},
		{"type", "type_"},
		{"htmlBody", "htmlBody"},
	}
	for _, tt := range tests {
		if got := UnexportedName(tt.in); got != tt.want {
			t.Errorf("UnexportedName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMethodGoName(t *testing.T) {
	s := Standard()
	tests := []struct{ method, want string }{
		{"Email/get", "EmailGet"},
		{"Email/queryChanges", "EmailQueryChanges"},
		{"Mailbox/set", "MailboxSet"},
	}
	for _, tt := range tests {
		m, ok := s.Method(tt.method)
		if !ok {
			t.Errorf("method %q is not registered", tt.method)
			continue
		}
		if got := m.GoName(); got != tt.want {
			t.Errorf("%q GoName = %q, want %q", tt.method, got, tt.want)
		}
	}
}
