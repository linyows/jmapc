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
	// CapabilityQuota reports the limits an account is under and how much of
	// each is used.
	CapabilityQuota = "urn:ietf:params:jmap:quota"
	// CapabilitySieve manages the filtering scripts the server runs on
	// incoming mail.
	CapabilitySieve = "urn:ietf:params:jmap:sieve"
	// CapabilityMDN sends and reads the receipts that report what became of a
	// message.
	CapabilityMDN = "urn:ietf:params:jmap:mdn"
	// CapabilityWebPushVAPID states that the server authenticates itself to a
	// push service with VAPID. It defines no types and no methods: the value
	// it carries is a key, held in the session.
	CapabilityWebPushVAPID = "urn:ietf:params:jmap:webpush-vapid"
	// CapabilityPrincipalsOwner appears only in an account's capabilities,
	// where it names the principal that owns the account.
	CapabilityPrincipalsOwner = "urn:ietf:params:jmap:principals:owner"
)

// Session is the Session object described in RFC 8620, Section 2. It states
// where to send requests and what the server supports.
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

// Capability decodes this account's entry for a capability into dest.
// Several capabilities state their per-account limits here rather than in the
// session: the largest script, the largest blob, how many of each are allowed.
func (a *Account) Capability(uri string, dest any) error {
	raw, ok := a.AccountCapabilities[uri]
	if !ok {
		return fmt.Errorf("jmapc: account does not support %s", uri)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("jmapc: decoding the %s capability of the account: %w", uri, err)
	}
	return nil
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

// Capability decodes the session's entry for a capability into dest.
//
// Not every capability defines types and methods. Some carry only a value for
// the client — a limit, a key, an identifier — and this is how that value is
// read, including for a capability jmapc does not know.
func (s *Session) Capability(uri string, dest any) error {
	raw, ok := s.Capabilities[uri]
	if !ok {
		return fmt.Errorf("jmapc: session does not advertise %s", uri)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("jmapc: decoding the %s capability: %w", uri, err)
	}
	return nil
}

// Core returns the server's core capability limits.
func (s *Session) Core() (*CoreCapability, error) {
	var c CoreCapability
	if err := s.Capability(CapabilityCore, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// WebPushVAPIDCapability holds what a server says about VAPID, as defined by
// RFC 9749.
type WebPushVAPIDCapability struct {
	// ApplicationServerKey is the ECDSA public key the push service will use
	// to check that a notification really came from this server, in
	// uncompressed form and base64url-encoded. A client passes it to the push
	// service when it subscribes there.
	ApplicationServerKey string `json:"applicationServerKey"`
}

// WebPushVAPID returns the key the server authenticates itself to a push
// service with. It changes when the server rotates its keys, which appears as
// a new sessionState.
func (s *Session) WebPushVAPID() (*WebPushVAPIDCapability, error) {
	var c WebPushVAPIDCapability
	if err := s.Capability(CapabilityWebPushVAPID, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// HasCapability reports whether the server advertises the given capability URI.
func (s *Session) HasCapability(uri string) bool {
	_, ok := s.Capabilities[uri]
	return ok
}

// resolveURLs makes apiUrl, downloadUrl, uploadUrl, and eventSourceUrl
// absolute, resolving each against base (the session URL) when the server
// sent it as a reference rather than a full URL. RFC 8620 does not say these
// have to be absolute, and a server that sends "/jmap" as its apiUrl
// otherwise turns into a request to that literal path, which fails with an
// error that does not mention the session at all.
func (s *Session) resolveURLs(base *url.URL) {
	s.APIURL = resolveSessionURL(base, s.APIURL)
	s.DownloadURL = resolveSessionURL(base, s.DownloadURL)
	s.UploadURL = resolveSessionURL(base, s.UploadURL)
	s.EventSourceURL = resolveSessionURL(base, s.EventSourceURL)
}

// resolveSessionURL resolves ref against base per RFC 3986, Section 5. It
// builds the result by concatenation rather than by round-tripping ref
// through url.URL, because downloadUrl and uploadUrl are URI templates
// carrying literal "{" and "}" that url.URL.String would percent-encode.
func resolveSessionURL(base *url.URL, ref string) string {
	if ref == "" {
		return ref
	}
	if u, err := url.Parse(ref); err == nil && u.IsAbs() {
		return ref
	}
	if strings.HasPrefix(ref, "//") {
		return base.Scheme + ":" + ref
	}
	if strings.HasPrefix(ref, "/") {
		return base.Scheme + "://" + base.Host + ref
	}
	dir := base.Path
	if i := strings.LastIndexByte(dir, '/'); i >= 0 {
		dir = dir[:i+1]
	} else {
		dir = "/"
	}
	return base.Scheme + "://" + base.Host + dir + ref
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
