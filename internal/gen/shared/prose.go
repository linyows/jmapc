package shared

import (
	"fmt"
	"strings"

	"github.com/linyows/jmapc/internal/spec"
)

// JoinMethods renders a list of method names for prose.
func JoinMethods(names []string) string {
	switch len(names) {
	case 0:
		return "no calls"
	case 1:
		return "one " + names[0] + " call"
	case 2:
		return names[0] + " and " + names[1] + " calls"
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1] + " calls"
	}
}

// RoundTripPhrase describes what batching the calls buys.
func RoundTripPhrase(n int) string {
	if n <= 1 {
		return "the server is asked once"
	}
	return fmt.Sprintf("%d dependent calls cost one round trip", n)
}

// HeaderPropertyDoc describes a property naming one header field of a message.
func HeaderPropertyDoc(h *spec.HeaderProperty) string {
	which := "The " + h.Name + " header field"
	if h.All {
		which = "Every " + h.Name + " header field in the message, in the order they appear"
	} else {
		which += ", or the last of them where the message has several"
	}
	switch h.Form {
	case "", "asRaw":
		return which + ", as it appears in the message."
	case "asText":
		return which + ", decoded and unfolded into text."
	case "asAddresses":
		return which + ", parsed as a list of addresses."
	case "asGroupedAddresses":
		return which + ", parsed as a list of addresses, keeping the groups they were written in."
	case "asMessageIds":
		return which + ", parsed as message ids, without their angle brackets."
	case "asDate":
		return which + ", parsed as a date."
	case "asURLs":
		return which + ", parsed as a list of URLs."
	}
	return which + "."
}

// DynamicPropertyDoc describes a property whose meaning comes from the server
// rather than from the data model.
func DynamicPropertyDoc(name string) string {
	switch {
	case strings.HasPrefix(name, "header:"):
		return "The " + name + " header field, in the form the query asked for."
	case strings.HasPrefix(name, "digest:"):
		return "The digest of the blob under the " + strings.TrimPrefix(name, "digest:") +
			" algorithm, as base64."
	case name == "data":
		return "The blob's octets. The server returns them under data:asText or " +
			"data:asBase64, whichever suits what they hold, so this property " +
			"itself does not come back."
	}
	return "The " + name + " property, whose meaning the server decides."
}

// PrimaryAccountPhrase describes the account a query is sent to where the
// query does not say. A session has a primary account for each capability
// rather than one for everything, so the capability is named: a query reading
// identities and one creating a mailbox may be talking to two different
// accounts, and the only place that shows is here.
func PrimaryAccountPhrase(capabilities []string) string {
	const cost = ", which costs a session lookup on first use."
	switch len(capabilities) {
	case 0:
		return ""
	case 1:
		return "The query does not say which account to use, so the session's primary account for " +
			capabilities[0] + " is used" + cost
	}
	return "The query does not say which account to use, so the session's primary account is used for each of " +
		joinURIs(capabilities) + cost + " They need not be the same account."
}

// joinURIs renders a list of capability URIs for prose.
func joinURIs(uris []string) string {
	if len(uris) == 2 {
		return uris[0] + " and " + uris[1]
	}
	return strings.Join(uris[:len(uris)-1], ", ") + ", and " + uris[len(uris)-1]
}
