package jmapc

import (
	"net/url"
	"testing"
)

func TestResolveSessionURL(t *testing.T) {
	base, err := url.Parse("https://example.com/jmap/session")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"empty is left alone", "", ""},
		{"already absolute is left alone", "https://other.example.com/jmap", "https://other.example.com/jmap"},
		{"path-absolute resolves against the host", "/jmap", "https://example.com/jmap"},
		{
			"a URI template's braces survive resolution",
			"/jmap/download/{accountId}/{blobId}/{name}?accept={type}",
			"https://example.com/jmap/download/{accountId}/{blobId}/{name}?accept={type}",
		},
		{"protocol-relative keeps the base scheme", "//other.example.com/jmap", "https://other.example.com/jmap"},
		{"relative resolves against the session URL's directory", "api", "https://example.com/jmap/api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSessionURL(base, tt.ref); got != tt.want {
				t.Errorf("resolveSessionURL(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}
