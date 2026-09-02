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
		{"smimeStatus", "SMIMEStatus"},
		{"smimeVerifiedAt", "SMIMEVerifiedAt"},
		{"hasVerifiedSmime", "HasVerifiedSMIME"},
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

// TestStandardCatalogueCovers pins down which methods the catalogue knows, so
// that one going missing is a test failure rather than a puzzling "unknown
// method" from the generator.
func TestStandardCatalogueCovers(t *testing.T) {
	want := []string{
		"AddressBook/changes", "AddressBook/get", "AddressBook/set",
		"Blob/copy", "Blob/get", "Blob/lookup", "Blob/upload",
		"Calendar/changes", "Calendar/get", "Calendar/set",
		"CalendarEvent/changes", "CalendarEvent/copy", "CalendarEvent/get",
		"CalendarEvent/parse", "CalendarEvent/query", "CalendarEvent/queryChanges",
		"CalendarEvent/set",
		"CalendarEventNotification/changes", "CalendarEventNotification/get",
		"CalendarEventNotification/query", "CalendarEventNotification/queryChanges",
		"CalendarEventNotification/set",
		"ContactCard/changes", "ContactCard/copy", "ContactCard/get",
		"ContactCard/query", "ContactCard/queryChanges", "ContactCard/set",
		"Core/echo",
		"Email/changes", "Email/copy", "Email/get", "Email/import", "Email/parse",
		"Email/query", "Email/queryChanges", "Email/set",
		"EmailSubmission/changes", "EmailSubmission/get", "EmailSubmission/query",
		"EmailSubmission/queryChanges", "EmailSubmission/set",
		"Identity/changes", "Identity/get", "Identity/set",
		"MDN/parse", "MDN/send",
		"Mailbox/changes", "Mailbox/get", "Mailbox/query", "Mailbox/queryChanges", "Mailbox/set",
		"ParticipantIdentity/changes", "ParticipantIdentity/get", "ParticipantIdentity/set",
		"Principal/changes", "Principal/get", "Principal/getAvailability",
		"Principal/query", "Principal/queryChanges", "Principal/set",
		"Quota/changes", "Quota/get", "Quota/query", "Quota/queryChanges",
		"SearchSnippet/get",
		"ShareNotification/changes", "ShareNotification/get", "ShareNotification/query",
		"ShareNotification/queryChanges", "ShareNotification/set",
		"SieveScript/get", "SieveScript/query", "SieveScript/set", "SieveScript/validate",
		"Thread/changes", "Thread/get",
		"VacationResponse/get", "VacationResponse/set",
	}
	got := Standard().MethodNames()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the catalogue holds:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestCapabilitiesAreDeclared checks that each method is tied to the capability
// a request has to declare in order to call it.
func TestCapabilitiesAreDeclared(t *testing.T) {
	s := Standard()
	want := map[string]string{
		"Core/echo":            CapabilityCore,
		"Blob/copy":            CapabilityCore,
		"Email/get":            CapabilityMail,
		"SearchSnippet/get":    CapabilityMail,
		"Identity/get":         CapabilitySubmission,
		"EmailSubmission/set":  CapabilitySubmission,
		"VacationResponse/set": CapabilityVacation,
	}
	for name, capability := range want {
		m, ok := s.Method(name)
		if !ok {
			t.Errorf("method %q is not registered", name)
			continue
		}
		if m.Capability != capability {
			t.Errorf("%s needs %q, want %q", name, m.Capability, capability)
		}
	}
}

// TestEchoTakesAnything checks that Core/echo is registered with no declared
// arguments, which is what lets a query pass it whatever it likes.
func TestEchoTakesAnything(t *testing.T) {
	s := Standard()
	args, err := s.ArgumentsOf("Core/echo")
	if err != nil {
		t.Fatalf("Core/echo: %v", err)
	}
	if len(args.Fields) != 0 {
		t.Errorf("Core/echo declares %d arguments, want none", len(args.Fields))
	}
}

// TestEnumValuesAppearInDocs checks that every value a property is allowed to
// take is also named in that property's documentation. The two are written
// separately — one for the checker, one for the reader — and nothing else would
// notice them drifting apart.
func TestEnumValuesAppearInDocs(t *testing.T) {
	s := Standard()
	for _, o := range s.Objects() {
		for _, f := range o.Fields {
			for _, value := range f.Enum {
				if !strings.Contains(f.Doc, `"`+value+`"`) {
					t.Errorf("%s.%s allows %q, but its documentation does not mention it:\n\t%s",
						o.Name, f.Name, value, f.Doc)
				}
			}
		}
	}
}

// TestEnumsAreOnStringProperties checks that a fixed set of values is only
// attached where it can be checked: a string, or a set keyed by strings.
func TestEnumsAreOnStringProperties(t *testing.T) {
	s := Standard()
	for _, o := range s.Objects() {
		for _, f := range o.Fields {
			if len(f.Enum) == 0 {
				continue
			}
			ty := f.ParsedType()
			ok := ty.Name == String || (ty.IsMap() && ty.Key != nil && ty.Key.Name == String)
			if !ok {
				t.Errorf("%s.%s has a fixed set of values but is a %s, where nothing would check them",
					o.Name, f.Name, f.Type)
			}
		}
	}
}
