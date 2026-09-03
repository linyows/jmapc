package shared

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/spec"
)

// TestJoinMethods checks the list of method names a generated function's
// documentation opens with, which reads as prose rather than as a list.
func TestJoinMethods(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, "no calls"},
		{[]string{"Email/get"}, "one Email/get call"},
		{[]string{"Email/query", "Email/get"}, "Email/query and Email/get calls"},
		{
			[]string{"Blob/upload", "Email/set", "EmailSubmission/set"},
			"Blob/upload, Email/set, and EmailSubmission/set calls",
		},
	}
	for _, tt := range tests {
		if got := JoinMethods(tt.in); got != tt.want {
			t.Errorf("JoinMethods(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRoundTripPhrase checks what the documentation claims batching buys, which
// is nothing at all for a query making one call.
func TestRoundTripPhrase(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "the server is asked once"},
		{1, "the server is asked once"},
		{2, "2 dependent calls cost one round trip"},
		{5, "5 dependent calls cost one round trip"},
	}
	for _, tt := range tests {
		if got := RoundTripPhrase(tt.in); got != tt.want {
			t.Errorf("RoundTripPhrase(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestHeaderPropertyDoc checks that a field generated for a header property
// says which header field it holds and in what form, since its Go type says
// neither.
func TestHeaderPropertyDoc(t *testing.T) {
	tests := []struct {
		name string
		in   *spec.HeaderProperty
		want string
	}{
		{
			name: "the raw form",
			in:   &spec.HeaderProperty{Name: "List-Id"},
			want: "The List-Id header field, or the last of them where the message has several, as it appears in the message.",
		},
		{
			name: "asText",
			in:   &spec.HeaderProperty{Name: "List-Id", Form: "asText"},
			want: "The List-Id header field, or the last of them where the message has several, decoded and unfolded into text.",
		},
		{
			name: "asAddresses",
			in:   &spec.HeaderProperty{Name: "To", Form: "asAddresses"},
			want: "The To header field, or the last of them where the message has several, parsed as a list of addresses.",
		},
		{
			name: "every instance",
			in:   &spec.HeaderProperty{Name: "Received", Form: "asText", All: true},
			want: "Every Received header field in the message, in the order they appear, decoded and unfolded into text.",
		},
		{
			name: "a form the catalogue does not describe",
			in:   &spec.HeaderProperty{Name: "X-Spam", Form: "asSomethingElse"},
			want: "The X-Spam header field, or the last of them where the message has several.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HeaderPropertyDoc(tt.in); got != tt.want {
				t.Errorf("HeaderPropertyDoc = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHeaderPropertyDocCoversEveryForm checks that no parsed form the catalogue
// accepts falls through to the bare sentence, which would leave a generated
// field documented as if it held the raw header.
func TestHeaderPropertyDocCoversEveryForm(t *testing.T) {
	for _, form := range []string{"asText", "asAddresses", "asGroupedAddresses", "asMessageIds", "asDate", "asURLs"} {
		doc := HeaderPropertyDoc(&spec.HeaderProperty{Name: "To", Form: form})
		if !strings.Contains(doc, ", ") || strings.HasSuffix(doc, "several.") {
			t.Errorf("%s is documented as %q, which says nothing about the form", form, doc)
		}
	}
}

// TestDynamicPropertyDoc checks the documentation for a property the data model
// does not describe, whose generated field is raw JSON and so says nothing for
// itself.
func TestDynamicPropertyDoc(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a header property the parser could not read",
			in:   "header:To:asAddresses:all:extra",
			want: "The header:To:asAddresses:all:extra header field, in the form the query asked for.",
		},
		{
			name: "a blob digest",
			in:   "digest:sha-256",
			want: "The digest of the blob under the sha-256 algorithm, as base64.",
		},
		{
			name: "the blob's octets",
			in:   "data",
			want: "The blob's octets. The server returns them under data:asText or data:asBase64, whichever suits what they hold, so this property itself does not come back.",
		},
		{
			name: "anything else",
			in:   "myExtension",
			want: "The myExtension property, whose meaning the server decides.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DynamicPropertyDoc(tt.in); got != tt.want {
				t.Errorf("DynamicPropertyDoc(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
