package jmapc

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Capability URIs defined by the core specification and by JMAP for Mail.
const (
	CapabilityCore       = "urn:ietf:params:jmap:core"
	CapabilityMail       = "urn:ietf:params:jmap:mail"
	CapabilitySubmission = "urn:ietf:params:jmap:submission"
	CapabilityVacation   = "urn:ietf:params:jmap:vacationresponse"
	CapabilityContacts   = "urn:ietf:params:jmap:contacts"
	CapabilityCalendars  = "urn:ietf:params:jmap:calendars"
	// CapabilityCalendarsParse covers CalendarEvent/parse, which a server may
	// support without supporting the rest of the calendar model.
	CapabilityCalendarsParse = "urn:ietf:params:jmap:calendars:parse"
	// CapabilityAvailability covers Principal/getAvailability.
	CapabilityAvailability = "urn:ietf:params:jmap:principals:availability"
	CapabilityPrincipals   = "urn:ietf:params:jmap:principals"
	// CapabilitySMIMEVerify adds the S/MIME verification properties to an
	// Email; it defines no types or methods of its own.
	CapabilitySMIMEVerify = "urn:ietf:params:jmap:smimeverify"
	// CapabilityBlob brings blob creation, reading and lookup into the API,
	// alongside the upload and download endpoints of the core specification.
	CapabilityBlob = "urn:ietf:params:jmap:blob"
	// CapabilityPrincipalsOwner appears only in an account's capabilities,
	// where it names the principal that owns the account.
	CapabilityPrincipalsOwner = "urn:ietf:params:jmap:principals:owner"
)

// Session is the Session object described in RFC 8620, Section 2. It tells the
// client where to send requests and what the server supports.
type Session struct {
	// Capabilities lists the capabilities the server supports, keyed by URI.
	Capabilities map[string]json.RawMessage `json:"capabilities"`
	// Accounts lists the accounts the authenticated user has access to.
	Accounts map[ID]*Account `json:"accounts"`
	// PrimaryAccounts maps a capability URI to the id of the account that
	// should be used for it by default.
	PrimaryAccounts map[string]ID `json:"primaryAccounts"`
	// Username identifies the authenticated user.
	Username string `json:"username"`
	// APIURL is the endpoint that JMAP requests are POSTed to.
	APIURL string `json:"apiUrl"`
	// DownloadURL is a URI template for downloading blobs.
	DownloadURL string `json:"downloadUrl"`
	// UploadURL is a URI template for uploading blobs.
	UploadURL string `json:"uploadUrl"`
	// EventSourceURL is a URI template for the push event source.
	EventSourceURL string `json:"eventSourceUrl"`
	// State changes whenever any other member of the Session object changes.
	State string `json:"state"`
}

// Account describes one account exposed by the session.
type Account struct {
	// Name is a user-facing label for the account.
	Name string `json:"name"`
	// IsPersonal reports whether the account belongs to the authenticated user
	// rather than being shared with them.
	IsPersonal bool `json:"isPersonal"`
	// IsReadOnly reports whether the user may only read from this account.
	IsReadOnly bool `json:"isReadOnly"`
	// AccountCapabilities gives per-account limits, keyed by capability URI.
	AccountCapabilities map[string]json.RawMessage `json:"accountCapabilities"`
}

// CoreCapability holds the server limits advertised under the core capability
// URI, as defined in RFC 8620, Section 2.
type CoreCapability struct {
	MaxSizeUpload         UnsignedInt `json:"maxSizeUpload"`
	MaxConcurrentUpload   UnsignedInt `json:"maxConcurrentUpload"`
	MaxSizeRequest        UnsignedInt `json:"maxSizeRequest"`
	MaxConcurrentRequests UnsignedInt `json:"maxConcurrentRequests"`
	MaxCallsInRequest     UnsignedInt `json:"maxCallsInRequest"`
	MaxObjectsInGet       UnsignedInt `json:"maxObjectsInGet"`
	MaxObjectsInSet       UnsignedInt `json:"maxObjectsInSet"`
	CollationAlgorithms   []string    `json:"collationAlgorithms"`
}

// Core returns the server's core capability limits.
func (s *Session) Core() (*CoreCapability, error) {
	raw, ok := s.Capabilities[CapabilityCore]
	if !ok {
		return nil, fmt.Errorf("jmapc: session does not advertise %s", CapabilityCore)
	}
	var c CoreCapability
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("jmapc: decoding core capability: %w", err)
	}
	return &c, nil
}

// HasCapability reports whether the server advertises the given capability URI.
func (s *Session) HasCapability(uri string) bool {
	_, ok := s.Capabilities[uri]
	return ok
}

// PrimaryAccountID returns the id of the account to use by default for the
// given capability. Generated code calls this when a query leaves accountId
// unset.
func (s *Session) PrimaryAccountID(capability string) (ID, error) {
	if id, ok := s.PrimaryAccounts[capability]; ok {
		return id, nil
	}
	if !s.HasCapability(capability) {
		return "", fmt.Errorf("jmapc: server does not support %s", capability)
	}
	return "", fmt.Errorf("jmapc: session has no primary account for %s", capability)
}

// WellKnownURL returns the session resource URL to start autodiscovery from,
// as described in RFC 8620, Section 2.2. The host may be given bare
// ("example.com") or as a URL ("https://example.com").
func WellKnownURL(host string) string {
	host = strings.TrimSuffix(host, "/")
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Host == "" {
		return host + "/.well-known/jmap"
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/.well-known/jmap"
	return u.String()
}
